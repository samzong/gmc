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

var ghRunFunc = ghRunDefault

func ghRunDefault(repoDir string, args ...string) ([]byte, error) {
	if _, err := exec.LookPath("gh"); err != nil {
		return nil, fmt.Errorf("gh CLI not found: install from https://cli.github.com")
	}
	cmd := exec.Command("gh", args...)
	cmd.Dir = repoDir
	return cmd.Output()
}

func (c *Client) Prune(opts PruneOptions) (PruneResult, error) {
	var result PruneResult

	root, err := c.GetWorktreeRoot()
	if err != nil {
		return result, fmt.Errorf("failed to find worktree root: %w", err)
	}

	baseBranch, err := c.resolveBaseBranch(root, opts.BaseBranch)
	if err != nil {
		return result, err
	}

	candidates, repoDir, err := c.collectPruneCandidates(root, baseBranch, &result.Report)
	if err != nil {
		return result, err
	}

	if opts.PRAware {
		return c.prunePRAware(opts, candidates, repoDir, result)
	}
	return c.pruneClassic(opts, candidates, root, baseBranch, repoDir, result)
}

func (c *Client) collectPruneCandidates(root, baseBranch string, report *Report) ([]pruneCandidate, string, error) {
	baseBranchName := localBranchName(baseBranch)

	worktrees, err := c.List()
	if err != nil {
		return nil, "", err
	}

	repoDir := repoDirForGit(root)
	isBare := repoDir != root

	var candidates []pruneCandidate
	for _, wt := range worktrees {
		if wt.IsBare || filepath.Base(wt.Path) == ".bare" || wt.Path == root {
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

func (c *Client) prunePRAware(opts PruneOptions, candidates []pruneCandidate, repoDir string, result PruneResult) (PruneResult, error) {
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
			status := c.GetWorktreeStatus(cand.wt.Path)
			if status != "clean" && !opts.Force {
				entry.Action = "skipped"
				entry.Reason = "PR merged but worktree has uncommitted changes"
				result.PruneEntries = append(result.PruneEntries, entry)
				continue
			}
			if opts.DryRun {
				entry.Action = "would_remove"
				entry.Reason = "PR merged"
				result.PruneEntries = append(result.PruneEntries, entry)
				continue
			}
			if err := c.removeWorktreeAndBranch(repoDir, cand.wt.Path, cand.wt.Branch, opts.Force, &result.Report); err != nil {
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

func (c *Client) pruneClassic(opts PruneOptions, candidates []pruneCandidate, root, baseBranch, repoDir string, result PruneResult) (PruneResult, error) {
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

		status := c.GetWorktreeStatus(cand.wt.Path)
		if status != "clean" && !opts.Force {
			result.Warn(fmt.Sprintf("Skipped %s: worktree has uncommitted changes (use --force)", cand.name))
			continue
		}

		candidate := PruneCandidate{Name: cand.name, Branch: cand.wt.Branch, Status: status}

		if opts.DryRun {
			result.Warn("Would remove worktree: " + cand.wt.Path)
			result.Warn("  Branch: " + cand.wt.Branch)
			result.Warn("  Status: " + status)
			result.Warn("Would delete branch: " + cand.wt.Branch)
			result.Candidates = append(result.Candidates, candidate)
			prunedAny = true
			continue
		}

		if err := c.removeWorktreeAndBranch(repoDir, cand.wt.Path, cand.wt.Branch, opts.Force, &result.Report); err != nil {
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

func (c *Client) removeWorktreeAndBranch(repoDir, wtPath, branch string, force bool, report *Report) error {
	name := filepath.Base(wtPath)
	args := []string{"-C", repoDir, "worktree", "remove"}
	if force {
		args = append(args, "--force")
	}
	args = append(args, wtPath)

	gitResult, err := c.runner.RunLogged(args...)
	if err != nil {
		return gitutil.WrapGitError("failed to remove worktree", gitResult, err)
	}
	report.Warn(fmt.Sprintf("Removed worktree '%s'", name))

	gitResult, err = c.runner.RunLogged("-C", repoDir, "branch", "-D", branch)
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
