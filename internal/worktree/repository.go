package worktree

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func (c *Client) resolveWorktreePath(worktreeName string) (string, error) {
	if worktreeName == "" {
		return "", errors.New("worktree name cannot be empty")
	}

	c.once.Do(c.init)
	repoRoot := c.worktreeRoot
	worktrees, err := c.ListCached()
	if err != nil {
		if repoRoot != "" {
			candidate := filepath.Join(repoRoot, worktreeName)
			if info, statErr := os.Stat(candidate); statErr == nil && info.IsDir() {
				return candidate, nil
			}
		}
		return "", err
	}

	var exactMatches []string
	var relMatches []string
	var baseMatches []string
	for _, wt := range worktrees {
		if wt.Path == worktreeName {
			exactMatches = append(exactMatches, wt.Path)
			continue
		}
		if repoRoot != "" {
			if rel, relErr := filepath.Rel(repoRoot, wt.Path); relErr == nil && rel == worktreeName {
				relMatches = append(relMatches, wt.Path)
				continue
			}
		}
		if filepath.Base(wt.Path) == worktreeName {
			baseMatches = append(baseMatches, wt.Path)
		}
	}

	for i, matches := range [][]string{exactMatches, relMatches, baseMatches} {
		if len(matches) > 0 {
			return uniqueWorktreeMatch(worktreeName, matches, []string{"exact path", "repo-relative path", "basename"}[i])
		}
	}

	return "", fmt.Errorf("worktree not found: %s", worktreeName)
}

func (c *Client) currentTopLevel() string {
	result, err := c.runner.Run("rev-parse", "--show-toplevel")
	if err != nil {
		return ""
	}
	root := result.StdoutString(true)
	if root == "" {
		return ""
	}
	if filepath.IsAbs(root) {
		return filepath.Clean(root)
	}
	absRoot, absErr := filepath.Abs(root)
	if absErr != nil {
		return ""
	}
	return absRoot
}

func uniqueWorktreeMatch(input string, matches []string, matchType string) (string, error) {
	if len(matches) == 0 {
		return "", nil
	}
	if len(matches) == 1 {
		return matches[0], nil
	}
	return "", fmt.Errorf("ambiguous worktree %q by %s: %s", input, matchType, strings.Join(matches, ", "))
}

func (c *Client) getGitOutput(dir string, args ...string) string {
	fullArgs := append([]string{"-C", dir}, args...)
	result, err := c.runner.Run(fullArgs...)
	if err != nil {
		return ""
	}
	return result.StdoutString(true)
}

func FindBareRoot(startDir string) (string, error) {
	if startDir == "" {
		var err error
		startDir, err = os.Getwd()
		if err != nil {
			return "", fmt.Errorf("failed to get current directory: %w", err)
		}
	}

	dir := startDir
	for {
		if filepath.Base(dir) == ".bare" {
			return filepath.Dir(dir), nil
		}
		bareDir := filepath.Join(dir, ".bare")
		if info, err := os.Stat(bareDir); err == nil && info.IsDir() {
			return dir, nil
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	return "", errors.New("no .bare directory found in parent directories")
}

func (c *Client) GetGitCommonDir() (string, error) {
	result, err := c.runner.Run("rev-parse", "--git-common-dir")
	if err != nil {
		return "", fmt.Errorf("not in a git repository: %w", err)
	}

	commonDir := result.StdoutString(true)
	if commonDir == "" {
		return "", errors.New("failed to determine git common directory")
	}

	if filepath.IsAbs(commonDir) {
		return filepath.Clean(commonDir), nil
	}

	absCommonDir, err := filepath.Abs(commonDir)
	if err != nil {
		return "", fmt.Errorf("failed to get absolute path: %w", err)
	}
	return absCommonDir, nil
}

func (c *Client) GetWorktreeRoot() (string, error) {
	if err := c.ensureInit(); err != nil {
		return "", err
	}
	return c.worktreeRoot, nil
}

func samePath(a, b string) bool {
	if filepath.Clean(a) == filepath.Clean(b) {
		return true
	}
	aa, aerr := filepath.EvalSymlinks(a)
	bb, berr := filepath.EvalSymlinks(b)
	return aerr == nil && berr == nil && filepath.Clean(aa) == filepath.Clean(bb)
}

func pathWithin(root, path string) bool {
	if rel, err := filepath.Rel(root, path); err == nil && isLocalRel(rel) {
		return true
	}
	rr, rerr := filepath.EvalSymlinks(root)
	pp, perr := filepath.EvalSymlinks(path)
	if rerr != nil || perr != nil {
		return false
	}
	rel, err := filepath.Rel(rr, pp)
	return err == nil && isLocalRel(rel)
}

func isLocalRel(rel string) bool {
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func isExternalPath(root, wtPath string) bool {
	if root == "" {
		return false
	}
	rel, err := filepath.Rel(root, wtPath)
	if err != nil {
		return true
	}
	return strings.HasPrefix(rel, ".."+string(filepath.Separator)) || rel == ".."
}

type ProtectionPolicy struct {
	MainBranch string
	RootPath   string
}

func (c *Client) NewProtectionPolicy() (ProtectionPolicy, error) {
	var p ProtectionPolicy
	root, err := c.GetWorktreeRoot()
	if err != nil {
		return p, fmt.Errorf("failed to get worktree root: %w", err)
	}
	p.RootPath = root
	repoDir := repoDirForGit(root)
	isBareLayout := repoDir != root
	branch, err := c.resolveBaseBranchWithPolicy(repoDir, "", isBareLayout)
	if err != nil {
		return p, fmt.Errorf("failed to resolve main branch: %w", err)
	}
	p.MainBranch = localBranchName(branch)
	return p, nil
}

func (p ProtectionPolicy) IsProtected(wt Info) bool {
	return wt.IsBare || (p.RootPath != "" && wt.Path == p.RootPath) ||
		(p.MainBranch != "" && wt.Branch == p.MainBranch)
}

func (p ProtectionPolicy) Reason(wt Info) string {
	if wt.IsBare {
		return "bare repository"
	}
	if p.RootPath != "" && wt.Path == p.RootPath {
		return "main worktree"
	}
	return "main branch"
}

func (c *Client) resolvedMainBranch() (string, error) {
	pp, err := c.NewProtectionPolicy()
	if err != nil {
		return "", err
	}
	return pp.MainBranch, nil
}
