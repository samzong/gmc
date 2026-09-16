package cmd

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

var (
	wtBaseBranch   string
	wtDupBase      string
	wtDupTasks     []string
	wtForce        bool
	wtDeleteBranch bool
	wtDryRun       bool
	wtAll          bool
	wtUpstream     string
	wtProjectName  string
	wtAddPR        int
	wtShowPR       bool
	wtDiffBase     string
)

var wtCmd = &cobra.Command{
	Use:     "wt",
	Aliases: []string{"worktree"},
	Short:   "Manage worktrees for parallel AI agents",
	Long: `Manage sibling worktrees on a bare (.bare) clone so each AI agent gets an isolated working tree.
Run without a subcommand to list worktrees.`,
	RunE: func(_ *cobra.Command, _ []string) error {
		return runWorktreeList(newWorktreeClient(), true)
	},
}

var wtAddCmd = &cobra.Command{
	Use:   "add [name...]",
	Short: "Create new worktrees with new branches",
	Long: `Create one or more worktrees, each on a new branch named after its directory.
With -b and no name, the worktree name is derived from the base branch.`,
	Example: `  gmc wt add feature-login
  gmc wt add -b feat/existing-branch`,
	Args: func(cmd *cobra.Command, args []string) error {
		if addPRMode(cmd) {
			if wtAddPR <= 0 {
				return errors.New("--pr must be greater than 0")
			}
			if len(args) > 0 {
				return errors.New("--pr is mutually exclusive with worktree names")
			}
			if strings.TrimSpace(wtBaseBranch) != "" {
				return errors.New("--pr is mutually exclusive with -b/--base")
			}
			if wtAddSync {
				return errors.New("--pr is mutually exclusive with --sync")
			}
			return nil
		}
		if len(args) == 0 && strings.TrimSpace(wtBaseBranch) == "" {
			return errors.New("requires at least 1 arg or -b/--base flag")
		}
		return nil
	},
	RunE: func(_ *cobra.Command, args []string) error {
		if len(args) == 0 {
			args = []string{wtBaseBranch}
		}
		return runWorktreeAdd(newWorktreeClient(), args)
	},
}

var wtListCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"ls"},
	Short:   "List all worktrees",
	RunE: func(_ *cobra.Command, _ []string) error {
		return runWorktreeList(newWorktreeClient(), false)
	},
}

var wtRemoveCmd = &cobra.Command{
	Use:     "remove [name...]",
	Aliases: []string{"rm"},
	Short:   "Remove worktrees",
	Long:    `Remove one or more worktrees. The branch is kept unless -D is given.`,
	Example: `  gmc wt remove feature-login
  gmc wt rm --all --dry-run`,
	Args: func(_ *cobra.Command, args []string) error {
		if wtAll && len(args) > 0 {
			return errors.New("--all and positional arguments are mutually exclusive")
		}
		if !wtAll && len(args) < 1 {
			return errors.New("requires at least 1 arg(s) or --all flag")
		}
		return nil
	},
	RunE: func(_ *cobra.Command, args []string) error {
		return runWorktreeRemove(newWorktreeClient(), args)
	},
}

var wtCloneCmd = &cobra.Command{
	Use:   "clone <url>",
	Short: "Clone a repo into bare + worktree layout",
	Long:  `Clone a repository into a .bare directory plus a worktree for the default branch.`,
	Example: `  gmc wt clone https://github.com/user/repo.git
  gmc wt clone https://github.com/me/fork.git --upstream https://github.com/org/repo.git --name repo`,
	Args: cobra.ExactArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		return runWorktreeClone(newWorktreeClient(), args[0])
	},
}

var wtDupCmd = &cobra.Command{
	Use:   "dup [count]",
	Short: "Fan out worktrees for parallel AI agents",
	Long: `Create N sibling worktrees on temporary branches (_dup/<base>/<timestamp>-<n>) for parallel agents.
Defaults to 2 worktrees from the current branch. Promote the winner with 'gmc wt promote'.`,
	Example: `  gmc wt dup
  gmc wt dup 3 -b main
  gmc wt dup 3 --task todo.md`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		return runWorktreeDup(newWorktreeClient(), args)
	},
}

var wtPromoteCmd = &cobra.Command{
	Use:   "promote <candidate>",
	Short: "Apply a candidate back into the current worktree",
	Long: `Apply a candidate's changes into the current parent worktree as uncommitted changes.
It never commits, pushes, opens PRs, or deletes the candidate.`,
	Example: `  gmc wt promote .dup-2 --dry-run
  gmc wt promote .dup-2`,
	Args: func(_ *cobra.Command, args []string) error {
		if len(args) == 2 {
			return errors.New(
				"gmc wt promote now accepts only <candidate>; " +
					"to rename a branch, cd into the worktree and run 'git branch -m <branch-name>'",
			)
		}
		if len(args) != 1 {
			return errors.New("requires exactly 1 arg(s)")
		}
		return nil
	},
	RunE: func(_ *cobra.Command, args []string) error {
		return runWorktreePromote(newWorktreeClient(), args[0])
	},
}

