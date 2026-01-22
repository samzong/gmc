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

// SyncOptions controls worktree sync behavior.
type SyncOptions struct {
	BaseBranch string // Base branch to sync
	DryRun     bool   // Preview actions without making changes
}

// ResolveSyncBaseBranch resolves the base branch used for syncing.
func (c *Client) ResolveSyncBaseBranch(override string) (string, error) {
	root, err := c.GetWorktreeRoot()
	if err != nil {
		return "", fmt.Errorf("failed to find worktree root: %w", err)
	}
	return c.resolveSyncBaseBranch(repoDirForGit(root), override)
}

// Sync updates the base branch refs and main worktree using fast-forward only.
func (c *Client) Sync(opts SyncOptions) error {
	root, err := c.GetWorktreeRoot()
	if err != nil {
		return fmt.Errorf("failed to find worktree root: %w", err)
	}

	repoDir := repoDirForGit(root)
	remote, err := c.selectSyncRemote(repoDir)
	if err != nil {
		return err
	}

	baseRef, err := c.resolveSyncBaseBranch(repoDir, opts.BaseBranch)
	if err != nil {
		return err
	}
	baseName := localBranchName(baseRef)

	remoteRef := fmt.Sprintf("%s/%s", remote, baseName)
	remoteFull := "refs/remotes/" + remoteRef
	localFull := "refs/heads/" + baseName

	origDir := c.runner.Dir
	if repoDir != "" {
		c.runner.Dir = repoDir
		defer func() {
			c.runner.Dir = origDir
		}()
	}
	worktrees, err := c.List()
	if err != nil {
		return err
	}
	baseWorktree := findWorktreeForBranch(worktrees, baseName)
	status := ""
	if baseWorktree != "" {
		status = c.GetWorktreeStatus(baseWorktree)
	}

	if opts.DryRun {
		return c.syncDryRun(repoDir, remote, baseName, remoteRef, localFull, remoteFull, baseWorktree, status)
	}

	result, err := c.runner.RunLogged("-C", repoDir, "fetch", remote)
	if err != nil {
		return gitutil.WrapGitError("failed to fetch "+remote, result, err)
	}

	canFF, err := c.canFastForward(repoDir, localFull, remoteFull)
	if err != nil {
		return err
	}
	if !canFF {
		return fmt.Errorf("base branch '%s' cannot be fast-forwarded to %s", baseName, remoteRef)
	}

	localHash := c.refHash(repoDir, localFull)
	remoteHash := c.refHash(repoDir, remoteFull)
	needsUpdate := localHash == "" || (remoteHash != "" && localHash != remoteHash)
	updated := false

	if needsUpdate {
		result, err = c.runner.RunLogged("-C", repoDir, "update-ref", localFull, remoteFull)
		if err != nil {
			return gitutil.WrapGitError("failed to update base branch", result, err)
		}
		updated = true
	}

	if remote == "upstream" && updated && c.remoteExists(repoDir, "origin") {
		result, err = c.runner.RunLogged("-C", repoDir, "push", "origin", baseName)
		if err != nil {
			msg := strings.TrimSpace(result.StderrString(true))
			if msg == "" {
				msg = err.Error()
			}
			fmt.Fprintf(os.Stderr, "Warning: failed to push origin %s: %s\n", baseName, msg)
		}
	}

	if updated {
		fmt.Printf("Synced %s to %s (%s..%s)\n", baseName, remoteRef, shortHash(localHash), shortHash(remoteHash))
	} else {
		fmt.Printf("%s already up to date with %s (%s)\n", baseName, remoteRef, shortHash(localHash))
	}

	if baseWorktree == "" {
		fmt.Fprintf(os.Stderr, "%s worktree not found, refs updated only\n", baseName)
		return nil
	}
	if status == "modified" {
		fmt.Fprintf(os.Stderr, "%s worktree has uncommitted changes, skipped\n", baseName)
		return nil
	}
	if status != "clean" {
		fmt.Fprintf(os.Stderr, "%s worktree status unknown, skipped\n", baseName)
		return nil
	}

	if updated {
		result, err = c.runner.RunLogged("-C", baseWorktree, "merge", "--ff-only", baseName)
		if err != nil {
			return gitutil.WrapGitError("failed to update worktree", result, err)
		}
	}

	return nil
}

