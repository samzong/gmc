package worktree

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/samzong/gmc/internal/gitcmd"
	"github.com/samzong/gmc/internal/gitutil"
)

type PromoteOptions struct {
	DryRun bool
}

type dupTaskFile struct {
	source string
	rel    string
}

type promotionPatch struct {
	name string
	data []byte
}

func (c *Client) resolveDupTaskFiles(parentRoot string, paths []string) ([]dupTaskFile, error) {
	if len(paths) == 0 {
		return nil, nil
	}
	canonicalParentRoot, err := filepath.EvalSymlinks(parentRoot)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve parent worktree path %s: %w", parentRoot, err)
	}
	canonicalParentRoot = filepath.Clean(canonicalParentRoot)

	result := make([]dupTaskFile, 0, len(paths))
	for _, raw := range paths {
		path := strings.TrimSpace(raw)
		if path == "" {
			return nil, errors.New("task file path cannot be empty")
		}
		absPath := path
		if !filepath.IsAbs(absPath) {
			var err error
			absPath, err = filepath.Abs(absPath)
			if err != nil {
				return nil, fmt.Errorf("failed to resolve task file %s: %w", path, err)
			}
		}
		absPath = filepath.Clean(absPath)
		info, err := os.Stat(absPath)
		if err != nil {
			return nil, fmt.Errorf("failed to inspect task file %s: %w", path, err)
		}
		if info.IsDir() {
			return nil, fmt.Errorf("task path must be a file, not a directory: %s", path)
		}
		canonicalAbsPath, err := filepath.EvalSymlinks(absPath)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve task file %s: %w", path, err)
		}
		rel, ok := relativePathWithin(canonicalParentRoot, canonicalAbsPath)
		if !ok {
			return nil, fmt.Errorf("task file must be inside parent worktree: %s", path)
		}
		result = append(result, dupTaskFile{source: absPath, rel: filepath.Clean(rel)})
	}
	return result, nil
}

func (c *Client) copyDupTaskFiles(files []dupTaskFile, targetRoot string) error {
	for _, file := range files {
		target := filepath.Join(targetRoot, file.rel)
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return fmt.Errorf("failed to create task file directory for %s: %w", file.rel, err)
		}
		if err := copyFile(file.source, target); err != nil {
			return fmt.Errorf("failed to copy task file %s: %w", file.rel, err)
		}
	}
	return nil
}

func relativePathFrom(base, target string) string {
	rel, err := filepath.Rel(base, target)
	if err != nil {
		return target
	}
	if rel == "." {
		return "."
	}
	if strings.HasPrefix(rel, ".."+string(filepath.Separator)) || rel == ".." {
		return target
	}
	return rel
}

func (c *Client) Promote(candidate string, opts PromoteOptions) (Report, error) {
	var report Report

	candidate = strings.TrimSpace(candidate)
	if candidate == "" {
		return report, errors.New("candidate worktree cannot be empty")
	}
	if err := c.ensureInit(); err != nil {
		return report, fmt.Errorf("failed to determine worktree search root: %w", err)
	}

	parentRoot, err := c.currentTopLevelRequired()
	if err != nil {
		return report, err
	}
	candidatePath, err := c.resolvePromoteCandidate(candidate)
	if err != nil {
		return report, err
	}
	if sameCleanPath(parentRoot, candidatePath) {
		return report, errors.New("candidate worktree must be different from the parent worktree")
	}

	untracked, err := c.untrackedFiles(candidatePath)
	if err != nil {
		return report, err
	}
	untracked, ignoredUntracked, err := filterExistingIdenticalFiles(parentRoot, candidatePath, untracked)
	if err != nil {
		return report, err
	}
	if rel, ok := relativePathWithin(parentRoot, candidatePath); ok {
		ignoredUntracked = append(ignoredUntracked, rel)
	}

	clean, err := c.isWorktreeCleanIgnoringUntracked(parentRoot, ignoredUntracked)
	if err != nil {
		return report, err
	}
	if !clean {
		return report, errors.New("parent worktree has uncommitted changes; clean it before promoting")
	}

	parentHead, err := c.gitOutput(parentRoot, "rev-parse", "HEAD")
	if err != nil {
		return report, err
	}
	candidateHead, err := c.gitOutput(candidatePath, "rev-parse", "HEAD")
	if err != nil {
		return report, err
	}
	mergeBase, err := c.gitOutput(parentRoot, "merge-base", parentHead, candidateHead)
	if err != nil {
		return report, fmt.Errorf("failed to find merge base between parent and candidate: %w", err)
	}

	patches, err := c.collectPromotionPatches(candidatePath, mergeBase)
	if err != nil {
		return report, err
	}
	files, err := c.collectPromotionFileNames(candidatePath, mergeBase, untracked)
	if err != nil {
		return report, err
	}
	if len(patches) == 0 && len(untracked) == 0 {
		report.Info("No candidate changes to promote.")
		return report, nil
	}

	if err := c.checkPromotion(parentHead, patches, candidatePath, untracked); err != nil {
		return report, err
	}
	if opts.DryRun {
		report.Info("Dry run successful.")
		appendPromoteSummary(&report, parentRoot, candidatePath, files, true)
		return report, nil
	}

	if err := c.preflightUntrackedCopies(parentRoot, untracked); err != nil {
		return report, err
	}
	if err := applyPromotion(parentRoot, patches, candidatePath, untracked); err != nil {
		return report, err
	}

	appendPromoteSummary(&report, parentRoot, candidatePath, files, false)
	return report, nil
}

