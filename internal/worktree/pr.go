package worktree

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/samzong/gmc/internal/gitutil"
)

// DetectPRRemote auto-detects the best remote for fetching PRs
func (c *Client) DetectPRRemote(repoDir string) (string, error) {
	result, err := c.runner.Run("-C", repoDir, "remote")
	if err != nil {
		return "", fmt.Errorf("failed to list remotes: %w", err)
	}

	remotes := strings.Fields(result.StdoutString(true))
	if len(remotes) == 0 {
		return "", errors.New("no git remotes found")
	}

	// Prefer upstream, then origin, then single remote
	preferences := []string{"upstream", "origin"}
	for _, preferred := range preferences {
		for _, remote := range remotes {
			if remote == preferred {
				return remote, nil
			}
		}
	}

	if len(remotes) == 1 {
		return remotes[0], nil
	}
	return "", fmt.Errorf("multiple remotes found (%v) but no 'upstream' or 'origin'. Use --remote to specify", remotes)
}

// PRExists checks if a PR exists on the remote
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

// AddPR creates a worktree from a Pull Request
func (c *Client) AddPR(prNumber int, remote string) (Report, error) {
	var report Report

	// Setup paths to get repoDir
	root, err := c.GetWorktreeRoot()
	if err != nil {
		return report, fmt.Errorf("failed to find worktree root: %w", err)
	}
	repoDir := repoDirForGit(root)

	// Auto-detect remote if not specified
	if remote == "" {
		detectedRemote, err := c.DetectPRRemote(repoDir)
		if err != nil {
			return report, err
		}
		remote = detectedRemote
		report.Info("Auto-detected remote: " + remote)
	}

	// Verify PR exists
	exists, commitHash, err := c.PRExists(prNumber, remote, repoDir)
	if err != nil {
		return report, err
	}
	if !exists {
		return report, fmt.Errorf("PR #%d not found on remote '%s'", prNumber, remote)
	}

	// Prepare worktree paths
	branchName := fmt.Sprintf("pr-%d", prNumber)
	targetPath := filepath.Join(root, branchName)

	if _, err := os.Stat(targetPath); err == nil {
		return report, fmt.Errorf("directory already exists: %s", targetPath)
	}

	// Fetch PR from remote
	refSpec := fmt.Sprintf("pull/%d/head:%s", prNumber, branchName)
	report.Info(fmt.Sprintf("Fetching PR #%d from %s...", prNumber, remote))

	fetchArgs := []string{"-C", repoDir, "fetch", remote, refSpec}
	result, err := c.runner.RunLogged(fetchArgs...)
	if err != nil {
		return report, gitutil.WrapGitError("failed to fetch PR", result, err)
	}

	// Create worktree
	addArgs := []string{"-C", repoDir, "worktree", "add", targetPath, branchName}
	result, err = c.runner.RunLogged(addArgs...)
	if err != nil {
		return report, gitutil.WrapGitError("failed to create worktree", result, err)
	}

	// Sync shared resources
	sharedReport, err := c.SyncSharedResources(branchName)
	report.Merge(sharedReport)
	if err != nil {
		report.Warn(fmt.Sprintf("Warning: failed to sync shared resources: %v", err))
	}

	report.Info(fmt.Sprintf("Created PR worktree '%s' at %s", branchName, targetPath))
	report.Info("Commit: " + commitHash[:7])
	report.Info("Next step: cd " + targetPath)

	return report, nil
}
