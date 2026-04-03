package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/samzong/gmc/internal/stringsutil"
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
	RunE: func(cmd *cobra.Command, _ []string) error {
		wtClient := newWorktreeClient()
		return runWorktreeDefault(wtClient, cmd)
	},
}

var wtAddCmd = &cobra.Command{
	Use:   "add <name> [name...]",
	Short: "Create new worktrees with new branches",
	Long: `Create one or more worktrees with new branches.

The branch name will be the same as the worktree directory name.

Examples:
  gmc wt add feature-login                    # Create one worktree
  gmc wt add feat-a feat-b feat-c             # Create multiple worktrees
  gmc wt add feature-login -b main            # Create based on main branch
  gmc wt add feature-login --sync             # Sync base branch before add
  gmc wt add hotfix-bug123 -b release`,
	Args: cobra.MinimumNArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		wtClient := newWorktreeClient()
		return runWorktreeAdd(wtClient, args)
	},
}

var wtListCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"ls"},
	Short:   "List all worktrees (alias: ls)",
	Long:    `List all worktrees in the current repository.`,
	RunE: func(_ *cobra.Command, _ []string) error {
		wtClient := newWorktreeClient()
		return runWorktreeList(wtClient)
	},
}

var wtRemoveCmd = &cobra.Command{
	Use:     "remove <name> [name...]",
	Aliases: []string{"rm"},
	Short:   "Remove worktrees (alias: rm)",
	Long: `Remove one or more worktrees.

By default, only removes the worktree directory, keeping the branch.
Use -D to also delete the branch.

Examples:
  gmc wt remove feature-login           # Remove one worktree
  gmc wt rm feat-a feat-b feat-c        # Remove multiple worktrees
  gmc wt rm feature-login -D            # Remove worktree and delete branch
  gmc wt rm feature-login -f            # Force remove (ignore dirty state)
  gmc wt rm feature-login --dry-run     # Preview what would be removed`,
	Args: cobra.MinimumNArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		wtClient := newWorktreeClient()
		return runWorktreeRemove(wtClient, args)
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
	RunE: func(_ *cobra.Command, args []string) error {
		wtClient := newWorktreeClient()
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
	RunE: func(_ *cobra.Command, args []string) error {
		wtClient := newWorktreeClient()
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
	RunE: func(_ *cobra.Command, args []string) error {
		wtClient := newWorktreeClient()
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
	RunE: func(_ *cobra.Command, args []string) error {
		prNumber, err := strconv.Atoi(args[0])
		if err != nil {
			return fmt.Errorf("invalid PR number: %s", args[0])
		}

		wtClient := newWorktreeClient()
		report, err := wtClient.AddPR(prNumber, prRemote)
		printWorktreeReport(report)
		return err
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
	wtCmd.AddCommand(wtInitCmd)
	wtCmd.AddCommand(wtSwitchCmd)

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
	wtPruneCmd.Flags().BoolVar(&wtPrunePRAware, "pr-aware", false, "Check GitHub PR state before pruning (requires gh CLI)")

	// Flags for pr-review command
	wtPrReviewCmd.Flags().StringVarP(&prRemote, "remote", "r", "",
		"Remote to fetch PR from (auto-detect if not specified)")

	// Shell completions for arguments
	wtRemoveCmd.ValidArgsFunction = completeWorktreeNames
	wtPromoteCmd.ValidArgsFunction = completeWorktreeNames

	// Shell completions for flags
	_ = wtAddCmd.RegisterFlagCompletionFunc("base", completeBranchNames)
	_ = wtDupCmd.RegisterFlagCompletionFunc("base", completeBranchNames)
	_ = wtPruneCmd.RegisterFlagCompletionFunc("base", completeBranchNames)
	_ = wtPrReviewCmd.RegisterFlagCompletionFunc("remote", completeRemoteNames)

	// Add to root command
	rootCmd.AddCommand(wtCmd)
}

type WorktreeJSON struct {
	Name   string `json:"name"`
	Path   string `json:"path"`
	Branch string `json:"branch"`
	Commit string `json:"commit"`
	Status string `json:"status"`
}

func runWorktreeDefault(wtClient *worktree.Client, _ *cobra.Command) error {
	worktrees, err := wtClient.List()
	if err != nil {
		return err
	}

	filtered := filterBareWorktrees(worktrees)

	if outputFormat() == "json" {
		return printWorktreeJSON(wtClient, filtered)
	}

	fmt.Fprintln(outWriter(), "Current Worktrees:")
	printWorktreeTable(wtClient, filtered)

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
func filterBareWorktrees(worktrees []worktree.Info) []worktree.Info {
	var filtered []worktree.Info
	for _, wt := range worktrees {
		// Skip bare worktrees and the .bare directory itself
		if wt.IsBare || filepath.Base(wt.Path) == ".bare" {
			continue
		}
		filtered = append(filtered, wt)
	}
	return filtered
}

func runWorktreeAdd(wtClient *worktree.Client, names []string) error {
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

func runWorktreeList(wtClient *worktree.Client) error {
	worktrees, err := wtClient.List()
	if err != nil {
		return err
	}

	filtered := filterBareWorktrees(worktrees)

	if outputFormat() == "json" {
		return printWorktreeJSON(wtClient, filtered)
	}

	if len(filtered) == 0 {
		fmt.Fprintln(outWriter(), "No worktrees found.")
		return nil
	}

	printWorktreeTable(wtClient, filtered)
	return nil
}

func runWorktreeRemove(wtClient *worktree.Client, names []string) error {
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

func runWorktreeClone(wtClient *worktree.Client, url string) error {
	opts := worktree.CloneOptions{
		Name:     wtProjectName,
		Upstream: wtUpstream,
	}
	report, err := wtClient.Clone(url, opts)
	printWorktreeReport(report)
	return err
}

// getDisplayRoot returns the root to use for worktree name display and external detection.
// Bare layout: root (parent of .bare) — all managed worktrees live inside it.
// Non-bare layout: parent of the repo dir — sibling linked worktrees show with short names.
func getDisplayRoot(wtClient *worktree.Client) string {
	root, err := wtClient.GetWorktreeRoot()
	if err != nil || root == "" {
		return ""
	}
	bareDir := filepath.Join(root, ".bare")
	if info, err := os.Stat(bareDir); err == nil && info.IsDir() {
		return root // bare layout
	}
	return filepath.Dir(root) // non-bare: use parent so siblings are not flagged external
}

// isExternalWorktree reports whether wtPath is outside the display root.
func isExternalWorktree(displayRoot, wtPath string) bool {
	if displayRoot == "" {
		return false
	}
	rel, err := filepath.Rel(displayRoot, wtPath)
	if err != nil {
		return true
	}
	return strings.HasPrefix(rel, ".."+string(filepath.Separator)) || rel == ".."
}

// isAgentWorktree reports whether the path is inside a known AI-agent worktree directory
// (e.g. .claude/worktrees/ or .codex/worktrees/).
func isAgentWorktree(wtPath string) bool {
	normalized := filepath.ToSlash(wtPath)
	return strings.Contains(normalized, "/.claude/worktrees/") ||
		strings.Contains(normalized, "/.codex/worktrees/")
}

func abbrevPath(path string) string {
	if home, err := os.UserHomeDir(); err == nil && strings.HasPrefix(path, home+string(filepath.Separator)) {
		return "~" + path[len(home):]
	}
	return path
}

func displayWorktreeName(displayRoot string, wtPath string) string {
	if displayRoot == "" {
		return filepath.Base(wtPath)
	}
	rel, err := filepath.Rel(displayRoot, wtPath)
	if err != nil || rel == "." || rel == "" {
		return filepath.Base(wtPath)
	}
	if strings.HasPrefix(rel, ".."+string(filepath.Separator)) || rel == ".." {
		// External: always show absolute path so the user knows where it is.
		return abbrevPath(wtPath)
	}
	// Agent worktrees inside the root also show their absolute path for easy navigation.
	if isAgentWorktree(wtPath) {
		return abbrevPath(wtPath)
	}
	return rel
}

func resolveWorktreeStatus(wtClient *worktree.Client, root string, wt worktree.Info) string {
	switch {
	case wt.IsBare:
		return "bare"
	case isExternalWorktree(root, wt.Path), isAgentWorktree(wt.Path):
		return "agent"
	default:
		return wtClient.GetWorktreeStatus(wt.Path)
	}
}

func printWorktreeTable(wtClient *worktree.Client, worktrees []worktree.Info) {
	if len(worktrees) == 0 {
		return
	}

	root := getDisplayRoot(wtClient)

	maxName := len("Name")
	maxBranch := len("Branch")
	for _, wt := range worktrees {
		name := displayWorktreeName(root, wt.Path)
		if len(name) > maxName {
			maxName = len(name)
		}
		if len(wt.Branch) > maxBranch {
			maxBranch = len(wt.Branch)
		}
	}

	maxName += 2
	maxBranch += 2

	fmt.Fprintf(outWriter(), "%-*s %-*s %-8s %s\n", maxName, "NAME", maxBranch, "BRANCH", "COMMIT", "STATUS")

	for _, wt := range worktrees {
		name := displayWorktreeName(root, wt.Path)
		shortCommit := stringsutil.ShortHash(wt.Commit, 7, "")
		status := resolveWorktreeStatus(wtClient, root, wt)
		fmt.Fprintf(outWriter(), "%-*s %-*s %-8s %s\n", maxName, name, maxBranch, wt.Branch, shortCommit, status)
	}
}

func buildWorktreeJSON(wtClient *worktree.Client, worktrees []worktree.Info) []WorktreeJSON {
	root := getDisplayRoot(wtClient)
	result := make([]WorktreeJSON, 0, len(worktrees))
	for _, wt := range worktrees {
		result = append(result, WorktreeJSON{
			Name:   displayWorktreeName(root, wt.Path),
			Path:   wt.Path,
			Branch: wt.Branch,
			Commit: wt.Commit,
			Status: resolveWorktreeStatus(wtClient, root, wt),
		})
	}
	return result
}

func printWorktreeJSON(wtClient *worktree.Client, worktrees []worktree.Info) error {
	return printJSON(outWriter(), buildWorktreeJSON(wtClient, worktrees))
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
	for _, warning := range result.Warnings {
		fmt.Fprintln(errWriter(), warning)
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
	report, err := wtClient.Promote(worktreeName, branchName)
	printWorktreeReport(report)
	return err
}

// Completion functions

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
		// Skip agent/external worktrees — rm/promote cannot operate on them
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

func completeRemoteNames(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
	wtClient := newWorktreeClient()
	remotes, err := wtClient.ListRemotes()
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	return remotes, cobra.ShellCompDirectiveNoFileComp
}

func completeStrategies(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
	return []string{"copy", "link"}, cobra.ShellCompDirectiveNoFileComp
}
