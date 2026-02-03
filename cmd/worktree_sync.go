package cmd

import (
	"github.com/samzong/gmc/internal/worktree"
	"github.com/spf13/cobra"
)

var (
	wtAddSync    bool
	wtSyncBase   string
	wtSyncDryRun bool
)

var wtSyncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Sync the base branch used for worktrees",
	Long: `Sync the base branch used for worktrees.

This updates the base branch using fast-forward only and optionally
updates the base worktree when it's clean.`,
	RunE: func(_ *cobra.Command, _ []string) error {
		wtClient := worktree.NewClient(worktree.Options{Verbose: verbose})
		return runWorktreeSync(wtClient)
	},
}

func init() {
	wtCmd.AddCommand(wtSyncCmd)
	wtAddCmd.Flags().BoolVar(&wtAddSync, "sync", false, "Sync base branch before creating worktree")
	wtSyncCmd.Flags().StringVarP(&wtSyncBase, "base", "b", "", "Base branch to sync")
	wtSyncCmd.Flags().BoolVar(&wtSyncDryRun, "dry-run", false, "Preview what would be updated without making changes")

	_ = wtSyncCmd.RegisterFlagCompletionFunc("base", completeBranchNames)
}

func runWorktreeSync(wtClient *worktree.Client) error {
	opts := worktree.SyncOptions{
		BaseBranch: wtSyncBase,
		DryRun:     wtSyncDryRun,
	}
	return wtClient.Sync(opts)
}
