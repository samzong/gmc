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

func (c *Client) Add(name string, opts AddOptions) (AddResult, error) {
	result := AddResult{CreateResult: CreateResult{Name: name}}
	err := c.add(name, opts, &result)
	result.Err = err
	result.Warnings = result.Report.Warnings()
	return result, err
}

func (c *Client) add(name string, opts AddOptions, result *AddResult) error {
	ctx, err := c.prepareAdd(name, opts)
	result.Path = ctx.targetPath
	result.Branch = ctx.branchName
	if err != nil {
		return err
	}

	if opts.Fetch {
		result.Report.Info("Fetching latest changes...")
		_ = c.runner.RunStreamingLogged("-C", c.repoDir, "fetch", "--all")
	}
	branchExists, created, err := c.createAddedWorktree(ctx, &result.Report)
	result.Created = created
	if created && !branchExists {
		result.Base = ctx.baseBranch
	}
	if err != nil {
		return err
	}
	c.appendAddSummary(&result.Report, ctx, branchExists)
	return nil
}

func (c *Client) createAddedWorktree(ctx addContext, report *Report) (bool, bool, error) {
	args, branchExists := c.addArgs(ctx)
	runResult, err := c.runner.RunLogged(args...)
	if err != nil {
		return branchExists, false, gitutil.WrapGitError("failed to create worktree", runResult, err)
	}
	if err := c.ensureAddedWorktreeConfig(ctx.targetPath); err != nil {
		return branchExists, true, err
	}

	sharedReport, err := c.prepareNewWorktree(ctx.targetPath)
	report.Merge(sharedReport)
	if err != nil {
		report.Warn(fmt.Sprintf("Warning: failed to sync shared resources: %v", err))
	}

	c.InvalidateList()

	return branchExists, true, nil
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

	baseBranch := opts.BaseBranch
	if baseBranch == "" {
		baseBranch = "HEAD"
	}

	ctx := addContext{
		name:       name,
		branchName: branchName,
		targetPath: targetPath,
		baseBranch: baseBranch,
	}
	if _, err := os.Stat(targetPath); err == nil {
		return ctx, fmt.Errorf("directory already exists: %s", targetPath)
	}
	return ctx, nil
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
