package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/samzong/gmc/internal/worktree"
	"github.com/spf13/cobra"
)

var (
	wtBaseBranch   string
	wtForce        bool
	wtDeleteBranch bool
	wtDryRun       bool
	wtUpstream     string
	wtProjectName  string
	prRemote       string
)

var wtCmd = &cobra.Command{
	Use:     "wt",
	Aliases: []string{"worktree"},
	Short:   "Manage git worktrees with bare repository support",
	Long: `Manage git worktrees with bare repository support.

This command simplifies multi-branch parallel development using the
bare repository (.bare) + worktree pattern.
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		wtClient := worktree.NewClient(worktree.Options{Verbose: verbose})
		return runWorktreeDefault(wtClient, cmd)
	},
}

var wtAddCmd = &cobra.Command{
	Use:   "add <name>",
	Short: "Create a new worktree with a new branch",
	Long: `Create a new worktree with a new branch.

The branch name will be the same as the worktree directory name.

Examples:
  gmc wt add feature-login           # Create based on current HEAD
  gmc wt add feature-login -b main   # Create based on main branch
  gmc wt add feature-login --sync    # Sync base branch before add
  gmc wt add hotfix-bug123 -b release`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		wtClient := worktree.NewClient(worktree.Options{Verbose: verbose})
		return runWorktreeAdd(wtClient, args[0])
	},
}

var wtListCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"ls"},
	Short:   "List all worktrees (alias: ls)",
	Long:    `List all worktrees in the current repository.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		wtClient := worktree.NewClient(worktree.Options{Verbose: verbose})
		return runWorktreeList(wtClient)
	},
}

var wtRemoveCmd = &cobra.Command{
	Use:     "remove <name>",
	Aliases: []string{"rm"},
	Short:   "Remove a worktree (alias: rm)",
	Long: `Remove a worktree.

By default, only removes the worktree directory, keeping the branch.
Use -D to also delete the branch.

Examples:
  gmc wt remove feature-login      # Remove worktree, keep branch
  gmc wt rm feature-login -D       # Remove worktree and delete branch
  gmc wt rm feature-login -f       # Force remove (ignore dirty state)
  gmc wt rm feature-login --dry-run  # Preview what would be removed`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		wtClient := worktree.NewClient(worktree.Options{Verbose: verbose})
		return runWorktreeRemove(wtClient, args[0])
	},
}

var wtCloneCmd = &cobra.Command{
	Use:   "clone <url>",
	Short: "Clone a repository as bare + worktree structure",
	Long: `Clone a repository as a bare + worktree structure.

Creates a .bare directory containing the bare repository and a worktree
for the default branch.

For fork workflows, use --upstream to specify the upstream repository.

Examples:
  gmc wt clone https://github.com/user/repo.git
  gmc wt clone https://github.com/user/repo.git --name my-project
  gmc wt clone https://github.com/me/fork.git --upstream https://github.com/org/repo.git`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		wtClient := worktree.NewClient(worktree.Options{Verbose: verbose})
		return runWorktreeClone(wtClient, args[0])
	},
}

var wtDupCmd = &cobra.Command{
	Use:   "dup [count]",
	Short: "Create multiple worktrees for parallel development",
	Long: `Create multiple worktrees with temporary branches for parallel AI development.

Each worktree gets a temporary branch (_dup/<base>/<timestamp>-<n>) that can
be promoted to a permanent name later using 'gmc wt promote'.

Examples:
  gmc wt dup           # Create 2 worktrees based on main
  gmc wt dup 3         # Create 3 worktrees based on main
  gmc wt dup -b dev    # Create 2 worktrees based on dev
  gmc wt dup 3 -b dev  # Create 3 worktrees based on dev`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		wtClient := worktree.NewClient(worktree.Options{Verbose: verbose})
		return runWorktreeDup(wtClient, args)
	},
}

var wtPromoteCmd = &cobra.Command{
	Use:   "promote <worktree> <branch-name>",
	Short: "Rename temporary branch to permanent name",
	Long: `Rename the temporary branch of a worktree to a permanent branch name.

Use this after evaluating parallel development results to keep the best solution.

Examples:
  gmc wt promote .dup-2 feature/add-auth
  gmc wt promote .dup-1 fix/login-bug`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		wtClient := worktree.NewClient(worktree.Options{Verbose: verbose})
		return runWorktreePromote(wtClient, args[0], args[1])
	},
}

