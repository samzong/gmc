package cmd

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/mattn/go-isatty"
	"github.com/samzong/gmc/internal/stringsutil"
	"github.com/samzong/gmc/internal/worktree"
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
	prRemote       string
	wtShowPR       bool
)

var wtCmd = &cobra.Command{
	Use:     "wt",
	Aliases: []string{"worktree"},
	Short:   "Manage worktrees for parallel AI agents",
	Long: `Manage git worktrees for running AI coding agents in parallel.

Uses a bare repository (.bare) + sibling worktree layout so each agent
(Claude Code, Codex, Copilot, ...) gets its own isolated working tree.
Use 'dup' to fan out N worktrees for parallel agents, 'share' to keep
.env / node_modules consistent across them, 'sync' to refresh against
the base branch, and 'promote' to keep the winning solution.
`,
	RunE: func(cmd *cobra.Command, _ []string) error {
		wtClient := newWorktreeClient()
		return runWorktreeDefault(wtClient, cmd)
	},
}

var wtAddCmd = &cobra.Command{
	Use:   "add [name...]",
	Short: "Create new worktrees with new branches",
	Long: `Create one or more worktrees with new branches.

The branch name will be the same as the worktree directory name.
When no name is given but -b is set, the worktree name is derived from
the base branch (useful for checking out an existing branch).

Examples:
  gmc wt add feature-login                    # Create one worktree
  gmc wt add feat-a feat-b feat-c             # Create multiple worktrees
  gmc wt add feature-login -b main            # Create based on main branch
  gmc wt add feature-login --sync             # Sync base branch before add
  gmc wt add hotfix-bug123 -b release
  gmc wt add -b feat/existing-branch          # Name derived from -b`,
	Args: func(_ *cobra.Command, args []string) error {
		if len(args) == 0 && strings.TrimSpace(wtBaseBranch) == "" {
			return errors.New("requires at least 1 arg or -b/--base flag")
		}
		return nil
	},
	RunE: func(_ *cobra.Command, args []string) error {
		if len(args) == 0 {
			args = []string{wtBaseBranch}
		}
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
	Use:     "remove [name...]",
	Aliases: []string{"rm"},
	Short:   "Remove worktrees (alias: rm)",
	Long: `Remove one or more worktrees.

By default, only removes the worktree directory, keeping the branch.
Use -D to also delete the branch. Use --all to remove all non-protected worktrees.

Examples:
  gmc wt remove feature-login           # Remove one worktree
  gmc wt rm feat-a feat-b feat-c        # Remove multiple worktrees
  gmc wt rm feature-login -D            # Remove worktree and delete branch
  gmc wt rm feature-login -f            # Force remove (ignore dirty state)
  gmc wt rm feature-login --dry-run     # Preview what would be removed
  gmc wt rm --all -D                    # Remove all non-protected worktrees and branches`,
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
		wtClient := newWorktreeClient()
		return runWorktreeRemove(wtClient, args)
	},
}

var wtCloneCmd = &cobra.Command{
	Use:   "clone <url>",
	Short: "Clone a repo into bare + worktree layout",
	Long: `Clone a repository into the bare (.bare) + worktree layout that the rest
of 'gmc wt' expects. This is the starting point for running parallel AI
agents: clone once, then 'gmc wt dup' to fan out.

Creates a .bare directory containing the bare repository and a worktree
for the default branch. For fork workflows, use --upstream to register
the original upstream repo alongside your fork.

Examples:
  # Basic clone into bare + worktree layout
  gmc wt clone https://github.com/user/repo.git

  # Custom project directory name
  gmc wt clone https://github.com/user/repo.git --name my-project

  # Fork workflow: clone your fork, register upstream, work in main/
  gmc wt clone https://github.com/me/fork.git \
    --upstream https://github.com/org/repo.git \
    --name upstream-repo

  # Typical next step: fan out worktrees for parallel AI agents
  cd upstream-repo && gmc wt dup 3`,
	Args: cobra.ExactArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		wtClient := newWorktreeClient()
		return runWorktreeClone(wtClient, args[0])
	},
}

var wtDupCmd = &cobra.Command{
	Use:   "dup [count]",
	Short: "Fan out worktrees for parallel AI agents",
	Long: `Fan out N sibling worktrees so multiple AI coding agents can work in parallel.

Each worktree gets a temporary branch (_dup/<base>/<timestamp>-<n>). Point a
different agent (Claude Code, Codex, Copilot, ...) at each one, compare the
results, then promote the winner back into the current parent worktree with
'gmc wt promote'.

Use --task to copy task context files from the parent worktree into each
candidate. Task files are ordinary files after they are copied.

Examples:
  # Fan out 3 sibling worktrees based on main
  gmc wt dup 3 -b main

  # Fan out candidates with a copied task file
  gmc wt dup 3 --task todo.md

  # Typical parallel workflow with Claude Code / Codex / Copilot
  gmc wt dup 3
  cd ../.dup-1 && claude    # agent 1
  cd ../.dup-2 && codex     # agent 2
  cd ../.dup-3 && copilot   # agent 3

  # When one agent's solution wins, promote it into the current worktree:
  gmc wt promote .dup-1

  # Defaults: count=2, base=main
  gmc wt dup
  gmc wt dup -b dev`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		wtClient := newWorktreeClient()
		return runWorktreeDup(wtClient, args)
	},
}

