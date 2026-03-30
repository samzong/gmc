package cmd

import (
	"fmt"
	"text/tabwriter"

	"github.com/samzong/gmc/internal/worktree"
	"github.com/spf13/cobra"
)

var (
	wtPruneBase    string
	wtPruneForce   bool
	wtPruneDryRun  bool
	wtPrunePRAware bool
)

var wtPruneCmd = &cobra.Command{
	Use:   "prune",
	Short: "Remove worktrees whose branches are merged",
	Long: `Remove worktrees whose branches are already merged into the base branch.

This command uses pure git ancestry checks to decide which worktrees are safe to remove.
By default it removes both the worktree directory and the local branch.

Use --pr-aware to check GitHub PR state (via gh CLI) before deciding whether
to remove each worktree. Only worktrees with MERGED PRs are removed.`,
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
		PRAware:    wtPrunePRAware,
	}
	result, err := wtClient.Prune(opts)
	if err != nil {
		return err
	}

	if outputFormat() == "json" {
		if opts.PRAware {
			return printJSON(outWriter(), result.PruneEntries)
		}
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
	if len(result.PruneEntries) > 0 {
		printPruneTable(result.PruneEntries)
	}
	return nil
}

func printPruneTable(entries []worktree.PruneEntry) {
	w := tabwriter.NewWriter(outWriter(), 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tBRANCH\tPR\tPR STATE\tACTION\tREASON")
	for _, e := range entries {
		pr := "-"
		if e.PRNum > 0 {
			pr = fmt.Sprintf("#%d", e.PRNum)
		}
		state := e.PRState
		if state == "" {
			state = "none"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n", e.Name, e.Branch, pr, state, e.Action, e.Reason)
	}
	w.Flush()
}
