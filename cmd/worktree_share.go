package cmd

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/samzong/gmc/internal/worktree"
	"github.com/spf13/cobra"
)

var (
	shareStrategy string
	discoverAuto  bool
)

var wtShareCmd = &cobra.Command{
	Use:   "share",
	Short: "Manage shared resources for worktrees",
	Long: `Manage resources prepared before hooks run in new worktrees.

Global defaults live under worktree in the selected gmc config file.
Repository rules in the git common dir's gmc-share.yml override global paths.
Run without arguments for interactive repository management.`,
	Example: "  gmc wt share discover\n  gmc wt share list --global",
	Args:    cobra.NoArgs,
	RunE: func(_ *cobra.Command, _ []string) error {
		return runWorktreeShareInteractive(newWorktreeClient())
	},
}

var wtShareAddCmd = &cobra.Command{
	Use:   "add <path>",
	Short: "Add or update a shared resource",
	Long: `Add a file or directory shared across worktrees in each repository.

copy creates an independent copy; link shares a writable source through a symlink.
Global defaults apply when creating worktrees. Adding a global rule does not sync
existing worktrees. Run share sync in a repository to apply its effective rules.
Global and pattern rules use the primary worktree as their source, never a
directory found only in another linked worktree.

Quote patterns such as '**/.local' to match nested project paths. Pattern scans
skip dependency, build, and .local directory interiors. Linking dependency environments
shares writable state across branches; prefer package-manager caches where possible.`,
	Example: "  gmc wt share add .env --strategy copy\n" +
		"  gmc wt share add .local --strategy link --global\n" +
		"  gmc wt share add '**/node_modules' --strategy link --global",
	Args:              cobra.ExactArgs(1),
	ValidArgsFunction: cobra.FixedCompletions(nil, cobra.ShellCompDirectiveDefault),
	RunE: func(cmd *cobra.Command, args []string) error {
		client := newWorktreeClient()
		strategy := worktree.ResourceStrategy(shareStrategy)
		if !cmd.Flags().Changed("strategy") {
			strategy = promptStrategy(bufio.NewReader(os.Stdin))
		}
		global, _ := cmd.Flags().GetBool("global")
		if global {
			report, err := client.AddGlobalSharedResource(args[0], strategy)
			printPreparationReport(report)
			return err
		}
		report, err := client.AddSharedResource(args[0], strategy)
		printPreparationReport(report)
		if err != nil {
			return err
		}
		return askToSyncAll(client)
	},
}

var wtShareRemoveCmd = &cobra.Command{
	Use:     "remove <path>",
	Aliases: []string{"rm"},
	Short:   "Remove or disable a shared resource",
	Long: `Remove a repository rule and disable any inherited global rule for that path.
Use --global to remove a global default. Existing files are preserved.`,
	Example:           "  gmc wt share remove .local\n  gmc wt share remove .local --global",
	Args:              cobra.ExactArgs(1),
	ValidArgsFunction: completeSharedResources,
	RunE: func(cmd *cobra.Command, args []string) error {
		client := newWorktreeClient()
		global, _ := cmd.Flags().GetBool("global")
		var report worktree.Report
		var err error
		if global {
			report, err = client.RemoveGlobalSharedResource(args[0])
		} else {
			report, err = client.RemoveSharedResource(args[0])
		}
		printPreparationReport(report)
		if err == nil {
			fmt.Fprintln(errWriter(), "Existing files are preserved.")
		}
		return err
	},
}

type ShareJSON struct {
	Path     string `json:"path"`
	Strategy string `json:"strategy"`
	Origin   string `json:"origin"`
	Disabled bool   `json:"disabled"`
}