var wtPromoteCmd = &cobra.Command{
	Use:   "promote <candidate>",
	Short: "Apply a candidate back into the current worktree",
	Long: `Apply a candidate worktree's changes back into the current parent worktree.

Run this from the parent worktree that should receive the winning candidate.
The result is left as uncommitted working tree changes. This command never
commits, pushes, opens PRs, or deletes candidate worktrees.

Examples:
  gmc wt promote .dup-2 --dry-run
  gmc wt promote .dup-2
  gmc wt promote ../.dup-1`,
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
		wtClient := newWorktreeClient()
		return runWorktreePromote(wtClient, args[0])
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
	wtRemoveCmd.Flags().BoolVarP(&wtAll, "all", "a", false, "Remove all non-protected worktrees")

	// Flags for clone command
	wtCloneCmd.Flags().StringVar(&wtUpstream, "upstream", "", "Upstream repository URL (for fork workflow)")
	wtCloneCmd.Flags().StringVar(&wtProjectName, "name", "", "Custom project directory name")

	// Flags for dup command
	wtDupCmd.Flags().StringVarP(&wtDupBase, "base", "b", "main", "Base branch to create from")
	wtDupCmd.Flags().StringArrayVar(&wtDupTasks, "task", nil, "Task context file to copy into each candidate (repeatable)")

	wtPromoteCmd.Flags().BoolVar(&wtDryRun, "dry-run", false,
		"Check whether the candidate can be promoted without changing files")

	// Flags for prune command
	wtPruneCmd.Flags().StringVarP(&wtPruneBase, "base", "b", "", "Base branch to check merge status against")
	wtPruneCmd.Flags().BoolVarP(&wtPruneForce, "force", "f", false, "Force removal even if worktree is dirty")
	wtPruneCmd.Flags().BoolVar(&wtPruneDryRun, "dry-run", false, "Preview what would be removed without making changes")
	wtPruneCmd.Flags().BoolVar(&wtPrunePRAware, "pr-aware", false,
		"Check GitHub PR state before pruning (requires gh CLI)")

	// Flags for pr-review command
	wtPrReviewCmd.Flags().StringVarP(&prRemote, "remote", "r", "",
		"Remote to fetch PR from (auto-detect if not specified)")
	wtCmd.Flags().BoolVar(&wtShowPR, "pr", false,
		"Show review request status for each branch (requires gh or glab CLI)")
	wtListCmd.Flags().BoolVar(&wtShowPR, "pr", false,
		"Show review request status for each branch (requires gh or glab CLI)")

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
	Name           string `json:"name"`
	Path           string `json:"path"`
	Branch         string `json:"branch"`
	Commit         string `json:"commit"`
	Status         string `json:"status"`
	ReviewProvider string `json:"review_provider,omitempty"`
	ReviewNumber   int    `json:"review_number,omitempty"`
	ReviewState    string `json:"review_state,omitempty"`
	ReviewURL      string `json:"review_url,omitempty"`
}

