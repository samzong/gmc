package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/samzong/gmc/internal/worktree"
	"github.com/spf13/cobra"
)

var (
	wtBaseBranch   string
	wtForce        bool
	wtDeleteBranch bool
	wtUpstream     string
	wtProjectName  string
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
		worktree.Verbose = verbose
		return runWorktreeDefault()
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
  gmc wt add hotfix-bug123 -b release`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		worktree.Verbose = verbose
		return runWorktreeAdd(args[0])
	},
}

var wtListCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"ls"},
	Short:   "List all worktrees (alias: ls)",
	Long:    `List all worktrees in the current repository.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		worktree.Verbose = verbose
		return runWorktreeList()
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
  gmc wt rm feature-login -f       # Force remove (ignore dirty state)`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		worktree.Verbose = verbose
		return runWorktreeRemove(args[0])
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
		worktree.Verbose = verbose
		return runWorktreeClone(args[0])
	},
}

func init() {
	// Add subcommands
	wtCmd.AddCommand(wtAddCmd)
	wtCmd.AddCommand(wtListCmd)
	wtCmd.AddCommand(wtRemoveCmd)
	wtCmd.AddCommand(wtCloneCmd)

	// Flags for add command
	wtAddCmd.Flags().StringVarP(&wtBaseBranch, "base", "b", "", "Base branch to create from")

	// Flags for remove command
	wtRemoveCmd.Flags().BoolVarP(&wtForce, "force", "f", false, "Force removal even if worktree is dirty")
	wtRemoveCmd.Flags().BoolVarP(&wtDeleteBranch, "delete-branch", "D", false, "Also delete the branch")

	// Flags for clone command
	wtCloneCmd.Flags().StringVar(&wtUpstream, "upstream", "", "Upstream repository URL (for fork workflow)")
	wtCloneCmd.Flags().StringVar(&wtProjectName, "name", "", "Custom project directory name")

	// Add to root command
	rootCmd.AddCommand(wtCmd)
}

func runWorktreeDefault() error {
	// Check if this is a bare worktree setup
	isBareWorktree := worktree.IsBareWorktree()

	// If not using bare worktree pattern, show status + help
	if !isBareWorktree {
		fmt.Println("Current repository is not using the bare worktree pattern.")
		return nil
	}

	// In bare worktree mode - show full worktree info
	worktrees, err := worktree.List()
	if err != nil {
		return err
	}

	// Filter out bare worktrees
	filtered := filterBareWorktrees(worktrees)

	fmt.Println("Current Worktrees:")
	printWorktreeTable(filtered)

	// Print common commands
	fmt.Println()
	fmt.Println("Common Commands:")
	fmt.Println("  gmc wt add <branch>      Create new worktree with branch")
	fmt.Println("  gmc wt add <branch> -b   Create based on specific branch")
	fmt.Println("  gmc wt rm <name>         Remove worktree (keeps branch)")
	fmt.Println("  gmc wt rm <name> -D      Remove worktree and delete branch")

	// Show current location
	cwd, err := os.Getwd()
	if err == nil {
		for _, wt := range filtered {
			if strings.HasPrefix(cwd, wt.Path) {
				fmt.Println()
				fmt.Printf("You are here: ./%s (branch: %s)\n", filepath.Base(wt.Path), wt.Branch)
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

func runWorktreeAdd(name string) error {
	opts := worktree.AddOptions{
		BaseBranch: wtBaseBranch,
		Fetch:      false,
	}
	return worktree.Add(name, opts)
}

func runWorktreeList() error {
	worktrees, err := worktree.List()
	if err != nil {
		return err
	}

	// Filter out bare worktrees
	filtered := filterBareWorktrees(worktrees)

	if len(filtered) == 0 {
		fmt.Println("No worktrees found.")
		return nil
	}

	printWorktreeTable(filtered)
	return nil
}

func runWorktreeRemove(name string) error {
	opts := worktree.RemoveOptions{
		Force:        wtForce,
		DeleteBranch: wtDeleteBranch,
	}
	return worktree.Remove(name, opts)
}

func runWorktreeClone(url string) error {
	opts := worktree.CloneOptions{
		Name:     wtProjectName,
		Upstream: wtUpstream,
	}
	return worktree.Clone(url, opts)
}

func printWorktreeTable(worktrees []worktree.WorktreeInfo) {
	if len(worktrees) == 0 {
		return
	}

	// Calculate column widths
	maxBranch := len("Branch")
	maxPath := len("Path")
	for _, wt := range worktrees {
		if len(wt.Branch) > maxBranch {
			maxBranch = len(wt.Branch)
		}
		relPath := getRelativePath(wt.Path)
		if len(relPath) > maxPath {
			maxPath = len(relPath)
		}
	}

	// Add padding
	maxBranch += 2
	maxPath += 2

	// Print header
	fmt.Printf("┌%s┬%s┬%s┐\n",
		strings.Repeat("─", maxBranch),
		strings.Repeat("─", maxPath),
		strings.Repeat("─", 10))
	fmt.Printf("│ %-*s│ %-*s│ %-8s│\n",
		maxBranch-1, "Branch",
		maxPath-1, "Path",
		"Status")
	fmt.Printf("├%s┼%s┼%s┤\n",
		strings.Repeat("─", maxBranch),
		strings.Repeat("─", maxPath),
		strings.Repeat("─", 10))

	// Print rows
	for _, wt := range worktrees {
		relPath := getRelativePath(wt.Path)
		status := worktree.GetWorktreeStatus(wt.Path)
		if wt.IsBare {
			status = "bare"
		}
		fmt.Printf("│ %-*s│ %-*s│ %-8s│\n",
			maxBranch-1, wt.Branch,
			maxPath-1, relPath,
			status)
	}

	fmt.Printf("└%s┴%s┴%s┘\n",
		strings.Repeat("─", maxBranch),
		strings.Repeat("─", maxPath),
		strings.Repeat("─", 10))
}

func getRelativePath(absPath string) string {
	cwd, err := os.Getwd()
	if err != nil {
		return absPath
	}

	// Try to get relative path from worktree root
	root, err := worktree.GetWorktreeRoot()
	if err == nil {
		rel, err := filepath.Rel(root, absPath)
		if err == nil {
			return "./" + rel
		}
	}

	// Fall back to relative from cwd
	rel, err := filepath.Rel(cwd, absPath)
	if err != nil {
		return absPath
	}

	if !strings.HasPrefix(rel, ".") {
		rel = "./" + rel
	}
	return rel
}