var wtPrReviewCmd = &cobra.Command{
	Use:   "pr-review <PR_NUMBER>",
	Short: "Create a worktree from a GitHub Pull Request",
	Long: `Create a worktree from a GitHub pull request for review.
The remote is detected automatically (upstream, then origin, then the single remote).`,
	Args: cobra.ExactArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		prNumber, err := strconv.Atoi(args[0])
		if err != nil {
			return fmt.Errorf("invalid PR number: %s", args[0])
		}

		return runWorktreeAddPR(newWorktreeClient(), prNumber)
	},
}

func init() {
	wtCmd.AddCommand(wtAddCmd, wtListCmd, wtRemoveCmd, wtCloneCmd, wtDupCmd,
		wtPromoteCmd, wtPruneCmd, wtPrReviewCmd, wtInitCmd, wtSwitchCmd)
	wtAddCmd.Flags().StringVarP(&wtBaseBranch, "base", "b", "", "Base branch to create from")
	wtAddCmd.Flags().IntVar(&wtAddPR, "pr", 0, "Create a worktree from a pull request")
	wtRemoveCmd.Flags().BoolVarP(&wtForce, "force", "f", false, "Force removal even if worktree is dirty")
	wtRemoveCmd.Flags().BoolVarP(&wtDeleteBranch, "delete-branch", "D", false, "Also delete the branch")
	wtRemoveCmd.Flags().BoolVar(&wtDryRun, "dry-run", false, "Preview what would be removed without making changes")
	wtRemoveCmd.Flags().BoolVarP(&wtAll, "all", "a", false, "Remove all non-protected worktrees")
	wtCloneCmd.Flags().StringVar(&wtUpstream, "upstream", "", "Upstream repository URL (for fork workflow)")
	wtCloneCmd.Flags().StringVar(&wtProjectName, "name", "", "Custom project directory name")
	wtDupCmd.Flags().StringVarP(&wtDupBase, "base", "b", "", "Base branch to create from")
	wtDupCmd.Flags().StringArrayVar(&wtDupTasks, "task", nil, "Task context file to copy into each candidate (repeatable)")

	wtPromoteCmd.Flags().BoolVar(&wtDryRun, "dry-run", false,
		"Check whether the candidate can be promoted without changing files")
	wtPruneCmd.Flags().StringVarP(&wtPruneBase, "base", "b", "", "Base branch to check merge status against")
	wtPruneCmd.Flags().BoolVarP(&wtPruneForce, "force", "f", false, "Force removal even if worktree is dirty")
	wtPruneCmd.Flags().BoolVar(&wtPruneDryRun, "dry-run", false, "Preview what would be removed without making changes")
	wtPruneCmd.Flags().BoolVar(&wtPrunePRAware, "pr-aware", false,
		"Check GitHub PR state before pruning (requires gh CLI)")
	wtPruneCmd.Flags().BoolVar(&wtPruneBranches, "branches", false,
		"Also delete merged local branches that have no worktree")

	wtCmd.Flags().BoolVar(&wtShowPR, "pr", false,
		"Show review request status for each branch (requires gh or glab CLI)")
	wtListCmd.Flags().BoolVar(&wtShowPR, "pr", false,
		"Show review request status for each branch (requires gh or glab CLI)")
	wtCmd.Flags().StringVar(&wtDiffBase, "diff-base", "",
		"Base branch/ref for worktree diff stats")
	wtListCmd.Flags().StringVar(&wtDiffBase, "diff-base", "",
		"Base branch/ref for worktree diff stats")
	wtRemoveCmd.ValidArgsFunction = completeWorktreeNames
	wtPromoteCmd.ValidArgsFunction = completeWorktreeNames
	_ = wtAddCmd.RegisterFlagCompletionFunc("base", completeBranchNames)
	_ = wtDupCmd.RegisterFlagCompletionFunc("base", completeBranchNames)
	_ = wtPruneCmd.RegisterFlagCompletionFunc("base", completeBranchNames)
	_ = wtCmd.RegisterFlagCompletionFunc("diff-base", completeBranchNames)
	_ = wtListCmd.RegisterFlagCompletionFunc("diff-base", completeBranchNames)
	rootCmd.AddCommand(wtCmd)
}

func addPRMode(cmd *cobra.Command) bool {
	if wtAddPR > 0 {
		return true
	}
	return cmd != nil && cmd.Flags().Changed("pr")
}

func completeWorktreeNames(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
	wtClient := newWorktreeClient()
	worktrees, err := wtClient.List()
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	filtered := filterBareWorktrees(worktrees)
	root := getDisplayRoot(wtClient)

	names := make([]string, 0, len(filtered))
	for _, wt := range filtered {
		if isExternalWorktree(root, wt.Path) || isAgentWorktree(wt.Path) {
			continue
		}
		names = append(names, displayWorktreeName(root, wt.Path))
	}
	return names, cobra.ShellCompDirectiveNoFileComp
}

func completeBranchNames(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
	wtClient := newWorktreeClient()
	branches, err := wtClient.ListBranches()
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	return branches, cobra.ShellCompDirectiveNoFileComp
}

func completeStrategies(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
	return []string{"copy", "link"}, cobra.ShellCompDirectiveNoFileComp
}
