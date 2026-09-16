package cmd

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/samzong/gmc/internal/worktree"
)

func runWorktreeAdd(wtClient *worktree.Client, names []string) error {
	if wtAddPR > 0 {
		return runWorktreeAddPR(wtClient, wtAddPR)
	}

	baseBranch := wtBaseBranch
	if wtAddSync {
		if baseBranch == "" {
			resolved, err := wtClient.ResolveSyncBaseBranch("")
			if err != nil {
				return err
			}
			baseBranch = resolved
		}
		syncOpts := worktree.SyncOptions{
			BaseBranch: baseBranch,
			DryRun:     false,
		}
		report, err := wtClient.Sync(syncOpts)
		printWorktreeReport(report)
		if err != nil {
			return err
		}
	}
	opts := worktree.AddOptions{
		BaseBranch: baseBranch,
		Fetch:      false,
	}
	var failed []string
	for _, name := range names {
		report, err := wtClient.Add(name, opts)
		printWorktreeReport(report)
		if err != nil {
			fmt.Fprintf(errWriter(), "Error adding '%s': %v\n", name, err)
			failed = append(failed, name)
		}
	}
	if len(failed) > 0 {
		return fmt.Errorf("failed to add worktrees: %s", strings.Join(failed, ", "))
	}
	return nil
}

func runWorktreeAddPR(wtClient *worktree.Client, prNumber int) error {
	report, err := wtClient.AddPR(prNumber, "")
	printWorktreeReport(report)
	return err
}

func runWorktreeRemove(wtClient *worktree.Client, names []string) error {
	if wtAll {
		resolved, err := resolveAllRemovableWorktrees(wtClient)
		if err != nil {
			return err
		}
		if len(resolved) == 0 {
			fmt.Fprintln(outWriter(), "No removable worktrees found.")
			return nil
		}
		names = resolved
	}

	opts := worktree.RemoveOptions{
		Force:        wtForce,
		DeleteBranch: wtDeleteBranch,
		DryRun:       wtDryRun,
	}

	result := wtClient.RemoveBatch(names, opts)
	printWorktreeReport(result.Report)

	var failed []string
	for _, name := range names {
		if err, ok := result.Failed[name]; ok {
			fmt.Fprintf(errWriter(), "Error removing '%s': %v\n", name, err)
			failed = append(failed, name)
		}
	}

	if len(failed) > 0 {
		return fmt.Errorf("failed to remove worktrees: %s", strings.Join(failed, ", "))
	}
	return nil
}

func resolveAllRemovableWorktrees(wtClient *worktree.Client) ([]string, error) {
	all, err := wtClient.List()
	if err != nil {
		return nil, err
	}

	pp, err := wtClient.NewProtectionPolicy()
	if err != nil {
		return nil, err
	}
	root := getDisplayRoot(wtClient)
	var names []string
	for _, wt := range all {
		if pp.IsProtected(wt) {
			continue
		}
		if isExternalWorktree(root, wt.Path) || isAgentWorktree(wt.Path) {
			continue
		}
		names = append(names, displayWorktreeName(root, wt.Path))
	}
	return names, nil
}

func runWorktreeClone(wtClient *worktree.Client, url string) error {
	opts := worktree.CloneOptions{
		Name:     wtProjectName,
		Upstream: wtUpstream,
	}
	report, err := wtClient.Clone(url, opts)
	printWorktreeReport(report)
	return err
}

func runWorktreeDup(wtClient *worktree.Client, args []string) error {
	opts := worktree.DupOptions{
		BaseBranch: wtDupBase,
		Count:      2,
		TaskFiles:  wtDupTasks,
	}

	if len(args) > 0 {
		count, err := strconv.Atoi(args[0])
		if err != nil {
			return fmt.Errorf("invalid count: %s", args[0])
		}
		opts.Count = count
	}

	result, err := wtClient.Dup(opts)
	if err != nil {
		return err
	}
	for _, warning := range result.Warnings {
		fmt.Fprintln(errWriter(), warning)
	}

	fmt.Fprintf(outWriter(), "Created %d worktrees based on '%s':\n", len(result.Worktrees), result.BaseBranch)
	for i, wt := range result.Worktrees {
		relPath := wt
		if i < len(result.RelativePaths) && result.RelativePaths[i] != "" {
			relPath = result.RelativePaths[i]
		}
		absPath := ""
		if i < len(result.WorktreePaths) {
			absPath = result.WorktreePaths[i]
		}
		if absPath == "" {
			fmt.Fprintf(outWriter(), "  %s -> %s\n", relPath, result.Branches[i])
		} else {
			fmt.Fprintf(outWriter(), "  %s (%s) -> %s\n", relPath, absPath, result.Branches[i])
		}
	}
	if len(result.TaskFiles) > 0 {
		fmt.Fprintln(outWriter(), "Copied task files:")
		for _, task := range result.TaskFiles {
			fmt.Fprintf(outWriter(), "  %s\n", task)
		}
	}
	fmt.Fprintln(outWriter())
	fmt.Fprintln(outWriter(), "Next steps:")
	fmt.Fprintln(outWriter(), "  1. Work in each directory with different AI tools")
	fmt.Fprintln(outWriter(), "  2. Evaluate and pick the best solution")
	fmt.Fprintf(outWriter(), "  3. Dry-run promote: gmc wt promote <candidate> --dry-run\n")
	fmt.Fprintf(outWriter(), "  4. Promote winner: gmc wt promote <candidate>\n")
	fmt.Fprintln(outWriter(), "  5. Clean up: gmc wt rm <other-worktrees> -D")

	return nil
}

func runWorktreePromote(wtClient *worktree.Client, candidate string) error {
	report, err := wtClient.Promote(candidate, worktree.PromoteOptions{
		DryRun: wtDryRun,
	})
	printWorktreeReport(report)
	return err
}
