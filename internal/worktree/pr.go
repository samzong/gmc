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
func (c *Client) DetectPRRemote() (string, error) {
	result, err := c.runner.Run("remote")
	if err != nil {
		return "", fmt.Errorf("failed to list remotes: %w", err)
	}

	remotes := strings.Fields(result.StdoutString(true))
	if len(remotes) == 0 {
		return "", errors.New("no git remotes found")
	}

	// 1. Prefer upstream
	for _, remote := range remotes {
		if remote == "upstream" {
			return remote, nil
		}
	}

	// 2. Fallback to origin
	for _, remote := range remotes {
		if remote == "origin" {
			return remote, nil
		}
	}

	// 3. If only one remote, use it
	if len(remotes) == 1 {
		return remotes[0], nil
	}

	// 4. Multiple remotes but no common names
	return "", fmt.Errorf("multiple remotes found (%v) but no 'upstream' or 'origin'. Use --remote to specify", remotes)
}

// PRExists checks if a PR exists on the remote
func (c *Client) PRExists(prNumber int, remote string) (bool, string, error) {
	refPath := fmt.Sprintf("refs/pull/%d/head", prNumber)

	result, err := c.runner.Run("ls-remote", remote, refPath)
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
func (c *Client) AddPR(prNumber int, remote string) error {
	// 1. Auto-detect remote if not specified
	if remote == "" {
		detectedRemote, err := c.DetectPRRemote()
		if err != nil {
			return err
		}
		remote = detectedRemote
		fmt.Printf("Auto-detected remote: %s\n", remote)
	}

	// 2. Check PR exists
	exists, commitHash, err := c.PRExists(prNumber, remote)
	if err != nil {
		return err
	}
	if !exists {
		return fmt.Errorf("PR #%d not found on remote '%s'", prNumber, remote)
	}

	// 3. Setup paths
	branchName := fmt.Sprintf("pr-%d", prNumber)

	root, err := c.GetWorktreeRoot()
	if err != nil {
		return fmt.Errorf("failed to find worktree root: %w", err)
	}
	repoDir := repoDirForGit(root)
	targetPath := filepath.Join(root, branchName)

	// 4. Check directory doesn't exist
	if _, err := os.Stat(targetPath); err == nil {
		return fmt.Errorf("directory already exists: %s", targetPath)
	}

	// 5. Fetch PR
	refSpec := fmt.Sprintf("pull/%d/head:%s", prNumber, branchName)
	fmt.Printf("Fetching PR #%d from %s...\n", prNumber, remote)

	fetchArgs := []string{"-C", repoDir, "fetch", remote, refSpec}
	result, err := c.runner.RunLogged(fetchArgs...)
	if err != nil {
		return gitutil.WrapGitError("failed to fetch PR", result, err)
	}

	// 6. Create worktree
	addArgs := []string{"-C", repoDir, "worktree", "add", targetPath, branchName}
	result, err = c.runner.RunLogged(addArgs...)
	if err != nil {
		return gitutil.WrapGitError("failed to create worktree", result, err)
	}

	// 7. Sync shared resources
	if err := c.SyncSharedResources(branchName); err != nil {
		fmt.Printf("Warning: failed to sync shared resources: %v\n", err)
	}

	fmt.Printf("Created PR worktree '%s' at %s\n", branchName, targetPath)
	fmt.Printf("Commit: %s\n", commitHash[:7])
	fmt.Printf("Next step: cd %s\n", targetPath)

	return nil
}