func (c *Client) currentTopLevelRequired() (string, error) {
	root := c.currentTopLevel()
	if root == "" {
		return "", errors.New("failed to determine current worktree root")
	}
	return root, nil
}

func (c *Client) resolvePromoteCandidate(candidate string) (string, error) {
	absCandidate := candidate
	if !filepath.IsAbs(absCandidate) {
		var err error
		absCandidate, err = filepath.Abs(candidate)
		if err != nil {
			return c.resolveWorktreePath(candidate)
		}
	}
	if absCandidate != "" {
		worktrees, listErr := c.ListCached()
		if listErr == nil {
			for _, wt := range worktrees {
				if sameCleanPath(wt.Path, absCandidate) {
					return wt.Path, nil
				}
			}
		}
	}
	return c.resolveWorktreePath(candidate)
}

func sameCleanPath(a, b string) bool {
	cleanA := filepath.Clean(a)
	cleanB := filepath.Clean(b)
	evalA, errA := filepath.EvalSymlinks(cleanA)
	if errA == nil {
		cleanA = filepath.Clean(evalA)
	}
	evalB, errB := filepath.EvalSymlinks(cleanB)
	if errB == nil {
		cleanB = filepath.Clean(evalB)
	}
	return cleanA == cleanB
}

func relativePathWithin(base, target string) (string, bool) {
	rel, err := filepath.Rel(filepath.Clean(base), filepath.Clean(target))
	if err != nil || rel == "." || filepath.IsAbs(rel) || rel == ".." ||
		strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", false
	}
	return filepath.Clean(rel), true
}

func (c *Client) isWorktreeCleanIgnoringUntracked(path string, ignored []string) (bool, error) {
	ignoredPaths := make([]string, 0, len(ignored))
	for _, rel := range ignored {
		ignoredPaths = append(ignoredPaths, filepath.Clean(rel))
	}

	result, err := c.runner.Run("-C", path, "status", "--porcelain=v1", "-z", "--untracked-files=all")
	if err != nil {
		return false, gitutil.WrapGitError("failed to inspect parent status", result, err)
	}
	for _, entry := range bytes.Split(result.Stdout, []byte{0}) {
		if len(entry) == 0 {
			continue
		}
		if len(entry) < 4 {
			return false, nil
		}
		status := string(entry[:2])
		name := filepath.Clean(string(entry[3:]))
		if status == "??" && isIgnoredUntrackedPath(name, ignoredPaths) {
			continue
		}
		return false, nil
	}
	return true, nil
}

func isIgnoredUntrackedPath(name string, ignored []string) bool {
	name = filepath.Clean(name)
	for _, rel := range ignored {
		if name == rel || strings.HasPrefix(name, rel+string(filepath.Separator)) {
			return true
		}
	}
	return false
}

func (c *Client) gitOutput(dir string, args ...string) (string, error) {
	result, err := c.runner.Run(append([]string{"-C", dir}, args...)...)
	if err != nil {
		return "", gitutil.WrapGitError("failed to run git "+strings.Join(args, " "), result, err)
	}
	output := result.StdoutString(true)
	if output == "" {
		return "", fmt.Errorf("git %s returned empty output", strings.Join(args, " "))
	}
	return output, nil
}

