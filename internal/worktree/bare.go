package worktree

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

type CloneOptions struct {
	Name     string // Custom project name
	Upstream string // Upstream URL for fork workflow
}

func (c *Client) Clone(repoURL string, opts CloneOptions) (Report, error) {
	var report Report

	if repoURL == "" {
		return report, errors.New("repository URL cannot be empty")
	}

	projectName := opts.Name
	if projectName == "" {
		var err error
		projectName, err = extractProjectName(repoURL)
		if err != nil {
			return report, err
		}
	}

	if _, err := os.Stat(projectName); err == nil {
		return report, fmt.Errorf("directory already exists: %s", projectName)
	}

	report.Info(fmt.Sprintf("Cloning %s as bare + worktree structure...", repoURL))
	report.Info("")

	if err := os.MkdirAll(projectName, 0755); err != nil {
		return report, fmt.Errorf("failed to create directory: %w", err)
	}

	bareDir := filepath.Join(projectName, ".bare")

	args := []string{"clone", "--bare", "--progress", repoURL, bareDir}
	if err := c.runner.RunStreamingLogged(args...); err != nil {
		os.RemoveAll(projectName)
		return report, fmt.Errorf("failed to clone repository: %w", err)
	}
	report.Info("")

	defaultBranch, err := c.getDefaultBranch(bareDir)
	if err != nil {
		defaultBranch = "main"
	}

	absProjectDir, err := filepath.Abs(projectName)
	if err != nil {
		os.RemoveAll(projectName)
		return report, fmt.Errorf("failed to get absolute path: %w", err)
	}
	mainWorktree := filepath.Join(absProjectDir, defaultBranch)
	args = []string{"-C", bareDir, "worktree", "add", mainWorktree, defaultBranch}
	if err := c.runner.RunStreamingLogged(args...); err != nil {
		os.RemoveAll(projectName)
		return report, fmt.Errorf("failed to create main worktree: %w", err)
	}

	configReport, err := c.configureBareRepo(bareDir, opts.Upstream)
	report.Merge(configReport)
	if err != nil {
		os.RemoveAll(projectName)
		return report, err
	}

	report.Info("")
	report.Info(fmt.Sprintf("Successfully cloned to %s/", projectName))
	report.Info("")
	report.Info("Directory Structure:")
	report.Info(fmt.Sprintf("  %s/", projectName))
	report.Info("  ├── .bare/          # Bare repository")
	report.Info(fmt.Sprintf("  └── %s/           # Main worktree", defaultBranch))
	report.Info("")

	if opts.Upstream != "" {
		report.Info("Remote Configuration:")
		report.Info("  origin   = " + repoURL + " (your fork)")
		report.Info("  upstream = " + opts.Upstream + " (upstream)")
		report.Info("")
	}

	report.Info("Next steps:")
	report.Info(fmt.Sprintf("  cd %s/%s", projectName, defaultBranch))
	report.Info("  gmc wt add feature-name")

	return report, nil
}

func (c *Client) configureBareRepo(bareDir string, upstreamURL string) (Report, error) {
	var report Report

	if err := c.gitConfig(bareDir, "remote.origin.fetch", "+refs/heads/*:refs/remotes/origin/*"); err != nil {
		return report, fmt.Errorf("failed to configure remote.origin.fetch: %w", err)
	}

	if c.verbose {
		report.Warn("Fetching remote references...")
	}
	_, err := c.runner.Run("-C", bareDir, "fetch", "origin")
	if err != nil && c.verbose {
		report.Warn(fmt.Sprintf("Warning: 'git fetch origin' failed: %v", err))
	}

	if upstreamURL != "" {
		args := []string{"-C", bareDir, "remote", "add", "upstream", upstreamURL}
		if _, err := c.runner.RunLogged(args...); err != nil {
			return report, fmt.Errorf("failed to add upstream remote: %w", err)
		}
	}

	return report, nil
}

func (c *Client) gitConfig(repoDir string, key string, value string) error {
	args := []string{"-C", repoDir, "config", key, value}
	_, err := c.runner.RunLogged(args...)
	return err
}

func (c *Client) getDefaultBranch(bareDir string) (string, error) {
	args := []string{"-C", bareDir, "symbolic-ref", "--short", "HEAD"}
	result, err := c.runner.Run(args...)
	if err == nil {
		branch := result.StdoutString(true)
		if branch != "" {
			return branch, nil
		}
	}

	for _, branch := range []string{"main", "master"} {
		args := []string{"-C", bareDir, "rev-parse", "--verify", "refs/heads/" + branch}
		if _, err := c.runner.Run(args...); err == nil {
			return branch, nil
		}
	}

	return "", errors.New("could not determine default branch")
}

func extractProjectName(repoURL string) (string, error) {
	if strings.Contains(repoURL, "@") && strings.Contains(repoURL, ":") {
		parts := strings.Split(repoURL, ":")
		if len(parts) == 2 {
			path := parts[1]
			return cleanProjectName(path), nil
		}
	}

	parsed, err := url.Parse(repoURL)
	if err != nil {
		return "", fmt.Errorf("invalid repository URL: %w", err)
	}

	path := parsed.Path
	return cleanProjectName(path), nil
}

func cleanProjectName(path string) string {
	name := strings.TrimSuffix(path, ".git")
	name = filepath.Base(name)
	name = strings.TrimPrefix(name, "/")
	return name
}
