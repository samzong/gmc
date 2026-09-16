package worktree

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/samzong/gmc/internal/gitutil"
)

type dupTaskFile struct {
	source string
	rel    string
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
	if evalBase, err := filepath.EvalSymlinks(base); err == nil {
		base = evalBase
	}
	if evalTarget, err := filepath.EvalSymlinks(target); err == nil {
		target = evalTarget
	}
	rel, err := filepath.Rel(base, target)
	if err != nil {
		return target
	}
	if rel == "." {
		return "."
	}
	return rel
}

type DupOptions struct {
	BaseBranch string
	Count      int
	TaskFiles  []string
}

type DupResult struct {
	Worktrees     []string
	WorktreePaths []string
	RelativePaths []string
	Branches      []string
	TaskFiles     []string
	Warnings      []string
	BaseBranch    string
}

func (c *Client) Dup(opts DupOptions) (*DupResult, error) {
	if opts.Count < 1 {
		opts.Count = 2
	}

	if err := c.ensureInit(); err != nil {
		return nil, fmt.Errorf("failed to find worktree root: %w", err)
	}

	opts.BaseBranch = c.resolveDupBaseBranch(opts.BaseBranch)
	targetRoot := c.dupTargetRoot()

	relativeBase := targetRoot
	if cwd, cwdErr := os.Getwd(); cwdErr == nil {
		relativeBase = cwd
	}
	var taskFiles []dupTaskFile
	var taskPaths []string
	if len(opts.TaskFiles) > 0 {
		parentRoot, err := c.currentTopLevelRequired()
		if err != nil {
			return nil, err
		}
		taskFiles, err = c.resolveDupTaskFiles(parentRoot, opts.TaskFiles)
		if err != nil {
			return nil, err
		}
		taskPaths = make([]string, 0, len(taskFiles))
		for _, file := range taskFiles {
			taskPaths = append(taskPaths, file.rel)
		}
	}

	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	dupResult := &DupResult{
		Worktrees:     make([]string, 0, opts.Count),
		WorktreePaths: make([]string, 0, opts.Count),
		RelativePaths: make([]string, 0, opts.Count),
		Branches:      make([]string, 0, opts.Count),
		TaskFiles:     taskPaths,
		BaseBranch:    opts.BaseBranch,
	}

	for i := 1; i <= opts.Count; i++ {
		dirName := fmt.Sprintf(".dup-%d", i)
		branchName := fmt.Sprintf("_dup/%s/%s-%d", opts.BaseBranch, timestamp, i)
		targetPath := filepath.Join(targetRoot, dirName)

		if _, err := os.Stat(targetPath); err == nil {
			return nil, fmt.Errorf("directory already exists: %s", targetPath)
		}

		args := []string{"-C", c.repoDir, "worktree", "add", "-b", branchName, targetPath, opts.BaseBranch}
		runResult, err := c.runner.RunLogged(args...)
		if err != nil {
			return nil, gitutil.WrapGitError("failed to create worktree "+dirName, runResult, err)
		}
		if err := c.ensureAddedWorktreeConfig(targetPath); err != nil {
			return nil, err
		}

		sharedReport, err := c.prepareNewWorktree(targetPath)
		if err != nil {
			dupResult.Warnings = append(
				dupResult.Warnings,
				fmt.Sprintf("Warning: failed to sync shared resources for %s: %v", dirName, err),
			)
		}
		for _, event := range sharedReport.Events {
			if event.Level == EventWarn {
				dupResult.Warnings = append(dupResult.Warnings, event.Message)
			}
		}
		if err := c.copyDupTaskFiles(taskFiles, targetPath); err != nil {
			return nil, err
		}

		dupResult.Worktrees = append(dupResult.Worktrees, dirName)
		dupResult.WorktreePaths = append(dupResult.WorktreePaths, targetPath)
		dupResult.RelativePaths = append(dupResult.RelativePaths, relativePathFrom(relativeBase, targetPath))
		dupResult.Branches = append(dupResult.Branches, branchName)
	}

	c.InvalidateList()

	return dupResult, nil
}

func (c *Client) resolveDupBaseBranch(override string) string {
	if override != "" {
		return override
	}
	if currentRoot := c.currentTopLevel(); currentRoot != "" {
		if branch := c.gitSymbolicRef(currentRoot, "HEAD"); branch != "" {
			return branch
		}
		return "HEAD"
	}
	if c.repoDir != "" {
		if branch := c.gitSymbolicRef(c.repoDir, "HEAD"); branch != "" {
			return branch
		}
	}
	return "HEAD"
}

func (c *Client) dupTargetRoot() string {
	if currentRoot := c.currentTopLevel(); currentRoot != "" {
		return filepath.Dir(currentRoot)
	}
	return c.worktreeRoot
}