var wtShareListCmd = &cobra.Command{
	Use:               "list",
	Aliases:           []string{"ls"},
	Short:             "List effective shared resources",
	Args:              cobra.NoArgs,
	ValidArgsFunction: cobra.NoFileCompletions,
	RunE: func(cmd *cobra.Command, _ []string) error {
		global, _ := cmd.Flags().GetBool("global")
		cfg, err := loadListedSharedConfig(newWorktreeClient(), global)
		if err != nil {
			return err
		}
		if outputFormat() == "json" {
			items := make([]ShareJSON, len(cfg.Resources))
			for i, res := range cfg.Resources {
				items[i] = ShareJSON{Path: res.Path, Strategy: string(res.Strategy), Origin: res.Origin, Disabled: res.Disabled}
			}
			return printJSON(cmd.OutOrStdout(), items)
		}
		if len(cfg.Resources) == 0 {
			fmt.Fprintln(cmd.OutOrStdout(), "No shared resources configured.")
			return nil
		}
		for _, res := range cfg.Resources {
			fmt.Fprintf(cmd.OutOrStdout(), "%s (%s; %s%s)\n", res.Path, res.Strategy, res.Origin, disabledSuffix(res.Disabled))
		}
		return nil
	},
}

var wtShareDiscoverCmd = &cobra.Command{
	Use:   "discover",
	Short: "Inspect sharing and dependency hotspots",
	Long: `Inspect nested projects, shared resources, and dependency directories across repository worktrees.

The preview lists directories first, largest first, with readable sizes and sharing
status. Project manifests are summarized instead of listed as resources.
Use --output json for full source paths, matched rules, and per-resource guidance.

The preview includes effective hooks, which run after sharing only during worktree
creation. Dependency environments and build outputs are reported with guidance;
only safe configuration-file candidates are automatically added. Size estimates
are apparent bytes, not exclusive disk usage or guaranteed savings.
Sources outside the primary worktree are inspected but not propagated by global
or pattern rules.

Use --auto to add candidates and sync effective rules to existing worktrees.
Hooks are not executed by discover or share sync.`,
	Example:           "  gmc wt share discover\n  gmc wt share discover --output json\n  gmc wt share discover --auto",
	Args:              cobra.NoArgs,
	ValidArgsFunction: cobra.NoFileCompletions,
	RunE:              runWorktreeShareDiscover,
}

func runWorktreeShareDiscover(cmd *cobra.Command, _ []string) error {
	return discoverSharedResources(cmd, newWorktreeClient())
}

func discoverSharedResources(cmd *cobra.Command, client *worktree.Client) error {
	if discoverAuto && cmd.Flags().Changed("dry-run") {
		return errors.New("--auto and --dry-run are mutually exclusive")
	}
	results, err := client.Discover(worktree.DiscoverOptions{IncludeConfigured: true})
	if err != nil {
		return err
	}
	cfg, err := client.LoadEffectiveSharedConfig()
	if err != nil {
		return err
	}
	if outputFormat() == "json" {
		if results == nil {
			results = []worktree.DiscoverResult{}
		}
		if err := printJSON(cmd.OutOrStdout(), struct {
			Resources []worktree.DiscoverResult `json:"resources"`
			Hooks     []HookJSON                `json:"hooks"`
		}{results, hookJSON(cfg.Hooks)}); err != nil {
			return err
		}
	} else {
		printDiscoverOverview(cmd, results, cfg.Hooks)
	}
	if !discoverAuto {
		return nil
	}
	report, err := client.AddDiscoveredResources(results)
	printPreparationReport(report)
	if err != nil {
		return err
	}
	report, err = client.SyncAllSharedResources()
	printPreparationReport(report)
	return err
}

var wtShareSyncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Sync effective resources to all worktrees",
	Long: "Sync global and repository rules to existing worktrees without running hooks " +
		"or replacing existing directories.",
	Example:           "  gmc wt share sync",
	Args:              cobra.NoArgs,
	ValidArgsFunction: cobra.NoFileCompletions,
	RunE: func(_ *cobra.Command, _ []string) error {
		report, err := newWorktreeClient().SyncAllSharedResources()
		printPreparationReport(report)
		return err
	},
}

