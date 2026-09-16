package cmd

import (
	"bufio"
	"errors"
	"fmt"
	"os"
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
	Long: `Manage files and directories prepared in new worktrees before hooks run.
Global rules live under worktree in the gmc config file; repository rules in gmc-share.yml override them.
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
	Long: `Add a rule that shares a path with worktrees: copy makes an independent copy, ` +
		`link symlinks the primary worktree's source so writes are shared.
Global rules apply only to new worktrees; run 'gmc wt share sync' to apply them to existing ones.`,
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
		report, err := client.AddSharedResource(args[0], strategy, global)
		printPreparationReport(report)
		if err != nil || global {
			return err
		}
		return syncSharedResources(client)
	},
}

var wtShareRemoveCmd = &cobra.Command{
	Use:     "remove <path>",
	Aliases: []string{"rm"},
	Short:   "Remove or disable a shared resource",
	Long: `Remove a repository rule and disable any inherited global rule for that path.
Existing files are preserved.`,
	Example:           "  gmc wt share remove .local",
	Args:              cobra.ExactArgs(1),
	ValidArgsFunction: completeSharedResources,
	RunE: func(cmd *cobra.Command, args []string) error {
		client := newWorktreeClient()
		global, _ := cmd.Flags().GetBool("global")
		report, err := client.RemoveSharedResource(args[0], global)
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
	Long: `Preview nested projects, shared resources, and dependency directories across worktrees, ` +
		`largest first.`,
	Example:           "  gmc wt share discover",
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
	return syncSharedResources(client)
}

var wtShareSyncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Sync effective resources to all worktrees",
	Long: "Sync global and repository rules to existing worktrees without running hooks " +
		"or replacing existing directories.",
	Args:              cobra.NoArgs,
	ValidArgsFunction: cobra.NoFileCompletions,
	RunE: func(_ *cobra.Command, _ []string) error {
		return syncSharedResources(newWorktreeClient())
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

func syncSharedResources(c *worktree.Client) error {
	report, err := c.SyncAllSharedResources()
	printPreparationReport(report)
	return err
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
