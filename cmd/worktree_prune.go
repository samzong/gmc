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

type PruneJSON struct {
	Name   string `json:"name"`
	Branch string `json:"branch"`
	Status string `json:"status"`
	Action string `json:"action"`
}

func runWorktreePrune(wtClient *worktree.Client) error {
	opts := worktree.PruneOptions{
		BaseBranch: wtPruneBase,
		Force:      wtPruneForce,
		DryRun:     wtPruneDryRun,
	}
	result, err := wtClient.Prune(opts)
	if err != nil {
		return err
	}
	if outputFormat() == "json" {
		action := "removed"
		if opts.DryRun {
			action = "would-remove"
		}
		items := make([]PruneJSON, len(result.Candidates))
		for i, c := range result.Candidates {
			items[i] = PruneJSON{
				Name:   c.Name,
				Branch: c.Branch,
				Status: c.Status,
				Action: action,
			}
		}
		return printJSON(outWriter(), items)
	}
	printWorktreeReport(result.Report)
	return nil
}