func (c *Client) gitBytes(dir string, args ...string) ([]byte, error) {
	result, err := c.runner.Run(append([]string{"-C", dir}, args...)...)
	if err != nil {
		return nil, gitutil.WrapGitError("failed to run git "+strings.Join(args, " "), result, err)
	}
	return result.Stdout, nil
}

func (c *Client) collectPromotionPatches(candidatePath, mergeBase string) ([]promotionPatch, error) {
	specs := []struct {
		name string
		args []string
	}{
		{name: "committed changes", args: []string{"diff", "--binary", "--full-index", "-M", mergeBase + "..HEAD"}},
		{name: "staged changes", args: []string{"diff", "--binary", "--full-index", "-M", "--cached"}},
		{name: "unstaged changes", args: []string{"diff", "--binary", "--full-index", "-M"}},
	}

	var patches []promotionPatch
	for _, spec := range specs {
		data, err := c.gitBytes(candidatePath, spec.args...)
		if err != nil {
			return nil, err
		}
		if len(bytes.TrimSpace(data)) == 0 {
			continue
		}
		patches = append(patches, promotionPatch{name: spec.name, data: data})
	}
	return patches, nil
}

func (c *Client) untrackedFiles(candidatePath string) ([]string, error) {
	data, err := c.gitBytes(candidatePath, "ls-files", "--others", "--exclude-standard", "-z")
	if err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return nil, nil
	}

	parts := bytes.Split(data, []byte{0})
	files := make([]string, 0, len(parts))
	for _, part := range parts {
		if len(part) == 0 {
			continue
		}
		path := filepath.Clean(string(part))
		if path == "." || filepath.IsAbs(path) || strings.HasPrefix(path, ".."+string(filepath.Separator)) || path == ".." {
			return nil, fmt.Errorf("unsafe untracked path: %s", string(part))
		}
		files = append(files, path)
	}
	return files, nil
}

func (c *Client) collectPromotionFileNames(candidatePath, mergeBase string, untracked []string) ([]string, error) {
	specs := [][]string{
		{"diff", "--name-only", "-M", mergeBase + "..HEAD"},
		{"diff", "--name-only", "-M", "--cached"},
		{"diff", "--name-only", "-M"},
	}

	var files []string
	for _, args := range specs {
		data, err := c.gitBytes(candidatePath, args...)
		if err != nil {
			return nil, err
		}
		for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			files = append(files, filepath.Clean(line))
		}
	}
	files = append(files, untracked...)
	return uniquePromotionFiles(files), nil
}

func uniquePromotionFiles(files []string) []string {
	seen := make(map[string]struct{}, len(files))
	var result []string
	for _, file := range files {
		if _, ok := seen[file]; ok {
			continue
		}
		seen[file] = struct{}{}
		result = append(result, file)
	}
	return result
}

func filterExistingIdenticalFiles(parentRoot, candidatePath string, files []string) ([]string, []string, error) {
	filtered := make([]string, 0, len(files))
	var ignored []string
	for _, rel := range files {
		same, err := sameRegularFileContent(filepath.Join(candidatePath, rel), filepath.Join(parentRoot, rel))
		if err != nil {
			return nil, nil, err
		}
		if same {
			ignored = append(ignored, rel)
			continue
		}
		filtered = append(filtered, rel)
	}
	return filtered, ignored, nil
}

func sameRegularFileContent(a, b string) (bool, error) {
	aInfo, err := os.Lstat(a)
	if err != nil {
		return false, fmt.Errorf("failed to inspect source file %s: %w", a, err)
	}
	if !aInfo.Mode().IsRegular() {
		return false, nil
	}

	bInfo, err := os.Lstat(b)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("failed to inspect destination file %s: %w", b, err)
	}
	if !bInfo.Mode().IsRegular() || aInfo.Size() != bInfo.Size() {
		return false, nil
	}

	aFile, err := os.Open(a)
	if err != nil {
		return false, fmt.Errorf("failed to open source file %s: %w", a, err)
	}
	defer aFile.Close()

	bFile, err := os.Open(b)
	if err != nil {
		return false, fmt.Errorf("failed to open destination file %s: %w", b, err)
	}
	defer bFile.Close()

	aBuf := make([]byte, 32*1024)
	bBuf := make([]byte, 32*1024)
	for {
		aN, aErr := aFile.Read(aBuf)
		bN, bErr := bFile.Read(bBuf)
		if aN != bN || !bytes.Equal(aBuf[:aN], bBuf[:bN]) {
			return false, nil
		}
		if errors.Is(aErr, io.EOF) && errors.Is(bErr, io.EOF) {
			return true, nil
		}
		if aErr != nil && !errors.Is(aErr, io.EOF) {
			return false, fmt.Errorf("failed to read source file %s: %w", a, aErr)
		}
		if bErr != nil && !errors.Is(bErr, io.EOF) {
			return false, fmt.Errorf("failed to read destination file %s: %w", b, bErr)
		}
	}
}

