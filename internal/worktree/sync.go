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
	BaseBranch string
	DryRun     bool
}

// syncContext holds resolved sync parameters.
type syncContext struct {
	repoDir      string
	remote       string
	baseName     string
	remoteRef    string
	localFull    string
	remoteFull   string
	baseWorktree string
	status       string
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

	worktrees, err := c.List()
	if err != nil {
		return err
	}
	baseWorktree := findWorktreeForBranch(worktrees, baseName)
	status := ""
	if baseWorktree != "" {
		status = c.GetWorktreeStatus(baseWorktree)
	}

	ctx := syncContext{
		repoDir:      repoDir,
		remote:       remote,
		baseName:     baseName,
		remoteRef:    remoteRef,
		localFull:    localFull,
		remoteFull:   remoteFull,
		baseWorktree: baseWorktree,
		status:       status,
	}

	if opts.DryRun {
		return c.syncDryRun(ctx)
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

	if msg := checkWorktreeReady(baseWorktree, status, baseName); msg != "" {
		fmt.Fprintln(os.Stderr, msg)
		return nil
	}

	if needsUpdate {
		result, err = c.runner.RunLogged("-C", baseWorktree, "reset", "--hard", remoteRef)
		if err != nil {
			return gitutil.WrapGitError("failed to update worktree", result, err)
		}
		fmt.Printf("Synced %s to %s (%s..%s)\n", baseName, remoteRef, shortHash(localHash), shortHash(remoteHash))

		if remote == "upstream" && c.remoteExists(repoDir, "origin") {
			result, err = c.runner.RunLogged("-C", repoDir, "push", "origin", baseName)
			if err != nil {
				msg := strings.TrimSpace(result.StderrString(true))
				if msg == "" {
					msg = err.Error()
				}
				fmt.Fprintf(os.Stderr, "Warning: failed to push origin %s: %s\n", baseName, msg)
			}
		}
	} else {
		fmt.Printf("%s already up to date with %s (%s)\n", baseName, remoteRef, shortHash(localHash))
	}

	return nil
}

func (c *Client) syncDryRun(ctx syncContext) error {
	canFF, err := c.canFastForward(ctx.repoDir, ctx.localFull, ctx.remoteFull)
	if err != nil {
		return err
	}
	if !canFF {
		return fmt.Errorf("base branch '%s' cannot be fast-forwarded to %s", ctx.baseName, ctx.remoteRef)
	}

	localHash := c.refHash(ctx.repoDir, ctx.localFull)
	remoteHash := c.refHash(ctx.repoDir, ctx.remoteFull)
	needsUpdate := localHash == "" || (remoteHash != "" && localHash != remoteHash)

	fmt.Fprintf(os.Stderr, "Would fetch %s\n", ctx.remote)
	if needsUpdate {
		fmt.Fprintf(os.Stderr, "Would fast-forward %s to %s\n", ctx.baseName, ctx.remoteRef)
	} else {
		fmt.Fprintf(os.Stderr, "%s is already up to date with %s\n", ctx.baseName, ctx.remoteRef)
	}

	if ctx.remote == "upstream" && needsUpdate && c.remoteExists(ctx.repoDir, "origin") {
		fmt.Fprintf(os.Stderr, "Would push origin %s\n", ctx.baseName)
	}

	if msg := checkWorktreeReady(ctx.baseWorktree, ctx.status, ctx.baseName); msg != "" {
		fmt.Fprintln(os.Stderr, msg)
		return nil
	}
	if needsUpdate {
		fmt.Fprintf(os.Stderr, "Would update worktree: %s\n", ctx.baseWorktree)
	}

	return nil
}

func checkWorktreeReady(path, status, branch string) string {
	if path == "" {
		return fmt.Sprintf("%s worktree not found, skipped worktree update", branch)
	}
	if status == "modified" {
		return fmt.Sprintf("%s worktree has uncommitted changes, skipped", branch)
	}
	if status != "clean" {
		return fmt.Sprintf("%s worktree status unknown, skipped", branch)
	}
	return ""
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
