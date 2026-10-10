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

type DupItem struct {
	CreateResult
	RelativePath string
}

type DupResult struct {
	Items      []DupItem
	TaskFiles  []string
	BaseBranch string
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
		Items:      make([]DupItem, 0, opts.Count),
		TaskFiles:  taskPaths,
		BaseBranch: opts.BaseBranch,
	}

	for i := 1; i <= opts.Count; i++ {
		dirName := fmt.Sprintf(".dup-%d", i)
		targetPath := filepath.Join(targetRoot, dirName)
		item := DupItem{
			CreateResult: CreateResult{
				Name:   dirName,
				Path:   targetPath,
				Branch: fmt.Sprintf("_dup/%s/%s-%d", opts.BaseBranch, timestamp, i),
				Base:   opts.BaseBranch,
			},
		}
		err := c.createDupWorktree(&item, taskFiles)
		item.Err = err
		if item.Created {
			item.RelativePath = relativePathFrom(relativeBase, item.Path)
			c.InvalidateList()
		}
		dupResult.Items = append(dupResult.Items, item)
		if err != nil {
			return dupResult, err
		}
	}

	return dupResult, nil
}

func (c *Client) createDupWorktree(item *DupItem, taskFiles []dupTaskFile) error {
	if _, err := os.Stat(item.Path); err == nil {
		return fmt.Errorf("directory already exists: %s", item.Path)
	}

	args := []string{"-C", c.repoDir, "worktree", "add", "-b", item.Branch, item.Path, item.Base}
	runResult, err := c.runner.RunLogged(args...)
	if err != nil {
		return gitutil.WrapGitError("failed to create worktree "+item.Name, runResult, err)
	}
	item.Created = true
	if err := c.ensureAddedWorktreeConfig(item.Path); err != nil {
		return err
	}

	sharedReport, err := c.prepareNewWorktree(item.Path)
	if err != nil {
		item.Warnings = append(item.Warnings,
			fmt.Sprintf("Warning: failed to sync shared resources for %s: %v", item.Name, err))
	}
	item.Warnings = append(item.Warnings, sharedReport.Warnings()...)
	return c.copyDupTaskFiles(taskFiles, item.Path)
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