func (c *Client) syncDryRun(
	repoDir string, remote string, baseName string, remoteRef string,
	localFull string, remoteFull string, baseWorktree string, status string,
) error {
	canFF, err := c.canFastForward(repoDir, localFull, remoteFull)
	if err != nil {
		return err
	}
	if !canFF {
		return fmt.Errorf("base branch '%s' cannot be fast-forwarded to %s", baseName, remoteRef)
	}

	localHash := c.refHash(repoDir, localFull)
	remoteHash := c.refHash(repoDir, remoteFull)
	needsUpdate := localHash == "" || (remoteHash != "" && localHash != remoteHash)

	fmt.Fprintf(os.Stderr, "Would fetch %s\n", remote)
	if needsUpdate {
		fmt.Fprintf(os.Stderr, "Would fast-forward %s to %s\n", baseName, remoteRef)
	} else {
		fmt.Fprintf(os.Stderr, "%s is already up to date with %s\n", baseName, remoteRef)
	}

	if remote == "upstream" && needsUpdate && c.remoteExists(repoDir, "origin") {
		fmt.Fprintf(os.Stderr, "Would push origin %s\n", baseName)
	}

	if baseWorktree == "" {
		fmt.Fprintf(os.Stderr, "%s worktree not found, refs updated only\n", baseName)
		return nil
	}
	if status == "modified" {
		fmt.Fprintf(os.Stderr, "%s worktree has uncommitted changes, skipped\n", baseName)
		return nil
	}
	if status != "clean" {
		fmt.Fprintf(os.Stderr, "%s worktree status unknown, skipped\n", baseName)
		return nil
	}
	if needsUpdate {
		fmt.Fprintf(os.Stderr, "Would update worktree: %s\n", baseWorktree)
	}

	return nil
}

func (c *Client) resolveSyncBaseBranch(repoDir string, override string) (string, error) {
	if override != "" {
		return override, nil
	}
	if repoDir == "" {
		return "", errors.New("cannot determine base branch: repository root is empty")
	}

	if ref := c.gitSymbolicRef(repoDir, "refs/remotes/origin/HEAD"); ref != "" {
		return ref, nil
	}
	if ref := c.gitSymbolicRef(repoDir, "refs/remotes/upstream/HEAD"); ref != "" {
		return ref, nil
	}

	for _, branch := range []string{"main", "master"} {
		if c.gitRefExists(repoDir, "refs/heads/"+branch) {
			return branch, nil
		}
	}

	return "", errors.New("could not determine base branch; specify with --base")
}

func (c *Client) selectSyncRemote(repoDir string) (string, error) {
	if c.remoteExists(repoDir, "upstream") {
		return "upstream", nil
	}
	if c.remoteExists(repoDir, "origin") {
		return "origin", nil
	}
	return "", errors.New("no upstream or origin remote found")
}

func (c *Client) remoteExists(repoDir string, name string) bool {
	result, err := c.runner.Run("-C", repoDir, "remote", "get-url", name)
	return err == nil && strings.TrimSpace(result.StdoutString(true)) != ""
}

func findWorktreeForBranch(worktrees []Info, branch string) string {
	for _, wt := range worktrees {
		if wt.IsBare || filepath.Base(wt.Path) == ".bare" {
			continue
		}
		if wt.Branch == branch {
			return wt.Path
		}
	}
	return ""
}

func (c *Client) canFastForward(repoDir string, localFull string, remoteFull string) (bool, error) {
	if !c.gitRefExists(repoDir, remoteFull) {
		return false, fmt.Errorf("remote branch '%s' not found", strings.TrimPrefix(remoteFull, "refs/remotes/"))
	}
	if !c.gitRefExists(repoDir, localFull) {
		return true, nil
	}

	result, err := c.runner.Run("-C", repoDir, "merge-base", "--is-ancestor", localFull, remoteFull)
	if err == nil {
		return true, nil
	}
	if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
		return false, nil
	}

	return false, gitutil.WrapGitError("failed to check fast-forward", result, err)
}

func (c *Client) refHash(repoDir string, ref string) string {
	result, err := c.runner.Run("-C", repoDir, "rev-parse", ref)
	if err != nil {
		return ""
	}
	return result.StdoutString(true)
}

func shortHash(hash string) string {
	hash = strings.TrimSpace(hash)
	if len(hash) > 7 {
		return hash[:7]
	}
	if hash == "" {
		return "none"
	}
	return hash
}