var wtPrReviewCmd = &cobra.Command{
	Use:   "pr-review <PR_NUMBER>",
	Short: "Create a worktree from a GitHub Pull Request",
	Long: `Create a worktree from a GitHub Pull Request for code review.

Automatically detects remote (upstream > origin > single remote).

Examples:
  gmc wt pr-review 1065                 # Auto-detect remote
  gmc wt pr-review 1065 --remote fork   # Use specific remote`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		prNumber, err := strconv.Atoi(args[0])
		if err != nil {
			return fmt.Errorf("invalid PR number: %s", args[0])
		}

		wtClient := worktree.NewClient(worktree.Options{Verbose: verbose})
		return wtClient.AddPR(prNumber, prRemote)
	},
}

func init() {
	// Add subcommands
	wtCmd.AddCommand(wtAddCmd)
	wtCmd.AddCommand(wtListCmd)
	wtCmd.AddCommand(wtRemoveCmd)
	wtCmd.AddCommand(wtCloneCmd)
	wtCmd.AddCommand(wtDupCmd)
	wtCmd.AddCommand(wtPromoteCmd)
	wtCmd.AddCommand(wtPruneCmd)
	wtCmd.AddCommand(wtPrReviewCmd)

	// Flags for add command
	wtAddCmd.Flags().StringVarP(&wtBaseBranch, "base", "b", "", "Base branch to create from")

	// Flags for remove command
	wtRemoveCmd.Flags().BoolVarP(&wtForce, "force", "f", false, "Force removal even if worktree is dirty")
	wtRemoveCmd.Flags().BoolVarP(&wtDeleteBranch, "delete-branch", "D", false, "Also delete the branch")
	wtRemoveCmd.Flags().BoolVar(&wtDryRun, "dry-run", false, "Preview what would be removed without making changes")

	// Flags for clone command
	wtCloneCmd.Flags().StringVar(&wtUpstream, "upstream", "", "Upstream repository URL (for fork workflow)")
	wtCloneCmd.Flags().StringVar(&wtProjectName, "name", "", "Custom project directory name")

	// Flags for dup command
	wtDupCmd.Flags().StringVarP(&wtBaseBranch, "base", "b", "main", "Base branch to create from")

	// Flags for prune command
	wtPruneCmd.Flags().StringVarP(&wtPruneBase, "base", "b", "", "Base branch to check merge status against")
	wtPruneCmd.Flags().BoolVarP(&wtPruneForce, "force", "f", false, "Force removal even if worktree is dirty")
	wtPruneCmd.Flags().BoolVar(&wtPruneDryRun, "dry-run", false, "Preview what would be removed without making changes")

	// Flags for pr-review command
	wtPrReviewCmd.Flags().StringVarP(&prRemote, "remote", "r", "",
		"Remote to fetch PR from (auto-detect if not specified)")

	// Add to root command
	rootCmd.AddCommand(wtCmd)
}

func runWorktreeDefault(wtClient *worktree.Client, cmd *cobra.Command) error {
	// Auto-detect if we're in bare worktree mode
	isBareWorktree := wtClient.IsBareWorktree()

	// If not using bare worktree pattern, show status + help
	if !isBareWorktree {
		fmt.Fprintln(outWriter(), "Current repository is not using the bare worktree pattern.")
		return nil
	}

	// In bare worktree mode - show full worktree info
	worktrees, err := wtClient.List()
	if err != nil {
		return err
	}

	// Filter out bare worktrees
	filtered := filterBareWorktrees(worktrees)

	fmt.Fprintln(outWriter(), "Current Worktrees:")
	printWorktreeTable(wtClient, filtered)

	// Print common commands
	fmt.Fprintln(outWriter())
	fmt.Fprintln(outWriter(), "Available Commands:")

	// Dynamically generate from subcommands
	for _, subcmd := range cmd.Commands() {
		if subcmd.Hidden {
			continue
		}
		// Format: "  command_name   Description"
		fmt.Fprintf(outWriter(), "  %-18s %s\n", subcmd.Name(), subcmd.Short)
	}

	fmt.Fprintln(outWriter())
	fmt.Fprintf(outWriter(), "Run 'gmc wt <command> --help' for more information on a command.\n")

	// Show current location
	cwd, err := os.Getwd()
	if err == nil {
		for _, wt := range filtered {
			if strings.HasPrefix(cwd, wt.Path) {
				fmt.Fprintln(outWriter())
				fmt.Fprintf(outWriter(), "You are here: ./%s (branch: %s)\n", filepath.Base(wt.Path), wt.Branch)
				break
			}
		}
	}

	return nil
}

