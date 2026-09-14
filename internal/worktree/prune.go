package worktree

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/samzong/gmc/internal/gitutil"
)

type PruneOptions struct {
	BaseBranch string
	Force      bool
	DryRun     bool
	PRAware    bool
	Branches   bool
}

type PruneEntry struct {
	Name    string `json:"name"`
	Branch  string `json:"branch"`
	PRNum   int    `json:"pr_number,omitempty"`
	PRState string `json:"pr_state"`
	Action  string `json:"action"`
	Reason  string `json:"reason"`
}

type PruneCandidate struct {
	Name   string
	Branch string
	Status string
}

type PruneResult struct {
	Report
	Candidates   []PruneCandidate
	PruneEntries []PruneEntry
}

type pruneCandidate struct {
	wt   Info
	name string
}

func (p pruneCandidate) hasWorktree() bool {
	return p.wt.Path != ""
}

var ghRunFunc = ghRunDefault

func ghRunDefault(repoDir string, args ...string) ([]byte, error) {
	if _, err := exec.LookPath("gh"); err != nil {
		return nil, errors.New("gh CLI not found: install from https://cli.github.com")
	}
	cmd := exec.Command("gh", args...)
	cmd.Dir = repoDir
	return cmd.Output()
}

func (c *Client) Prune(opts PruneOptions) (PruneResult, error) {
	var result PruneResult

	if err := c.ensureInit(); err != nil {
		return result, fmt.Errorf("failed to find worktree root: %w", err)
	}
	if !opts.DryRun {
		defer c.InvalidateList()
	}

	baseBranch, err := c.resolveBaseBranch(c.worktreeRoot, opts.BaseBranch)
	if err != nil {
		return result, err
	}

	candidates, repoDir, err := c.collectPruneCandidates(c.worktreeRoot, baseBranch, &result.Report)
	if err != nil {
		return result, err
	}
	if opts.Branches {
		orphans, err := c.collectOrphanBranchCandidates(repoDir, baseBranch, candidates)
		if err != nil {
			return result, err
		}
		candidates = append(candidates, orphans...)
	}

	if opts.PRAware {
		return c.prunePRAware(opts, candidates, repoDir, result)
	}
	return c.pruneClassic(opts, candidates, c.worktreeRoot, baseBranch, repoDir, result)
}

func (c *Client) collectPruneCandidates(root, baseBranch string, report *Report) ([]pruneCandidate, string, error) {
	baseBranchName := localBranchName(baseBranch)

	worktrees, err := c.ListCached()
	if err != nil {
		return nil, "", err
	}

	repoDir := repoDirForGit(root)
	isBare := repoDir != root

	pp, err := c.NewProtectionPolicy()
	if err != nil {
		return nil, "", err
	}
	var candidates []pruneCandidate
	for _, wt := range worktrees {
		if pp.IsProtected(wt) {
			continue
		}
		if isBare && isExternalPath(root, wt.Path) {
			continue
		}
		name := filepath.Base(wt.Path)
		if wt.IsLocked {
			if report != nil {
				report.Warn(fmt.Sprintf("Skipped %s: worktree is locked", name))
			}
			continue
		}
		if wt.Branch == "" || wt.Branch == "(detached)" {
			if report != nil {
				report.Warn(fmt.Sprintf("Skipped %s: detached HEAD", name))
			}
			continue
		}
		if wt.Branch == baseBranchName {
			if report != nil {
				report.Warn(fmt.Sprintf("Skipped %s: base branch '%s'", name, baseBranchName))
			}
			continue
		}
		candidates = append(candidates, pruneCandidate{wt: wt, name: name})
	}

	return candidates, repoDir, nil
}

