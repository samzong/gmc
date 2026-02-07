package worktree

import (
	"errors"
	"fmt"
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
func (c *Client) Sync(opts SyncOptions) (Report, error) {
	var report Report

	root, err := c.GetWorktreeRoot()
	if err != nil {
		return report, fmt.Errorf("failed to find worktree root: %w", err)
	}

	repoDir := repoDirForGit(root)
	remote, err := c.selectSyncRemote(repoDir)
	if err != nil {
		return report, err
	}

	baseRef, err := c.resolveSyncBaseBranch(repoDir, opts.BaseBranch)
	if err != nil {
		return report, err
	}
	baseName := localBranchName(baseRef)

	remoteRef := fmt.Sprintf("%s/%s", remote, baseName)
	remoteFull := "refs/remotes/" + remoteRef
	localFull := "refs/heads/" + baseName

	worktrees, err := c.List()
	if err != nil {
		return report, err
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
		return report, gitutil.WrapGitError("failed to fetch "+remote, result, err)
	}

	canFF, err := c.canFastForward(repoDir, localFull, remoteFull)
	if err != nil {
		return report, err
	}
	if !canFF {
		return report, fmt.Errorf("base branch '%s' cannot be fast-forwarded to %s", baseName, remoteRef)
	}

	localHash := c.refHash(repoDir, localFull)
	remoteHash := c.refHash(repoDir, remoteFull)
	needsUpdate := localHash == "" || (remoteHash != "" && localHash != remoteHash)

	if msg := checkWorktreeReady(baseWorktree, status, baseName); msg != "" {
		report.Warn(msg)
		return report, nil
	}

	if needsUpdate {
		result, err = c.runner.RunLogged("-C", baseWorktree, "reset", "--hard", remoteRef)
		if err != nil {
			return report, gitutil.WrapGitError("failed to update worktree", result, err)
		}
		report.Info(fmt.Sprintf("Synced %s to %s (%s..%s)", baseName, remoteRef, shortHash(localHash), shortHash(remoteHash)))

		if remote == "upstream" && c.remoteExists(repoDir, "origin") {
			result, err = c.runner.RunLogged("-C", repoDir, "push", "origin", baseName)
			if err != nil {
				msg := strings.TrimSpace(result.StderrString(true))
				if msg == "" {
					msg = err.Error()
				}
				report.Warn(fmt.Sprintf("Warning: failed to push origin %s: %s", baseName, msg))
			}
		}
	} else {
		report.Info(fmt.Sprintf("%s already up to date with %s (%s)", baseName, remoteRef, shortHash(localHash)))
	}

	return report, nil
}

func (c *Client) syncDryRun(ctx syncContext) (Report, error) {
	var report Report

	canFF, err := c.canFastForward(ctx.repoDir, ctx.localFull, ctx.remoteFull)
	if err != nil {
		return report, err
	}
	if !canFF {
		return report, fmt.Errorf("base branch '%s' cannot be fast-forwarded to %s", ctx.baseName, ctx.remoteRef)
	}

	localHash := c.refHash(ctx.repoDir, ctx.localFull)
	remoteHash := c.refHash(ctx.repoDir, ctx.remoteFull)
	needsUpdate := localHash == "" || (remoteHash != "" && localHash != remoteHash)

	report.Warn("Would fetch " + ctx.remote)
	if needsUpdate {
		report.Warn(fmt.Sprintf("Would fast-forward %s to %s", ctx.baseName, ctx.remoteRef))
	} else {
		report.Warn(fmt.Sprintf("%s is already up to date with %s", ctx.baseName, ctx.remoteRef))
	}

	if ctx.remote == "upstream" && needsUpdate && c.remoteExists(ctx.repoDir, "origin") {
		report.Warn("Would push origin " + ctx.baseName)
	}

	if msg := checkWorktreeReady(ctx.baseWorktree, ctx.status, ctx.baseName); msg != "" {
		report.Warn(msg)
		return report, nil
	}
	if needsUpdate {
		report.Warn("Would update worktree: " + ctx.baseWorktree)
	}

	return report, nil
}

func (c *Client) resolveSyncBaseBranch(repoDir string, override string) (string, error) {
	return c.resolveBaseBranchWithPolicy(repoDir, override, false)
}

func checkWorktreeReady(path, status, branch string) string {
	if path == "" {
		return branch + " worktree not found, skipped worktree update"
	}
	if status == "modified" {
		return branch + " worktree has uncommitted changes, skipped"
	}
	if status != "clean" {
		return branch + " worktree status unknown, skipped"
	}
	return ""
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
