package worktree

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/samzong/gmc/internal/gitutil"
)

// PruneOptions options for pruning merged worktrees.
type PruneOptions struct {
	BaseBranch string // Base branch to check merge status against
	Force      bool   // Force removal even if worktree is dirty
	DryRun     bool   // Preview what would be removed without making changes
}

type PruneCandidate struct {
	Name   string
	Branch string
	Status string
}

type PruneResult struct {
	Report
	Candidates []PruneCandidate
}

// Prune removes worktrees whose branches are merged into the base branch.
func (c *Client) Prune(opts PruneOptions) (PruneResult, error) {
	var result PruneResult

	root, err := c.GetWorktreeRoot()
	if err != nil {
		return result, fmt.Errorf("failed to find worktree root: %w", err)
	}

	baseBranch, err := c.resolveBaseBranch(root, opts.BaseBranch)
	if err != nil {
		return result, err
	}

	baseBranchName := localBranchName(baseBranch)

	worktrees, err := c.List()
	if err != nil {
		return result, err
	}

	repoDir := repoDirForGit(root)
	isBare := repoDir != root
	var prunedAny bool
	for _, wt := range worktrees {
		if wt.IsBare || filepath.Base(wt.Path) == ".bare" || wt.Path == root {
			continue
		}
		if isBare && isExternalPath(root, wt.Path) {
			continue
		}

		name := filepath.Base(wt.Path)
		if wt.IsLocked {
			result.Warn(fmt.Sprintf("Skipped %s: worktree is locked", name))
			continue
		}
		if wt.Branch == "" || wt.Branch == "(detached)" {
			result.Warn(fmt.Sprintf("Skipped %s: detached HEAD", name))
			continue
		}
		if wt.Branch == baseBranchName {
			result.Warn(fmt.Sprintf("Skipped %s: base branch '%s'", name, baseBranchName))
			continue
		}

		merged, err := c.isBranchMerged(root, wt.Branch, baseBranch)
		if err != nil {
			result.Warn(fmt.Sprintf("Skipped %s: %v", name, err))
			continue
		}
		if !merged {
			continue
		}

		status := c.GetWorktreeStatus(wt.Path)
		if status == "modified" && !opts.Force {
			result.Warn(fmt.Sprintf("Skipped %s: worktree has uncommitted changes (use --force)", name))
			continue
		}

		candidate := PruneCandidate{Name: name, Branch: wt.Branch, Status: status}

		if opts.DryRun {
			result.Warn("Would remove worktree: " + wt.Path)
			result.Warn("  Branch: " + wt.Branch)
			result.Warn("  Status: " + status)
			result.Warn("Would delete branch: " + wt.Branch)
			result.Candidates = append(result.Candidates, candidate)
			prunedAny = true
			continue
		}

		args := []string{"-C", repoDir, "worktree", "remove"}
		if opts.Force {
			args = append(args, "--force")
		}
		args = append(args, wt.Path)

		gitResult, err := c.runner.RunLogged(args...)
		if err != nil {
			return result, gitutil.WrapGitError("failed to remove worktree", gitResult, err)
		}
		result.Warn(fmt.Sprintf("Removed worktree '%s'", name))

		gitResult, err = c.runner.RunLogged("-C", repoDir, "branch", "-D", wt.Branch)
		if err != nil {
			return result, gitutil.WrapGitError("failed to delete branch", gitResult, err)
		}
		result.Warn(fmt.Sprintf("Deleted branch '%s'", wt.Branch))
		result.Candidates = append(result.Candidates, candidate)
		prunedAny = true
	}

	if !prunedAny {
		result.Warn("No worktrees pruned.")
	}

	return result, nil
}

func (c *Client) resolveBaseBranch(root string, override string) (string, error) {
	return c.resolveBaseBranchWithPolicy(repoDirForGit(root), override, true)
}

func (c *Client) isBranchMerged(root string, branch string, base string) (bool, error) {
	if branch == "" || base == "" {
		return false, errors.New("branch or base is empty")
	}

	return c.isAncestor(repoDirForGit(root), branch, base)
}

func repoDirForGit(root string) string {
	if root == "" {
		return ""
	}
	bareDir := filepath.Join(root, ".bare")
	if info, err := os.Stat(bareDir); err == nil && info.IsDir() {
		return bareDir
	}
	return root
}

func localBranchName(ref string) string {
	if strings.HasPrefix(ref, "refs/heads/") {
		return strings.TrimPrefix(ref, "refs/heads/")
	}
	if strings.HasPrefix(ref, "refs/remotes/") {
		rest := strings.TrimPrefix(ref, "refs/remotes/")
		parts := strings.SplitN(rest, "/", 2)
		if len(parts) == 2 {
			return parts[1]
		}
		return rest
	}
	if strings.HasPrefix(ref, "origin/") || strings.HasPrefix(ref, "upstream/") {
		parts := strings.SplitN(ref, "/", 2)
		if len(parts) == 2 {
			return parts[1]
		}
	}
	return ref
}
