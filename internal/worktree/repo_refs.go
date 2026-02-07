package worktree

import (
	"errors"
	"strings"
)

func (c *Client) resolveBaseBranchWithPolicy(repoDir string, override string, allowHeadFallback bool) (string, error) {
	if override != "" {
		return override, nil
	}
	if repoDir == "" {
		return "", errors.New("cannot determine base branch: repository root is empty")
	}

	candidates := []string{
		"refs/remotes/origin/HEAD",
		"refs/remotes/upstream/HEAD",
	}
	if allowHeadFallback {
		candidates = append(candidates, "HEAD")
	}
	for _, ref := range candidates {
		if resolved := c.gitSymbolicRef(repoDir, ref); resolved != "" {
			return resolved, nil
		}
	}

	for _, branch := range []string{"main", "master"} {
		if c.gitRefExists(repoDir, "refs/heads/"+branch) {
			return branch, nil
		}
	}

	return "", errors.New("could not determine base branch; specify with --base")
}

func (c *Client) remoteExists(repoDir string, name string) bool {
	result, err := c.runner.Run("-C", repoDir, "remote", "get-url", name)
	return err == nil && strings.TrimSpace(result.StdoutString(true)) != ""
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