func loadListedSharedConfig(client *worktree.Client, global bool) (*worktree.SharedConfig, error) {
	if global {
		cfg, _, err := client.LoadGlobalSharedConfig()
		if err == nil {
			for i := range cfg.Resources {
				cfg.Resources[i].Origin = "global"
			}
			for i := range cfg.Hooks {
				cfg.Hooks[i].Origin = "global"
			}
		}
		return cfg, err
	}
	return client.LoadEffectiveSharedConfig()
}

func printPreparationReport(report worktree.Report) {
	for _, event := range report.Events {
		fmt.Fprintln(errWriter(), event.Message)
	}
}

func disabledSuffix(disabled bool) string {
	if disabled {
		return "; disabled"
	}
	return ""
}

func completeSharedResources(cmd *cobra.Command, args []string, prefix string) ([]string, cobra.ShellCompDirective) {
	if len(args) != 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	global, _ := cmd.Flags().GetBool("global")
	cfg, err := loadListedSharedConfig(newWorktreeClient(), global)
	if err != nil {
		return nil, cobra.ShellCompDirectiveError
	}
	var paths []string
	for _, res := range cfg.Resources {
		if strings.HasPrefix(res.Path, prefix) {
			paths = append(paths, res.Path)
		}
	}
	return paths, cobra.ShellCompDirectiveNoFileComp
}

func init() {
	wtCmd.AddCommand(wtShareCmd)
	wtShareCmd.AddCommand(wtShareAddCmd, wtShareRemoveCmd, wtShareListCmd, wtShareSyncCmd, wtShareDiscoverCmd)
	for _, command := range []*cobra.Command{wtShareAddCmd, wtShareRemoveCmd, wtShareListCmd} {
		command.Flags().Bool("global", false, "Use defaults in the selected gmc config file")
	}
	wtShareAddCmd.Flags().StringVarP(&shareStrategy, "strategy", "s", "copy", "Sync strategy: copy or link")
	_ = wtShareAddCmd.RegisterFlagCompletionFunc("strategy", completeStrategies)
	wtShareDiscoverCmd.Flags().BoolVar(&discoverAuto, "auto", false, "Add safe candidates and sync effective rules")
	wtShareDiscoverCmd.Flags().Bool("dry-run", true, "Preview only (also the default without --auto)")
	wtShareDiscoverCmd.MarkFlagsMutuallyExclusive("auto", "dry-run")
}

func runWorktreeShareInteractive(c *worktree.Client) error {
	reader := bufio.NewReader(os.Stdin)

	for {
		fmt.Fprintln(errWriter(), "--- Manage Shared Resources ---")

		cfg, err := c.LoadEffectiveSharedConfig()
		if err != nil {
			return err
		}

		if len(cfg.Resources) > 0 {
			fmt.Fprintln(errWriter(), "Current Resources:")
			for i, res := range cfg.Resources {
				fmt.Fprintf(errWriter(), "  %d. %s (%s; %s%s)\n",
					i+1, res.Path, res.Strategy, res.Origin, disabledSuffix(res.Disabled))
			}
		} else {
			fmt.Fprintln(errWriter(), "No shared resources configured.")
		}
		fmt.Fprintln(errWriter())
		fmt.Fprintln(errWriter(), "Options:")
		fmt.Fprintln(errWriter(), "  a. Add new resource")
		fmt.Fprintln(errWriter(), "  r. Remove resource")
		fmt.Fprintln(errWriter(), "  s. Sync all worktrees now")
		fmt.Fprintln(errWriter(), "  q. Quit")
		fmt.Fprint(errWriter(), "\nSelect option: ")

		input, _ := reader.ReadString('\n')
		input = strings.TrimSpace(strings.ToLower(input))

		switch input {
		case "a":
			promptAddResource(c, reader)
		case "r":
			promptRemoveResource(c, reader, cfg)
		case "s":
			report, err := c.SyncAllSharedResources()
			printPreparationReport(report)
			if err != nil {
				fmt.Fprintf(errWriter(), "Error syncing: %v\n", err)
			} else {
				fmt.Fprintln(errWriter(), "Sync complete!")
			}
			promptContinue(reader)
		case "q":
			return nil
		default:
			fmt.Fprintln(errWriter(), "Invalid option")
		}
	}
}

