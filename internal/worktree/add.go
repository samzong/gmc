package worktree

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/samzong/gmc/internal/gitutil"
)

type AddOptions struct {
	BaseBranch string
	Fetch      bool
	Branch     string
}

type addContext struct {
	name       string
	branchName string
	targetPath string
	baseBranch string
}

func (c *Client) Add(name string, opts AddOptions) (Report, error) {
	var report Report

	ctx, err := c.prepareAdd(name, opts)
	if err != nil {
		return report, err
	}

	if opts.Fetch {
		report.Info("Fetching latest changes...")
		_ = c.runner.RunStreamingLogged("-C", c.repoDir, "fetch", "--all")
	}
	branchExists, err := c.createAddedWorktree(ctx, &report)
	if err != nil {
		return report, err
	}
	c.appendAddSummary(&report, ctx, branchExists)
	return report, nil
}

func (c *Client) createAddedWorktree(ctx addContext, report *Report) (bool, error) {
	args, branchExists := c.addArgs(ctx)
	result, err := c.runner.RunLogged(args...)
	if err != nil {
		return false, gitutil.WrapGitError("failed to create worktree", result, err)
	}
	if err := c.ensureAddedWorktreeConfig(ctx.targetPath); err != nil {
		return false, err
	}

	sharedReport, err := c.prepareNewWorktree(ctx.targetPath)
	report.Merge(sharedReport)
	if err != nil {
		report.Warn(fmt.Sprintf("Warning: failed to sync shared resources: %v", err))
	}

	c.InvalidateList()

	return branchExists, nil
}

func (c *Client) prepareAdd(name string, opts AddOptions) (addContext, error) {
	if name == "" {
		return addContext{}, errors.New("worktree name cannot be empty")
	}
	branchName := name
	if opts.Branch != "" {
		branchName = opts.Branch
	}
	if err := gitutil.ValidateBranchName(branchName); err != nil {
		return addContext{}, err
	}

	if err := c.ensureInit(); err != nil {
		return addContext{}, fmt.Errorf("failed to find worktree root: %w", err)
	}

	dirName := strings.ReplaceAll(name, "/", "--")

	var targetPath string
	if c.repoDir != c.worktreeRoot {
		targetPath = filepath.Join(c.worktreeRoot, dirName)
	} else {
		targetPath = filepath.Join(filepath.Dir(c.worktreeRoot), filepath.Base(c.worktreeRoot)+"--"+dirName)
	}

	if _, err := os.Stat(targetPath); err == nil {
		return addContext{}, fmt.Errorf("directory already exists: %s", targetPath)
	}

	baseBranch := opts.BaseBranch
	if baseBranch == "" {
		baseBranch = "HEAD"
	}

	return addContext{
		name:       name,
		branchName: branchName,
		targetPath: targetPath,
		baseBranch: baseBranch,
	}, nil
}

func (c *Client) addArgs(ctx addContext) ([]string, bool) {
	branchExists := c.gitRefExists(c.repoDir, "refs/heads/"+ctx.branchName)
	if branchExists {
		return []string{"-C", c.repoDir, "worktree", "add", ctx.targetPath, ctx.branchName}, true
	}
	return []string{
		"-C", c.repoDir, "worktree", "add", "-b", ctx.branchName, ctx.targetPath, ctx.baseBranch,
	}, false
}

func (c *Client) ensureAddedWorktreeConfig(targetPath string) error {
	if c.getGitOutput(c.repoDir, "config", "--local", "--bool", "extensions.worktreeConfig") != "true" {
		return nil
	}

	result, err := c.runner.RunLogged("-C", targetPath, "config", "--worktree", "core.bare", "false")
	if err != nil {
		return gitutil.WrapGitError("failed to configure worktree", result, err)
	}
	return nil
}

func (c *Client) appendAddSummary(report *Report, ctx addContext, branchExists bool) {
	report.Info(fmt.Sprintf("Created worktree '%s' at %s", ctx.name, ctx.targetPath))
	if branchExists {
		report.Info(fmt.Sprintf("Branch: %s (existing)", ctx.branchName))
	} else {
		report.Info(fmt.Sprintf("Branch: %s (based on %s)", ctx.branchName, ctx.baseBranch))
	}
	report.Info("Next step: cd " + ctx.targetPath)
}
