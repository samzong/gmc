package cmd

import (
	"fmt"
	"io"
	"os"
	"text/tabwriter"

	"github.com/samzong/gmc/internal/worktree"
	"github.com/spf13/cobra"
)

var wtPruneCmd = newWorktreePruneCommand()

func newWorktreePruneCommand() *cobra.Command {
	var opts worktree.PruneOptions
	var all bool
	cmd := &cobra.Command{
		Use:   "prune",
		Short: "Remove worktrees whose branches are merged",
		Long: `By default the local branch is deleted along with the worktree.

With --branches, merged local branches that have no worktree are deleted too,
so long-lived clones stop accumulating stale branches.

With --all, discover repositories with linked worktrees in the home directory
and prune each repository using its own base branch.`,
		Example: `  gmc wt prune --dry-run
  gmc wt prune --branches --pr-aware --dry-run
  gmc wt prune --all --pr-aware --dry-run`,
		Args:              cobra.NoArgs,
		ValidArgsFunction: cobra.NoFileCompletions,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if all {
				return runWorktreePruneAll(cmd, opts)
			}
			return runWorktreePrune(cmd, newWorktreeClient(), opts)
		},
	}
	cmd.Flags().StringVarP(&opts.BaseBranch, "base", "b", "", "Base branch to check merge status against")
	cmd.Flags().BoolVarP(&opts.Force, "force", "f", false, "Force removal even if worktree is dirty")
	cmd.Flags().BoolVar(&opts.DryRun, "dry-run", false, "Preview what would be removed without making changes")
	cmd.Flags().BoolVar(&opts.PRAware, "pr-aware", false, "Check GitHub PR state before pruning (requires gh CLI)")
	cmd.Flags().BoolVar(&opts.Branches, "branches", false, "Also delete merged local branches that have no worktree")
	cmd.Flags().BoolVar(&all, "all", false, "Prune repositories with linked worktrees in the home directory")
	cmd.MarkFlagsMutuallyExclusive("all", "base")
	_ = cmd.RegisterFlagCompletionFunc("base", completeBranchNames)
	return cmd
}

type PruneJSON struct {
	Repository string `json:"repository,omitempty"`
	Name       string `json:"name"`
	Branch     string `json:"branch"`
	Status     string `json:"status"`
	Action     string `json:"action"`
}

func runWorktreePrune(cmd *cobra.Command, wtClient *worktree.Client, opts worktree.PruneOptions) error {
	result, err := wtClient.Prune(opts)
	if err != nil {
		return err
	}
	if outputFormat() == "json" {
		if opts.PRAware {
			return printJSON(cmd.OutOrStdout(), result.PruneEntries)
		}
		return printJSON(cmd.OutOrStdout(), pruneJSON(result, opts, ""))
	}
	printPruneResult(cmd, result)
	return nil
}

func runWorktreePruneAll(cmd *cobra.Command, opts worktree.PruneOptions) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	fmt.Fprintf(cmd.ErrOrStderr(), "Scanning %s for worktrees...\n", sanitizeForTerminal(abbrevPath(home)))
	scan, err := worktree.Scan(home)
	if err != nil {
		return err
	}
	for _, warning := range scan.Warnings {
		fmt.Fprintf(cmd.ErrOrStderr(), "Warning: %s\n", sanitizeForTerminal(warning.Error()))
	}
	items := []PruneJSON{}
	entries := []worktree.PruneEntry{}
	seen := make(map[string]bool)
	for _, wt := range scan.Worktrees {
		repository := wt.Repository
		if seen[repository] {
			continue
		}
		if outputFormat() != "json" {
			if len(seen) > 0 {
				fmt.Fprintln(cmd.OutOrStdout())
			}
			fmt.Fprintln(cmd.OutOrStdout(), sanitizeForTerminal(abbrevPath(repository)))
		}
		seen[repository] = true
		result, pruneErr := pruneScannedRepository(repository, opts)
		if pruneErr != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "Warning: cannot prune %s: %s\n",
				sanitizeForTerminal(abbrevPath(repository)), sanitizeForTerminal(pruneErr.Error()))
		}
		if outputFormat() != "json" {
			printPruneResult(cmd, result)
			continue
		}
		for i := range result.PruneEntries {
			result.PruneEntries[i].Repository = repository
		}
		entries = append(entries, result.PruneEntries...)
		items = append(items, pruneJSON(result, opts, repository)...)
	}
	if outputFormat() == "json" {
		if opts.PRAware {
			return printJSON(cmd.OutOrStdout(), entries)
		}
		return printJSON(cmd.OutOrStdout(), items)
	}
	return nil
}

func pruneScannedRepository(repository string, opts worktree.PruneOptions) (worktree.PruneResult, error) {
	client := newWorktreeClientAt(repository)
	root, err := client.GetWorktreeRoot()
	if err != nil {
		return worktree.PruneResult{}, err
	}
	rootInfo, err := os.Stat(root)
	if err != nil {
		return worktree.PruneResult{}, err
	}
	repositoryInfo, err := os.Stat(repository)
	if err != nil {
		return worktree.PruneResult{}, err
	}
	if !os.SameFile(rootInfo, repositoryInfo) {
		return worktree.PruneResult{}, fmt.Errorf("resolved worktree root %s does not match scanned repository",
			abbrevPath(root))
	}
	return client.Prune(opts)
}

func pruneJSON(result worktree.PruneResult, opts worktree.PruneOptions, repository string) []PruneJSON {
	action := "removed"
	if opts.DryRun {
		action = "would-remove"
	}
	items := make([]PruneJSON, len(result.Candidates))
	for i, c := range result.Candidates {
		items[i] = PruneJSON{Repository: repository, Name: c.Name, Branch: c.Branch, Status: c.Status, Action: action}
	}
	return items
}

func printPruneResult(cmd *cobra.Command, result worktree.PruneResult) {
	printWorktreeReportTo(result.Report, cmd.OutOrStdout())
	if len(result.PruneEntries) > 0 {
		printPruneTable(cmd.OutOrStdout(), result.PruneEntries)
	}
}

func printPruneTable(out io.Writer, entries []worktree.PruneEntry) {
	w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tBRANCH\tPR\tSTATE\tACTION\tREASON")
	for _, e := range entries {
		name := e.Name
		if name == "" {
			name = "-"
		}
		pr := "-"
		if e.PRNum > 0 {
			pr = fmt.Sprintf("#%d", e.PRNum)
		}
		state := e.PRState
		if state == "" {
			state = "none"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n", name, e.Branch, pr, state, e.Action, e.Reason)
	}
	w.Flush()
}