func (c *Client) collectOrphanBranchCandidates(repoDir, baseBranch string, existing []pruneCandidate) ([]pruneCandidate, error) {
	worktrees, err := c.ListCached()
	if err != nil {
		return nil, err
	}
	pp, err := c.NewProtectionPolicy()
	if err != nil {
		return nil, err
	}

	claimed := make(map[string]struct{}, len(worktrees)+len(existing)+2)
	claimed[localBranchName(baseBranch)] = struct{}{}
	claimed[pp.MainBranch] = struct{}{}
	for _, wt := range worktrees {
		claimed[wt.Branch] = struct{}{}
	}
	for _, cand := range existing {
		claimed[cand.wt.Branch] = struct{}{}
	}

	result, err := c.runner.Run("-C", repoDir, "branch", "--format=%(refname:short)")
	if err != nil {
		return nil, gitutil.WrapGitError("failed to list branches", result, err)
	}

	var orphans []pruneCandidate
	for _, branch := range strings.Split(result.StdoutString(true), "\n") {
		branch = strings.TrimSpace(branch)
		if branch == "" {
			continue
		}
		if _, ok := claimed[branch]; ok {
			continue
		}
		orphans = append(orphans, pruneCandidate{wt: Info{Branch: branch}})
	}
	return orphans, nil
}

type ghPRInfo struct {
	Number      int    `json:"number"`
	State       string `json:"state"`
	HeadRefName string `json:"headRefName"`
}

func ghPRStates(repoDir string) (map[string]ghPRInfo, error) {
	out, err := ghRunFunc(repoDir,
		"pr", "list",
		"--state", "all",
		"--json", "number,state,headRefName",
		"--limit", "300",
	)
	if err != nil {
		return nil, fmt.Errorf("gh pr list failed: %w", err)
	}

	trimmed := strings.TrimSpace(string(out))
	if trimmed == "" || trimmed == "[]" {
		return map[string]ghPRInfo{}, nil
	}

	var prs []ghPRInfo
	if err := json.Unmarshal([]byte(trimmed), &prs); err != nil {
		return nil, fmt.Errorf("failed to parse gh output: %w", err)
	}

	m := make(map[string]ghPRInfo, len(prs))
	for _, pr := range prs {
		pr.State = strings.ToUpper(pr.State)
		if _, exists := m[pr.HeadRefName]; !exists {
			m[pr.HeadRefName] = pr
		}
	}
	return m, nil
}

func (c *Client) prunePRAware(
	opts PruneOptions, candidates []pruneCandidate, repoDir string, result PruneResult,
) (PruneResult, error) {
	prMap, err := ghPRStates(repoDir)
	if err != nil {
		return result, err
	}

	for _, cand := range candidates {
		pr, hasPR := prMap[cand.wt.Branch]

		entry := PruneEntry{
			Name:   cand.name,
			Branch: cand.wt.Branch,
		}

		if hasPR {
			entry.PRNum = pr.Number
			entry.PRState = pr.State
		}

		switch {
		case hasPR && pr.State == "MERGED":
			if cand.hasWorktree() {
				status := c.GetWorktreeStatus(cand.wt.Path)
				if status != "clean" && !opts.Force {
					entry.Action = "skipped"
					entry.Reason = "PR merged but worktree has uncommitted changes"
					result.PruneEntries = append(result.PruneEntries, entry)
					continue
				}
			}
			if opts.DryRun {
				entry.Action = "would_remove"
				entry.Reason = "PR merged"
				result.PruneEntries = append(result.PruneEntries, entry)
				continue
			}
			if err := c.pruneCandidateRefs(repoDir, cand, opts.Force, &result.Report); err != nil {
				return result, err
			}
			entry.Action = "removed"
			entry.Reason = "PR merged"
		case hasPR && pr.State == "CLOSED":
			entry.Action = "skipped"
			entry.Reason = "PR closed, not merged"
		case hasPR && pr.State == "OPEN":
			entry.Action = "skipped"
			entry.Reason = "PR still open"
		default:
			entry.Action = "skipped"
			entry.Reason = "no PR found"
		}

		result.PruneEntries = append(result.PruneEntries, entry)
	}

	if len(result.PruneEntries) == 0 {
		result.Warn("No worktrees to evaluate.")
	}

	return result, nil
}