func promptAddResource(c *worktree.Client, reader *bufio.Reader) {
	root, _ := c.GetWorktreeRoot()

	cwd, _ := os.Getwd()
	currentWorktree := ""
	if strings.HasPrefix(cwd, root) {
		rel, _ := filepath.Rel(root, cwd)
		parts := strings.SplitN(rel, string(filepath.Separator), 2)
		if len(parts) > 0 && parts[0] != "." && parts[0] != ".bare" {
			currentWorktree = parts[0]
		}
	}

	fmt.Fprintf(errWriter(), "\nProject root: %s\n", root)
	if currentWorktree != "" {
		fmt.Fprintf(errWriter(), "Current worktree: %s\n", currentWorktree)
	}
	fmt.Fprint(errWriter(), "\nPath: ")
	path, _ := reader.ReadString('\n')
	path = strings.TrimSpace(path)
	if path == "" {
		return
	}

	strategy := promptStrategy(reader)

	report, err := c.AddSharedResource(path, strategy)
	printPreparationReport(report)
	if err != nil {
		fmt.Fprintf(errWriter(), "Error adding resource: %v\n", err)
	} else {
		fmt.Fprintln(errWriter(), "Resource added!")
		fmt.Fprint(errWriter(), "Sync to all existing worktrees now? [Y/n]: ")
		syncInput, _ := reader.ReadString('\n')
		syncInput = strings.TrimSpace(strings.ToLower(syncInput))
		if syncInput == "" || syncInput == "y" || syncInput == "yes" {
			report, err := c.SyncAllSharedResources()
			printPreparationReport(report)
			if err != nil {
				fmt.Fprintf(errWriter(), "Warning: failed to sync: %v\n", err)
			}
		}
	}
}

func promptRemoveResource(c *worktree.Client, reader *bufio.Reader, cfg *worktree.SharedConfig) {
	if len(cfg.Resources) == 0 {
		return
	}
	fmt.Fprint(errWriter(), "\nEnter number to remove: ")
	numStr, _ := reader.ReadString('\n')
	var num int
	_, err := fmt.Sscanf(strings.TrimSpace(numStr), "%d", &num)
	if err != nil || num < 1 || num > len(cfg.Resources) {
		fmt.Fprintln(errWriter(), "Invalid selection")
		return
	}

	res := cfg.Resources[num-1]
	report, err := c.RemoveSharedResource(res.Path)
	printPreparationReport(report)
	if err != nil {
		fmt.Fprintf(errWriter(), "Error removing resource: %v\n", err)
	} else {
		fmt.Fprintf(errWriter(), "Resource '%s' removed from config.\n", res.Path)
	}
}

func promptStrategy(reader *bufio.Reader) worktree.ResourceStrategy {
	fmt.Fprintln(errWriter(), "\nStrategy:")
	fmt.Fprintln(errWriter(), "  1. copy - each worktree gets its own copy")
	fmt.Fprintln(errWriter(), "  2. link - symlink to shared source")
	fmt.Fprint(errWriter(), "\nSelect [1/2, default: 2]: ")
	input, _ := reader.ReadString('\n')
	input = strings.TrimSpace(strings.ToLower(input))
	if input == "1" || input == "copy" || input == "c" {
		return worktree.StrategyCopy
	}
	return worktree.StrategySymlink
}

func promptContinue(reader *bufio.Reader) {
	fmt.Fprint(errWriter(), "\nPress Enter to continue...")
	_, _ = reader.ReadString('\n')
}

func askToSyncAll(c *worktree.Client) error {
	report, err := c.SyncAllSharedResources()
	printPreparationReport(report)
	return err
}