func runWorktreeDefault(wtClient *worktree.Client, _ *cobra.Command) error {
	worktrees, err := wtClient.List()
	if err != nil {
		return err
	}

	filtered := filterBareWorktrees(worktrees)
	reviews := loadWorktreeReviews(wtClient, filtered)

	if outputFormat() == "json" {
		if err := printWorktreeJSON(wtClient, filtered, reviews.Reviews); err != nil {
			return err
		}
		printReviewWarning(errWriter(), reviews)
		return nil
	}

	fmt.Fprintln(outWriter(), "Current Worktrees:")
	printWorktreeTable(wtClient, filtered, reviews.Reviews)

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
	printReviewWarning(outWriter(), reviews)

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
	reviews := loadWorktreeReviews(wtClient, filtered)

	if outputFormat() == "json" {
		if err := printWorktreeJSON(wtClient, filtered, reviews.Reviews); err != nil {
			return err
		}
		printReviewWarning(errWriter(), reviews)
		return nil
	}

	if len(filtered) == 0 {
		fmt.Fprintln(outWriter(), "No worktrees found.")
		return nil
	}

	printWorktreeTable(wtClient, filtered, reviews.Reviews)
	printReviewWarning(outWriter(), reviews)
	return nil
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

func loadWorktreeReviews(wtClient *worktree.Client, worktrees []worktree.Info) worktree.ReviewLookup {
	if !wtShowPR || len(worktrees) == 0 {
		return worktree.ReviewLookup{}
	}
	return wtClient.ReviewStates(worktrees)
}

func printReviewWarning(w io.Writer, reviews worktree.ReviewLookup) {
	if reviews.Warning == "" {
		return
	}
	fmt.Fprintln(w, "Warning: "+reviews.Warning)
}

func formatWorktreeReview(reviews map[string]worktree.ReviewInfo, branch string) string {
	if reviews == nil {
		return ""
	}
	review, ok := reviews[branch]
	if !ok {
		return "-"
	}
	if review.State == "" {
		return fmt.Sprintf("#%d", review.Number)
	}
	return fmt.Sprintf("#%d %s", review.Number, review.State)
}

func formatWorktreeReviewDisplay(reviews map[string]worktree.ReviewInfo, branch string, links bool) string {
	text := formatWorktreeReview(reviews, branch)
	if text == "" || text == "-" || !links {
		return text
	}
	review, ok := reviews[branch]
	if !ok || review.URL == "" || review.Number == 0 {
		return text
	}
	number := fmt.Sprintf("#%d", review.Number)
	linked := fmt.Sprintf("\x1b]8;;%s\x1b\\%s\x1b]8;;\x1b\\", review.URL, number)
	return strings.Replace(text, number, linked, 1)
}

func terminalLinksEnabled(w io.Writer) bool {
	file, ok := w.(*os.File)
	if !ok || os.Getenv("TERM") == "dumb" {
		return false
	}
	return isatty.IsTerminal(file.Fd()) || isatty.IsCygwinTerminal(file.Fd())
}

func padVisibleRight(text string, visibleLen int, width int) string {
	if visibleLen >= width {
		return text
	}
	return text + strings.Repeat(" ", width-visibleLen)
}

func printWorktreeTable(wtClient *worktree.Client, worktrees []worktree.Info, reviews map[string]worktree.ReviewInfo) {
	if len(worktrees) == 0 {
		return
	}

	root := getDisplayRoot(wtClient)
	writer := outWriter()
	links := terminalLinksEnabled(writer)

	maxName := len("Name")
	maxBranch := len("Branch")
	maxPR := len("PR")
	for _, wt := range worktrees {
		name := displayWorktreeName(root, wt.Path)
		if len(name) > maxName {
			maxName = len(name)
		}
		if len(wt.Branch) > maxBranch {
			maxBranch = len(wt.Branch)
		}
		prText := formatWorktreeReview(reviews, wt.Branch)
		if len(prText) > maxPR {
			maxPR = len(prText)
		}
	}

	maxName += 2
	maxBranch += 2
	maxPR += 2

	if reviews != nil {
		fmt.Fprintf(
			writer,
			"%-*s %-*s %-8s %-*s %s\n",
			maxName, "NAME",
			maxBranch, "BRANCH",
			"COMMIT",
			maxPR, "PR",
			"STATUS",
		)
	} else {
		fmt.Fprintf(writer, "%-*s %-*s %-8s %s\n", maxName, "NAME", maxBranch, "BRANCH", "COMMIT", "STATUS")
	}

	for _, wt := range worktrees {
		name := displayWorktreeName(root, wt.Path)
		shortCommit := stringsutil.ShortHash(wt.Commit, 7, "")
		status := resolveWorktreeStatus(wtClient, root, wt)
		if reviews != nil {
			prText := formatWorktreeReview(reviews, wt.Branch)
			prDisplay := formatWorktreeReviewDisplay(reviews, wt.Branch, links)
			fmt.Fprintf(
				writer,
				"%-*s %-*s %-8s %s %s\n",
				maxName, name,
				maxBranch, wt.Branch,
				shortCommit,
				padVisibleRight(prDisplay, len(prText), maxPR),
				status,
			)
		} else {
			fmt.Fprintf(writer, "%-*s %-*s %-8s %s\n", maxName, name, maxBranch, wt.Branch, shortCommit, status)
		}
	}
}

func buildWorktreeJSON(
	wtClient *worktree.Client,
	worktrees []worktree.Info,
	reviews map[string]worktree.ReviewInfo,
) []WorktreeJSON {
	root := getDisplayRoot(wtClient)
	result := make([]WorktreeJSON, 0, len(worktrees))
	for _, wt := range worktrees {
		item := WorktreeJSON{
			Name:   displayWorktreeName(root, wt.Path),
			Path:   wt.Path,
			Branch: wt.Branch,
			Commit: wt.Commit,
			Status: resolveWorktreeStatus(wtClient, root, wt),
		}
		if reviews != nil {
			if review, ok := reviews[wt.Branch]; ok {
				item.ReviewProvider = review.Provider
				item.ReviewNumber = review.Number
				item.ReviewState = review.State
				item.ReviewURL = review.URL
			} else {
				item.ReviewState = "none"
			}
		}
		result = append(result, item)
	}
	return result
}

func printWorktreeJSON(
	wtClient *worktree.Client,
	worktrees []worktree.Info,
	reviews map[string]worktree.ReviewInfo,
) error {
	return printJSON(outWriter(), buildWorktreeJSON(wtClient, worktrees, reviews))
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

	fmt.Fprintf(outWriter(), "Created %d worktrees based on '%s':\n", len(result.Worktrees), opts.BaseBranch)
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