func (c *Client) checkPromotion(
	parentHead string,
	patches []promotionPatch,
	candidatePath string,
	untracked []string,
) error {
	tmpDir, err := os.MkdirTemp("", "gmc-promote-check-*")
	if err != nil {
		return fmt.Errorf("failed to create promote check temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	checkPath := filepath.Join(tmpDir, "worktree")
	result, err := c.runner.RunLogged("-C", c.repoDir, "worktree", "add", "--detach", checkPath, parentHead)
	if err != nil {
		return gitutil.WrapGitError("failed to create promote check worktree", result, err)
	}
	defer func() {
		_, _ = c.runner.Run("-C", c.repoDir, "worktree", "remove", "--force", checkPath)
	}()

	if err := applyPromotion(checkPath, patches, candidatePath, untracked); err != nil {
		return fmt.Errorf("promotion patch does not apply cleanly: %w", err)
	}
	return nil
}

func applyPromotion(targetPath string, patches []promotionPatch, candidatePath string, untracked []string) error {
	for _, patch := range patches {
		if err := gitApply(targetPath, patch); err != nil {
			return err
		}
	}
	return copyUntrackedPromotionFiles(candidatePath, targetPath, untracked)
}

func gitApply(targetPath string, patch promotionPatch) error {
	cmd := exec.Command("git", "-C", targetPath, "apply", "--3way", "--binary")
	cmd.Stdin = bytes.NewReader(patch.data)
	var out bytes.Buffer
	var errOut bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errOut
	if err := cmd.Run(); err != nil {
		result := gitcmd.Result{Stdout: out.Bytes(), Stderr: errOut.Bytes()}
		return gitutil.WrapGitError("failed to apply "+patch.name, result, err)
	}
	return nil
}

func (c *Client) preflightUntrackedCopies(parentRoot string, untracked []string) error {
	for _, rel := range untracked {
		target := filepath.Join(parentRoot, rel)
		if _, err := os.Lstat(target); err == nil {
			return fmt.Errorf("cannot promote untracked file %s: destination already exists", rel)
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("failed to inspect destination for %s: %w", rel, err)
		}
	}
	return nil
}

func copyUntrackedPromotionFiles(candidatePath, targetPath string, untracked []string) error {
	for _, rel := range untracked {
		src := filepath.Join(candidatePath, rel)
		dst := filepath.Join(targetPath, rel)
		if _, err := os.Lstat(dst); err == nil {
			return fmt.Errorf("cannot promote untracked file %s: destination already exists", rel)
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("failed to inspect destination for %s: %w", rel, err)
		}
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return fmt.Errorf("failed to create destination directory for %s: %w", rel, err)
		}
		if err := copyPromotionFile(src, dst); err != nil {
			return fmt.Errorf("failed to copy untracked file %s: %w", rel, err)
		}
	}
	return nil
}

func copyPromotionFile(src, dst string) error {
	info, err := os.Lstat(src)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		target, err := os.Readlink(src)
		if err != nil {
			return err
		}
		return os.Symlink(target, dst)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("unsupported untracked file type: %s", src)
	}
	return copyFile(src, dst)
}

func appendPromoteSummary(report *Report, parentRoot, candidatePath string, files []string, dryRun bool) {
	if dryRun {
		report.Info("Would promote candidate: " + candidatePath)
		report.Info("Parent worktree: " + parentRoot)
	} else {
		report.Info("Promoted candidate: " + candidatePath)
		report.Info("Parent worktree: " + parentRoot)
	}
	if len(files) == 0 {
		report.Info("Changed files: none")
		return
	}
	report.Info("Changed files:")
	for _, file := range files {
		report.Info("  " + file)
	}
	if !dryRun {
		report.Info("Next step: review changes in the parent worktree, then commit explicitly.")
	}
}