// filterBareWorktrees removes bare worktrees from the list (e.g., .bare directory)
func filterBareWorktrees(worktrees []worktree.WorktreeInfo) []worktree.WorktreeInfo {
	var filtered []worktree.WorktreeInfo
	for _, wt := range worktrees {
		// Skip bare worktrees and the .bare directory itself
		if wt.IsBare || filepath.Base(wt.Path) == ".bare" {
			continue
		}
		filtered = append(filtered, wt)
	}
	return filtered
}

func runWorktreeAdd(wtClient *worktree.Client, name string) error {
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
		if err := wtClient.Sync(syncOpts); err != nil {
			return err
		}
	}
	opts := worktree.AddOptions{
		BaseBranch: baseBranch,
		Fetch:      false,
	}
	return wtClient.Add(name, opts)
}

func runWorktreeList(wtClient *worktree.Client) error {
	worktrees, err := wtClient.List()
	if err != nil {
		return err
	}

	// Filter out bare worktrees
	filtered := filterBareWorktrees(worktrees)

	if len(filtered) == 0 {
		fmt.Fprintln(outWriter(), "No worktrees found.")
		return nil
	}

	printWorktreeTable(wtClient, filtered)
	return nil
}

func runWorktreeRemove(wtClient *worktree.Client, name string) error {
	opts := worktree.RemoveOptions{
		Force:        wtForce,
		DeleteBranch: wtDeleteBranch,
		DryRun:       wtDryRun,
	}
	return wtClient.Remove(name, opts)
}

func runWorktreeClone(wtClient *worktree.Client, url string) error {
	opts := worktree.CloneOptions{
		Name:     wtProjectName,
		Upstream: wtUpstream,
	}
	return wtClient.Clone(url, opts)
}

func printWorktreeTable(wtClient *worktree.Client, worktrees []worktree.WorktreeInfo) {
	if len(worktrees) == 0 {
		return
	}

	// Get root for calculating relative paths
	root, err := wtClient.GetWorktreeRoot()
	if err != nil {
		// Fallback to base name if we can't get root
		root = ""
	}

	// Calculate column widths
	maxName := len("Name")
	maxBranch := len("Branch")
	for _, wt := range worktrees {
		var name string
		if root != "" {
			name = strings.TrimPrefix(wt.Path, root+string(filepath.Separator))
		} else {
			name = filepath.Base(wt.Path)
		}
		if len(name) > maxName {
			maxName = len(name)
		}
		if len(wt.Branch) > maxBranch {
			maxBranch = len(wt.Branch)
		}
	}

	// Add padding
	maxName += 2
	maxBranch += 2

	// Print header
	fmt.Fprintf(outWriter(), "%-*s %-*s %-8s %s\n", maxName, "NAME", maxBranch, "BRANCH", "COMMIT", "STATUS")

	// Print rows
	for _, wt := range worktrees {
		var name string
		if root != "" {
			name = strings.TrimPrefix(wt.Path, root+string(filepath.Separator))
		} else {
			name = filepath.Base(wt.Path)
		}

		// Get short commit hash (7 characters)
		shortCommit := wt.Commit
		if len(shortCommit) > 7 {
			shortCommit = shortCommit[:7]
		}

		// Get enhanced status
		status := wtClient.GetWorktreeStatus(wt.Path)
		if wt.IsBare {
			status = "bare"
		}

		fmt.Fprintf(outWriter(), "%-*s %-*s %-8s %s\n", maxName, name, maxBranch, wt.Branch, shortCommit, status)
	}
}

func runWorktreeDup(wtClient *worktree.Client, args []string) error {
	opts := worktree.DupOptions{
		BaseBranch: wtBaseBranch,
		Count:      2,
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

	fmt.Fprintf(outWriter(), "Created %d worktrees based on '%s':\n", len(result.Worktrees), opts.BaseBranch)
	for i, wt := range result.Worktrees {
		fmt.Fprintf(outWriter(), "  %s -> %s\n", wt, result.Branches[i])
	}
	fmt.Fprintln(outWriter())
	fmt.Fprintln(outWriter(), "Next steps:")
	fmt.Fprintln(outWriter(), "  1. Work in each directory with different AI tools")
	fmt.Fprintln(outWriter(), "  2. Evaluate and pick the best solution")
	fmt.Fprintf(outWriter(), "  3. Run: gmc wt promote <worktree> <branch-name>\n")
	fmt.Fprintln(outWriter(), "  4. Clean up: gmc wt rm <other-worktrees> -D")

	return nil
}

func runWorktreePromote(wtClient *worktree.Client, worktreeName, branchName string) error {
	return wtClient.Promote(worktreeName, branchName)
}
