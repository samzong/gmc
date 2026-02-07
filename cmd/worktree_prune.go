package cmd

import (
	"github.com/samzong/gmc/internal/worktree"
	"github.com/spf13/cobra"
)

var (
	wtPruneBase   string
	wtPruneForce  bool
	wtPruneDryRun bool
)

var wtPruneCmd = &cobra.Command{
	Use:   "prune",
	Short: "Remove worktrees whose branches are merged",
	Long: `Remove worktrees whose branches are already merged into the base branch.

This command uses pure git ancestry checks to decide which worktrees are safe to remove.
By default it removes both the worktree directory and the local branch.`,
	RunE: func(_ *cobra.Command, _ []string) error {
		wtClient := newWorktreeClient()
		return runWorktreePrune(wtClient)
	},
}

func runWorktreePrune(wtClient *worktree.Client) error {
	opts := worktree.PruneOptions{
		BaseBranch: wtPruneBase,
		Force:      wtPruneForce,
		DryRun:     wtPruneDryRun,
	}
	report, err := wtClient.Prune(opts)
	printWorktreeReport(report)
	return err
}