func (c *Client) pruneClassic(
	opts PruneOptions, candidates []pruneCandidate, root, baseBranch, repoDir string, result PruneResult,
) (PruneResult, error) {
	var prunedAny bool

	for _, cand := range candidates {
		merged, err := c.isBranchMerged(root, cand.wt.Branch, baseBranch)
		if err != nil {
			result.Warn(fmt.Sprintf("Skipped %s: %v", cand.name, err))
			continue
		}
		if !merged {
			continue
		}

		var status string
		if cand.hasWorktree() {
			status = c.GetWorktreeStatus(cand.wt.Path)
			if status != "clean" && !opts.Force {
				result.Warn(fmt.Sprintf("Skipped %s: worktree has uncommitted changes (use --force)", cand.name))
				continue
			}
		}

		candidate := PruneCandidate{Name: cand.name, Branch: cand.wt.Branch, Status: status}

		if opts.DryRun {
			if cand.hasWorktree() {
				result.Warn("Would remove worktree: " + cand.wt.Path)
				result.Warn("  Branch: " + cand.wt.Branch)
				result.Warn("  Status: " + status)
			}
			result.Warn("Would delete branch: " + cand.wt.Branch)
			result.Candidates = append(result.Candidates, candidate)
			prunedAny = true
			continue
		}

		if err := c.pruneCandidateRefs(repoDir, cand, opts.Force, &result.Report); err != nil {
			return result, err
		}
		result.Candidates = append(result.Candidates, candidate)
		prunedAny = true
	}

	if !prunedAny {
		result.Warn("No worktrees pruned.")
	}

	return result, nil
}

func (c *Client) pruneCandidateRefs(repoDir string, cand pruneCandidate, force bool, report *Report) error {
	if cand.hasWorktree() {
		if err := c.removePrunedWorktree(repoDir, cand.wt.Path, force, report); err != nil {
			return err
		}
	}
	return c.deletePrunedBranch(repoDir, cand.wt.Branch, report)
}

func (c *Client) removePrunedWorktree(repoDir, wtPath string, force bool, report *Report) error {
	args := []string{"-C", repoDir, "worktree", "remove"}
	if force {
		args = append(args, "--force")
	}
	args = append(args, wtPath)

	gitResult, err := c.runner.RunLogged(args...)
	if err != nil {
		return gitutil.WrapGitError("failed to remove worktree", gitResult, err)
	}
	report.Warn(fmt.Sprintf("Removed worktree '%s'", filepath.Base(wtPath)))
	return nil
}

func (c *Client) deletePrunedBranch(repoDir, branch string, report *Report) error {
	gitResult, err := c.runner.RunLogged("-C", repoDir, "branch", "-D", branch)
	if err != nil {
		return gitutil.WrapGitError("failed to delete branch", gitResult, err)
	}
	report.Warn(fmt.Sprintf("Deleted branch '%s'", branch))
	return nil
}

func (c *Client) resolveBaseBranch(root string, override string) (string, error) {
	return c.resolveBaseBranchWithPolicy(repoDirForGit(root), override, true)
}

func (c *Client) isBranchMerged(root string, branch string, base string) (bool, error) {
	if branch == "" || base == "" {
		return false, errors.New("branch or base is empty")
	}

	return c.isAncestor(repoDirForGit(root), branch, base)
}

func repoDirForGit(root string) string {
	if root == "" {
		return ""
	}
	bareDir := filepath.Join(root, ".bare")
	if info, err := os.Stat(bareDir); err == nil && info.IsDir() {
		return bareDir
	}
	return root
}

func localBranchName(ref string) string {
	if strings.HasPrefix(ref, "refs/heads/") {
		return strings.TrimPrefix(ref, "refs/heads/")
	}
	if strings.HasPrefix(ref, "refs/remotes/") {
		rest := strings.TrimPrefix(ref, "refs/remotes/")
		parts := strings.SplitN(rest, "/", 2)
		if len(parts) == 2 {
			return parts[1]
		}
		return rest
	}
	if strings.HasPrefix(ref, "origin/") || strings.HasPrefix(ref, "upstream/") {
		parts := strings.SplitN(ref, "/", 2)
		if len(parts) == 2 {
			return parts[1]
		}
	}
	return ref
}
