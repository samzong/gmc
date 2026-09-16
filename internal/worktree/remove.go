package worktree

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/samzong/gmc/internal/gitutil"
)

func (c *Client) previewRemove(target removeContext, opts RemoveOptions, report *Report) {
	status := c.GetWorktreeStatus(target.wtInfo.Path)
	report.Warn("Would remove worktree: " + target.wtInfo.Path)
	report.Warn("  Branch: " + target.wtInfo.Branch)
	report.Warn("  Status: " + status)
	if opts.DeleteBranch && target.wtInfo.Branch != "" && target.wtInfo.Branch != "(detached)" {
		report.Warn("Would delete branch: " + target.wtInfo.Branch)
	}
	if status == "modified" && !opts.Force {
		report.Warn("Note: Worktree has uncommitted changes. Use -f to force removal.")
	}
}

func (c *Client) removeWorktree(path, name string, force bool, report *Report) error {
	args := []string{"-C", c.repoDir, "worktree", "remove"}
	if force {
		args = append(args, "--force")
	}
	result, err := c.runner.RunLogged(append(args, path)...)
	if err != nil {
		return gitutil.WrapGitError("failed to remove worktree", result, err)
	}
	report.Warn(fmt.Sprintf("Removed worktree '%s'", name))
	return nil
}

func (c *Client) deleteBranches(report *Report, branches ...string) error {
	args := append([]string{"-C", c.repoDir, "branch", "-D"}, branches...)
	result, err := c.runner.RunLogged(args...)
	if err != nil {
		return gitutil.WrapGitError("failed to delete branch", result, err)
	}
	for _, branch := range branches {
		report.Warn(fmt.Sprintf("Deleted branch '%s'", branch))
	}
	return nil
}

type RemoveOptions struct {
	Force        bool
	DeleteBranch bool
	DryRun       bool
}

type removeContext struct {
	name   string
	wtInfo Info
}

func (c *Client) Remove(name string, opts RemoveOptions) (Report, error) {
	var report Report

	ctx, err := c.prepareRemove(name)
	if err != nil {
		return report, err
	}

	if opts.DryRun {
		c.previewRemove(ctx, opts, &report)
		return report, nil
	}
	if err := c.removeWorktree(ctx.wtInfo.Path, ctx.name, opts.Force, &report); err != nil {
		return report, err
	}
	if opts.DeleteBranch && ctx.wtInfo.Branch != "" && ctx.wtInfo.Branch != "(detached)" {
		if err := c.deleteBranches(&report, ctx.wtInfo.Branch); err != nil {
			return report, err
		}
	}

	c.InvalidateList()

	return report, nil
}

type RemoveBatchResult struct {
	Succeeded []string
	Failed    map[string]error
	Report    Report
}

func (c *Client) RemoveBatch(names []string, opts RemoveOptions) RemoveBatchResult {
	result := RemoveBatchResult{
		Failed: make(map[string]error),
	}

	if err := c.ensureInit(); err != nil {
		for _, n := range names {
			result.Failed[n] = err
		}
		return result
	}

	worktrees, err := c.ListCached()
	if err != nil {
		for _, n := range names {
			result.Failed[n] = err
		}
		return result
	}

	var targets []removeContext
	for _, name := range names {
		ctx, resolveErr := c.resolveRemoveTarget(name, worktrees)
		if resolveErr != nil {
			result.Failed[name] = resolveErr
			continue
		}
		targets = append(targets, ctx)
	}

	if len(result.Failed) > 0 {
		return result
	}

	type pendingBranch struct {
		branch string
		name   string
	}
	var branchesToDelete []pendingBranch
	for _, t := range targets {
		if opts.DryRun {
			c.previewRemove(t, opts, &result.Report)
			result.Succeeded = append(result.Succeeded, t.name)
			continue
		}
		if err := c.removeWorktree(t.wtInfo.Path, t.name, opts.Force, &result.Report); err != nil {
			result.Failed[t.name] = err
			continue
		}
		result.Succeeded = append(result.Succeeded, t.name)

		if opts.DeleteBranch && t.wtInfo.Branch != "" && t.wtInfo.Branch != "(detached)" {
			branchesToDelete = append(branchesToDelete, pendingBranch{branch: t.wtInfo.Branch, name: t.name})
		}
	}

	if len(branchesToDelete) > 0 {
		branches := make([]string, 0, len(branchesToDelete))
		for _, branch := range branchesToDelete {
			branches = append(branches, branch.branch)
		}
		if err := c.deleteBranches(&result.Report, branches...); err != nil {
			for _, branch := range branchesToDelete {
				result.Failed[branch.name] = err
			}
		}
	}

	if !opts.DryRun && len(result.Succeeded) > 0 {
		c.InvalidateList()
	}

	return result
}

func (c *Client) resolveRemoveTarget(name string, worktrees []Info) (removeContext, error) {
	if name == "" {
		return removeContext{}, errors.New("worktree name cannot be empty")
	}

	targetPath := name
	if !filepath.IsAbs(name) {
		targetPath = filepath.Join(c.searchRoot, name)
	}
	var found bool
	var wtInfo Info
	for _, wt := range worktrees {
		relPath := strings.TrimPrefix(wt.Path, c.searchRoot+string(filepath.Separator))
		if samePath(wt.Path, targetPath) || relPath == name {
			wtInfo = wt
			found = true
			break
		}
	}
	if !found {
		return removeContext{}, fmt.Errorf("worktree not found: %s\nUse 'gmc wt ls' to see available worktrees", name)
	}
	pp, err := c.NewProtectionPolicy()
	if err != nil {
		return removeContext{}, err
	}
	if pp.IsProtected(wtInfo) {
		return removeContext{}, fmt.Errorf("cannot remove protected worktree '%s' (%s)", name, pp.Reason(wtInfo))
	}

	if !pathWithin(c.searchRoot, wtInfo.Path) {
		return removeContext{}, fmt.Errorf("worktree '%s' is external (not managed by gmc wt)", name)
	}

	return removeContext{name: name, wtInfo: wtInfo}, nil
}

func (c *Client) prepareRemove(name string) (removeContext, error) {
	if err := c.ensureInit(); err != nil {
		return removeContext{}, fmt.Errorf("failed to find worktree root: %w", err)
	}

	worktrees, err := c.ListCached()
	if err != nil {
		return removeContext{}, err
	}

	return c.resolveRemoveTarget(name, worktrees)
}
