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

// Prune removes worktrees whose branches are merged into the base branch.
func (c *Client) Prune(opts PruneOptions) (Report, error) {
	var report Report

	root, err := c.GetWorktreeRoot()
	if err != nil {
		return report, fmt.Errorf("failed to find worktree root: %w", err)
	}

	baseBranch, err := c.resolveBaseBranch(root, opts.BaseBranch)
	if err != nil {
		return report, err
	}

	baseBranchName := localBranchName(baseBranch)

	worktrees, err := c.List()
	if err != nil {
		return report, err
	}

	repoDir := repoDirForGit(root)
	var prunedAny bool
	for _, wt := range worktrees {
		if wt.IsBare || filepath.Base(wt.Path) == ".bare" || wt.Path == root {
			continue
		}

		name := filepath.Base(wt.Path)
		if wt.IsLocked {
			report.Warn(fmt.Sprintf("Skipped %s: worktree is locked", name))
			continue
		}
		if wt.Branch == "" || wt.Branch == "(detached)" {
			report.Warn(fmt.Sprintf("Skipped %s: detached HEAD", name))
			continue
		}
		if wt.Branch == baseBranchName {
			report.Warn(fmt.Sprintf("Skipped %s: base branch '%s'", name, baseBranchName))
			continue
		}

		merged, err := c.isBranchMerged(root, wt.Branch, baseBranch)
		if err != nil {
			report.Warn(fmt.Sprintf("Skipped %s: %v", name, err))
			continue
		}
		if !merged {
			continue
		}

		status := c.GetWorktreeStatus(wt.Path)
		if status == "modified" && !opts.Force {
			report.Warn(fmt.Sprintf("Skipped %s: worktree has uncommitted changes (use --force)", name))
			continue
		}

		if opts.DryRun {
			report.Warn("Would remove worktree: " + wt.Path)
			report.Warn("  Branch: " + wt.Branch)
			report.Warn("  Status: " + status)
			report.Warn("Would delete branch: " + wt.Branch)
			prunedAny = true
			continue
		}

		args := []string{"-C", repoDir, "worktree", "remove"}
		if opts.Force {
			args = append(args, "--force")
		}
		args = append(args, wt.Path)

		result, err := c.runner.RunLogged(args...)
		if err != nil {
			return report, gitutil.WrapGitError("failed to remove worktree", result, err)
		}
		report.Warn(fmt.Sprintf("Removed worktree '%s'", name))

		result, err = c.runner.RunLogged("-C", repoDir, "branch", "-D", wt.Branch)
		if err != nil {
			return report, gitutil.WrapGitError("failed to delete branch", result, err)
		}
		report.Warn(fmt.Sprintf("Deleted branch '%s'", wt.Branch))
		prunedAny = true
	}

	if !prunedAny {
		report.Warn("No worktrees pruned.")
	}

	return report, nil
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
