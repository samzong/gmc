package worktree

import (
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/samzong/gmc/internal/gitutil"
	"github.com/samzong/gmc/internal/stringsutil"
)

type SyncOptions struct {
	BaseBranch string
	DryRun     bool
}

func (c *Client) ResolveSyncBaseBranch(override string) (string, error) {
	if err := c.ensureInit(); err != nil {
		return "", fmt.Errorf("failed to find worktree root: %w", err)
	}
	return c.resolveSyncBaseBranch(c.repoDir, override)
}

func (c *Client) Sync(opts SyncOptions) (Report, error) {
	var report Report

	if err := c.ensureInit(); err != nil {
		return report, fmt.Errorf("failed to find worktree root: %w", err)
	}

	repoDir := c.repoDir
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

	worktrees, err := c.ListCached()
	if err != nil {
		return report, err
	}
	baseWorktree := findWorktreeForBranch(worktrees, baseName)
	status := ""
	if baseWorktree != "" {
		status = c.GetWorktreeStatus(baseWorktree)
	}

	if !opts.DryRun {
		result, err := c.runner.RunLogged("-C", repoDir, "fetch", remote)
		if err != nil {
			return report, gitutil.WrapGitError("failed to fetch "+remote, result, err)
		}
	}

	canFF, err := c.canFastForward(repoDir, localFull, remoteFull)
	if err != nil {
		return report, err
	}
	if !canFF {
		return report, fmt.Errorf("base branch '%s' cannot be fast-forwarded to %s", baseName, remoteRef)
	}

	localHash := c.getGitOutput(repoDir, "rev-parse", localFull)
	remoteHash := c.getGitOutput(repoDir, "rev-parse", remoteFull)
	needsUpdate := localHash == "" || (remoteHash != "" && localHash != remoteHash)

	if opts.DryRun {
		report.Warn("Would fetch " + remote)
		if needsUpdate {
			report.Warn(fmt.Sprintf("Would fast-forward %s to %s", baseName, remoteRef))
		} else {
			report.Warn(fmt.Sprintf("%s is already up to date with %s", baseName, remoteRef))
		}
		if remote == "upstream" && needsUpdate && c.remoteExists(repoDir, "origin") {
			report.Warn("Would push origin " + baseName)
		}
	}

	if msg := checkWorktreeReady(baseWorktree, status, baseName); msg != "" {
		report.Warn(msg)
		return report, nil
	}

	if opts.DryRun {
		if needsUpdate {
			report.Warn("Would update worktree: " + baseWorktree)
		}
		return report, nil
	}

	if needsUpdate {
		result, err := c.runner.RunLogged("-C", baseWorktree, "reset", "--hard", remoteRef)
		if err != nil {
			return report, gitutil.WrapGitError("failed to update worktree", result, err)
		}
		localShort := stringsutil.ShortHash(localHash, 7, "none")
		remoteShort := stringsutil.ShortHash(remoteHash, 7, "none")
		report.Info(fmt.Sprintf("Synced %s to %s (%s..%s)", baseName, remoteRef, localShort, remoteShort))

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
		localShort := stringsutil.ShortHash(localHash, 7, "none")
		report.Info(fmt.Sprintf("%s already up to date with %s (%s)", baseName, remoteRef, localShort))
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

	return c.isAncestor(repoDir, localFull, remoteFull)
}

func (c *Client) isAncestor(repoDir string, commitA, commitB string) (bool, error) {
	result, err := c.runner.Run("-C", repoDir, "merge-base", "--is-ancestor", commitA, commitB)
	if err == nil {
		return true, nil
	}
	if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
		return false, nil
	}

	return false, gitutil.WrapGitError("failed to check ancestry", result, err)
}
