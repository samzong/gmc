package worktree

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
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
func (c *Client) Prune(opts PruneOptions) error {
	root, err := c.GetWorktreeRoot()
	if err != nil {
		return fmt.Errorf("failed to find worktree root: %w", err)
	}

	baseBranch, err := c.resolveBaseBranch(root, opts.BaseBranch)
	if err != nil {
		return err
	}

	baseBranchName := localBranchName(baseBranch)

	worktrees, err := c.List()
	if err != nil {
		return err
	}

	var prunedAny bool
	for _, wt := range worktrees {
		if wt.IsBare || filepath.Base(wt.Path) == ".bare" || wt.Path == root {
			continue
		}

		name := filepath.Base(wt.Path)
		if wt.IsLocked {
			fmt.Fprintf(os.Stderr, "Skipped %s: worktree is locked\n", name)
			continue
		}
		if wt.Branch == "" || wt.Branch == "(detached)" {
			fmt.Fprintf(os.Stderr, "Skipped %s: detached HEAD\n", name)
			continue
		}
		if wt.Branch == baseBranchName {
			fmt.Fprintf(os.Stderr, "Skipped %s: base branch '%s'\n", name, baseBranchName)
			continue
		}

		merged, err := c.isBranchMerged(root, wt.Branch, baseBranch)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Skipped %s: %v\n", name, err)
			continue
		}
		if !merged {
			continue
		}

		status := c.GetWorktreeStatus(wt.Path)
		if status == "modified" && !opts.Force {
			fmt.Fprintf(os.Stderr, "Skipped %s: worktree has uncommitted changes (use --force)\n", name)
			continue
		}

		if opts.DryRun {
			fmt.Fprintf(os.Stderr, "Would remove worktree: %s\n", wt.Path)
			fmt.Fprintf(os.Stderr, "  Branch: %s\n", wt.Branch)
			fmt.Fprintf(os.Stderr, "  Status: %s\n", status)
			fmt.Fprintf(os.Stderr, "Would delete branch: %s\n", wt.Branch)
			prunedAny = true
			continue
		}

		args := []string{"worktree", "remove"}
		if opts.Force {
			args = append(args, "--force")
		}
		args = append(args, wt.Path)

		result, err := c.runner.RunLogged(args...)
		if err != nil {
			return gitutil.WrapGitError("failed to remove worktree", result, err)
		}
		fmt.Fprintf(os.Stderr, "Removed worktree '%s'\n", name)

		result, err = c.runner.RunLogged("branch", "-D", wt.Branch)
		if err != nil {
			return gitutil.WrapGitError("failed to delete branch", result, err)
		}
		fmt.Fprintf(os.Stderr, "Deleted branch '%s'\n", wt.Branch)
		prunedAny = true
	}

	if !prunedAny {
		fmt.Fprintln(os.Stderr, "No worktrees pruned.")
	}

	return nil
}

func (c *Client) resolveBaseBranch(root string, override string) (string, error) {
	if override != "" {
		return override, nil
	}
	if root == "" {
		return "", errors.New("cannot determine base branch: repository root is empty")
	}

	repoDir := repoDirForGit(root)

	if ref := c.gitSymbolicRef(repoDir, "refs/remotes/origin/HEAD"); ref != "" {
		return ref, nil
	}
	if ref := c.gitSymbolicRef(repoDir, "refs/remotes/upstream/HEAD"); ref != "" {
		return ref, nil
	}
	if ref := c.gitSymbolicRef(repoDir, "HEAD"); ref != "" {
		return ref, nil
	}

	for _, branch := range []string{"main", "master"} {
		if c.gitRefExists(repoDir, "refs/heads/"+branch) {
			return branch, nil
		}
	}

	return "", errors.New("could not determine base branch; specify with --base")
}

func (c *Client) isBranchMerged(root string, branch string, base string) (bool, error) {
	if branch == "" || base == "" {
		return false, errors.New("branch or base is empty")
	}

	repoDir := repoDirForGit(root)
	args := []string{"-C", repoDir, "merge-base", "--is-ancestor", branch, base}
	result, err := c.runner.Run(args...)
	if err == nil {
		return true, nil
	}

	// merge-base --is-ancestor exit codes: 0 (ancestor), 1 (not ancestor), 128+ (error)
	if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
		return false, nil
	}

	return false, gitutil.WrapGitError("failed to check merge status", result, err)
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

func (c *Client) gitSymbolicRef(repoDir string, ref string) string {
	result, err := c.runner.Run("-C", repoDir, "symbolic-ref", "--short", ref)
	if err != nil {
		return ""
	}
	return result.StdoutString(true)
}

func (c *Client) gitRefExists(repoDir string, ref string) bool {
	_, err := c.runner.Run("-C", repoDir, "rev-parse", "--verify", ref)
	return err == nil
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
