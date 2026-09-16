package worktree

import (
	"errors"
	"fmt"
	"strings"

	"github.com/samzong/gmc/internal/gitutil"
)

func (c *Client) DetectPRRemote(repoDir string) (string, error) {
	result, err := c.runner.Run("-C", repoDir, "remote")
	if err != nil {
		return "", fmt.Errorf("failed to list remotes: %w", err)
	}

	remotes := strings.Fields(result.StdoutString(true))
	if len(remotes) == 0 {
		return "", errors.New("no git remotes found")
	}

	if candidates := reviewRemoteCandidates(remotes); len(candidates) > 0 {
		return candidates[0], nil
	}
	return "", fmt.Errorf("multiple remotes found (%v) but no 'upstream' or 'origin'", remotes)
}

func (c *Client) PRExists(prNumber int, remote, repoDir string) (bool, string, error) {
	refPath := fmt.Sprintf("refs/pull/%d/head", prNumber)

	result, err := c.runner.Run("-C", repoDir, "ls-remote", remote, refPath)
	if err != nil {
		return false, "", fmt.Errorf("failed to query remote: %w", err)
	}

	output := result.StdoutString(true)
	if output == "" {
		return false, "", nil
	}

	parts := strings.Fields(output)
	if len(parts) < 2 {
		return false, "", errors.New("unexpected ls-remote output")
	}

	return true, parts[0], nil
}

func (c *Client) AddPR(prNumber int, remote string) (Report, error) {
	var report Report

	if err := c.ensureInit(); err != nil {
		return report, fmt.Errorf("failed to find worktree root: %w", err)
	}
	repoDir := c.repoDir

	if remote == "" {
		detectedRemote, err := c.DetectPRRemote(repoDir)
		if err != nil {
			return report, err
		}
		remote = detectedRemote
		report.Info("Auto-detected remote: " + remote)
	}

	exists, commitHash, err := c.PRExists(prNumber, remote, repoDir)
	if err != nil {
		return report, err
	}
	if !exists {
		return report, fmt.Errorf("PR #%d not found on remote '%s'", prNumber, remote)
	}

	branchName := fmt.Sprintf("pr/%d", prNumber)
	ctx, err := c.prepareAdd(branchName, AddOptions{})
	if err != nil {
		return report, err
	}

	refSpec := fmt.Sprintf("pull/%d/head:%s", prNumber, branchName)
	report.Info(fmt.Sprintf("Fetching PR #%d from %s...", prNumber, remote))

	result, err := c.runner.RunLogged("-C", c.repoDir, "fetch", remote, refSpec)
	if err != nil {
		return report, gitutil.WrapGitError("failed to fetch PR", result, err)
	}

	if _, err := c.createAddedWorktree(ctx, &report); err != nil {
		return report, err
	}

	report.Info(fmt.Sprintf("Created PR worktree '%s' at %s", branchName, ctx.targetPath))
	report.Info("Commit: " + commitHash[:7])
	report.Info("Next step: cd " + ctx.targetPath)

	return report, nil
}
