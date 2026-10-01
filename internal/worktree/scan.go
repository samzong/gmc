package worktree

import (
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"sort"
	"strings"

	"github.com/samzong/gmc/internal/gitcmd"
)

var repositoryEnvVars = []string{
	"GIT_ALTERNATE_OBJECT_DIRECTORIES",
	"GIT_CONFIG",
	"GIT_CONFIG_PARAMETERS",
	"GIT_CONFIG_COUNT",
	"GIT_OBJECT_DIRECTORY",
	"GIT_DIR",
	"GIT_WORK_TREE",
	"GIT_IMPLICIT_WORK_TREE",
	"GIT_GRAFT_FILE",
	"GIT_INDEX_FILE",
	"GIT_NO_REPLACE_OBJECTS",
	"GIT_REPLACE_REF_BASE",
	"GIT_PREFIX",
	"GIT_SHALLOW_FILE",
	"GIT_COMMON_DIR",
}

type ScanResult struct {
	Worktrees []Info
	Warnings  []error
}

func Scan(root string) (ScanResult, error) {
	result := ScanResult{Worktrees: []Info{}}
	if _, err := exec.LookPath("git"); err != nil {
		return result, err
	}
	root, err := filepath.EvalSymlinks(root)
	if err != nil {
		return result, err
	}
	root, err = filepath.Abs(root)
	if err != nil {
		return result, err
	}
	runner := gitcmd.Runner{Env: []string{"GIT_OPTIONAL_LOCKS=0"}}
	for _, value := range os.Environ() {
		name, _, _ := strings.Cut(value, "=")
		if name != "GIT_OPTIONAL_LOCKS" && !slices.Contains(repositoryEnvVars, name) {
			runner.Env = append(runner.Env, value)
		}
	}
	seen := make(map[string]bool)
	add := func(path string, bare bool) bool {
		runner.Dir = path
		dirs, dirsErr := runner.Run("rev-parse", "--git-dir", "--git-common-dir")
		if dirsErr != nil {
			result.Warnings = append(result.Warnings,
				fmt.Errorf("cannot inspect repository %s: %w", path, gitFailure(dirs, dirsErr)))
			return false
		}
		lines := strings.Split(strings.TrimSuffix(string(dirs.Stdout), "\n"), "\n")
		if len(lines) != 2 {
			result.Warnings = append(result.Warnings,
				fmt.Errorf("cannot inspect repository %s: unexpected rev-parse output", path))
			return false
		}
		gitDir, resolveErr := resolveGitPath(path, lines[0])
		if resolveErr != nil {
			result.Warnings = append(result.Warnings, resolveErr)
			return false
		}
		commonDir, resolveErr := resolveGitPath(path, lines[1])
		if resolveErr != nil {
			result.Warnings = append(result.Warnings, resolveErr)
			return false
		}
		if bare && commonDir != path {
			return false
		}
		if seen[commonDir] {
			return true
		}
		client := &Client{runner: runner}
		listed, listErr := runner.Run("worktree", "list", "--porcelain")
		if listErr != nil {
			result.Warnings = append(result.Warnings,
				fmt.Errorf("cannot list worktrees for %s: %w", path, gitFailure(listed, listErr)))
			return true
		}
		worktrees, _ := client.resolveWorktreeList(listed.Stdout)
		if len(worktrees) == 0 {
			return true
		}
		if bare && !worktrees[0].IsBare {
			return true
		}
		seen[commonDir] = true
		if len(worktrees) == 1 {
			return true
		}
		if !bare && gitDir == commonDir && !worktrees[0].IsBare && worktrees[0].Path == gitDir {
			worktrees[0].Path = path
		}
		repository := worktrees[0].Path
		if worktrees[0].IsBare && filepath.Base(repository) == ".bare" {
			repository = filepath.Dir(repository)
		}
		for _, wt := range worktrees {
			if wt.IsBare {
				continue
			}
			wt.Repository = repository
			if _, statErr := os.Stat(wt.Path); os.IsNotExist(statErr) {
				wt.Status = "missing"
			} else {
				wt.Status = client.scanWorktreeStatus(wt.Path)
			}
			result.Worktrees = append(result.Worktrees, wt)
		}
		return true
	}
	err = filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			if path == root {
				return walkErr
			}
			result.Warnings = append(result.Warnings, walkErr)
			return nil
		}
		if entry.IsDir() && path != root && skipScanDir(root, path, entry.Name()) {
			return filepath.SkipDir
		}
		if entry.Name() == ".git" {
			info, statErr := os.Stat(path)
			if statErr == nil && !info.IsDir() && info.Size() == 0 {
				return nil
			}
			add(filepath.Dir(path), false)
			if entry.IsDir() {
				return filepath.SkipDir
			}
		}
		if entry.Name() == "HEAD" && !entry.IsDir() {
			parent := filepath.Dir(path)
			if info, statErr := os.Stat(filepath.Join(parent, "objects")); statErr == nil && info.IsDir() && add(parent, true) {
				return filepath.SkipDir
			}
		}
		return nil
	})
	sort.SliceStable(result.Worktrees, func(i, j int) bool {
		return result.Worktrees[i].Repository < result.Worktrees[j].Repository
	})
	return result, err
}

func (c *Client) scanWorktreeStatus(path string) string {
	return c.statusSummary("--no-optional-locks", "-c", "core.fsmonitor=false", "-C", path, "status", "--porcelain")
}

func resolveGitPath(dir, value string) (string, error) {
	if !filepath.IsAbs(value) {
		value = filepath.Join(dir, value)
	}
	return filepath.EvalSymlinks(value)
}

func gitFailure(result gitcmd.Result, err error) error {
	lines := strings.Split(result.StderrString(true), "\n")
	for i, line := range lines {
		lines[i] = strings.TrimSpace(line)
	}
	reason := strings.Join(lines, "; ")
	if reason == "" {
		return err
	}
	return fmt.Errorf("%s (%w)", reason, err)
}

func skipScanDir(root, path, name string) bool {
	if name == "node_modules" {
		return true
	}
	return runtime.GOOS == "darwin" && (name == "Library" || name == ".Trash") && filepath.Dir(path) == root
}
