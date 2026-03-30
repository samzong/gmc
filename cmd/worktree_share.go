package cmd

import (
	"bufio"
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
	Long: `Manage shared resources that are automatically synced to all worktrees.

If run without arguments, it opens an interactive mode to manage resources.

Config file is stored at the repository's shared git common dir (for example .git/gmc-share.yml or .bare/gmc-share.yml).`,
	RunE: func(_ *cobra.Command, _ []string) error {
		wtClient := newWorktreeClient()
		return runWorktreeShareInteractive(wtClient)
	},
}

var wtShareAddCmd = &cobra.Command{
	Use:   "add <path>",
	Short: "Add or update a shared resource",
	Long: `Add a file or directory to be shared across all worktrees.

Strategies:
  - copy: Copies the file/directory (good for .env files that need isolation)
  - link: Creates a symlink (good for large model directories)`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		wtClient := newWorktreeClient()

		strategy := worktree.ResourceStrategy(shareStrategy)
		// If strategy not explicitly set via flag, ask interactively
		if !cmd.Flags().Changed("strategy") {
			strategy = promptStrategy(bufio.NewReader(os.Stdin))
		}

		report, err := wtClient.AddSharedResource(args[0], strategy)
		printWorktreeReport(report)
		if err != nil {
			return err
		}
		return askToSyncAll(wtClient)
	},
}

var wtShareRemoveCmd = &cobra.Command{
	Use:     "remove <path>",
	Aliases: []string{"rm"},
	Short:   "Remove a shared resource from config",
	Args:    cobra.ExactArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		wtClient := newWorktreeClient()
		report, err := wtClient.RemoveSharedResource(args[0])
		printWorktreeReport(report)
		if err != nil {
			return err
		}
		fmt.Println("Note: This does not remove the files from existing worktrees, only from the config.")
		return nil
	},
}

type ShareJSON struct {
	Path     string `json:"path"`
	Strategy string `json:"strategy"`
}

var wtShareListCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"ls"},
	Short:   "List configured shared resources",
	RunE: func(_ *cobra.Command, _ []string) error {
		wtClient := newWorktreeClient()
		cfg, _, err := wtClient.LoadSharedConfig()
		if err != nil {
			return err
		}

		if outputFormat() == "json" {
			items := make([]ShareJSON, len(cfg.Resources))
			for i, res := range cfg.Resources {
				items[i] = ShareJSON{Path: res.Path, Strategy: string(res.Strategy)}
			}
			return printJSON(outWriter(), items)
		}

		if len(cfg.Resources) == 0 {
			fmt.Println("No shared resources configured.")
			return nil
		}

		fmt.Println("Shared Resources:")
		for _, res := range cfg.Resources {
			fmt.Printf("  - %s (%s)\n", res.Path, res.Strategy)
		}
		return nil
	},
}

var wtShareDiscoverCmd = &cobra.Command{
	Use:   "discover",
	Short: "Discover files that should be shared across worktrees",
	Long: `Scan the main worktree for files that should be shared across worktrees.

By default, shows a preview of discovered files (dry-run mode).
Use --auto to actually add discovered files and sync them.`,
	RunE: func(_ *cobra.Command, _ []string) error {
		wtClient := newWorktreeClient()

		results, err := wtClient.Discover(worktree.DiscoverOptions{})
		if err != nil {
			return err
		}

		if len(results) == 0 {
			fmt.Println("No new shareable files discovered.")
			return nil
		}

		var copyResults, linkResults []worktree.DiscoverResult
		for _, r := range results {
			switch r.Strategy {
			case worktree.StrategyCopy:
				copyResults = append(copyResults, r)
			case worktree.StrategySymlink:
				linkResults = append(linkResults, r)
			}
		}

		fmt.Println("Discovered shareable files:")
		fmt.Println()
		if len(copyResults) > 0 {
			fmt.Println("Copy strategy (isolated per worktree):")
			for _, r := range copyResults {
				fmt.Printf("  %s\n", r.Path)
			}
		}
		if len(linkResults) > 0 {
			fmt.Println("Link strategy (shared, saves disk):")
			for _, r := range linkResults {
				fmt.Printf("  %s\n", r.Path)
			}
		}

		if !discoverAuto {
			fmt.Printf("\n[dry-run] Would add %d copy + %d link resources\n", len(copyResults), len(linkResults))
			return nil
		}

		fmt.Println()
		addReport, addErr := wtClient.AddDiscoveredResources(results)
		printWorktreeReport(addReport)
		if addErr != nil {
			return addErr
		}

		fmt.Println("\nSyncing to all worktrees...")
		report, err := wtClient.SyncAllSharedResources()
		printWorktreeReport(report)
		if err != nil {
			return err
		}
		fmt.Println("Done.")
		return nil
	},
}

var wtShareSyncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Manually sync shared resources to all worktrees",
	RunE: func(_ *cobra.Command, _ []string) error {
		wtClient := newWorktreeClient()
		report, err := wtClient.SyncAllSharedResources()
		printWorktreeReport(report)
		return err
	},
}

func init() {
	wtCmd.AddCommand(wtShareCmd)
	wtShareCmd.AddCommand(wtShareAddCmd)
	wtShareCmd.AddCommand(wtShareRemoveCmd)
	wtShareCmd.AddCommand(wtShareListCmd)
	wtShareCmd.AddCommand(wtShareSyncCmd)
	wtShareCmd.AddCommand(wtShareDiscoverCmd)

	wtShareAddCmd.Flags().StringVarP(&shareStrategy, "strategy", "s", "copy", "Sync strategy: copy or link")
	_ = wtShareAddCmd.RegisterFlagCompletionFunc("strategy", completeStrategies)

	wtShareDiscoverCmd.Flags().BoolVar(&discoverAuto, "auto", false, "Actually add discovered items and sync")
	wtShareDiscoverCmd.Flags().Bool("dry-run", true, "Preview mode (default behavior)")
}

func runWorktreeShareInteractive(c *worktree.Client) error {
	reader := bufio.NewReader(os.Stdin)

	for {
		// Clear screen strictly speaking is not easy cross-platform without libs, just print newlines
		fmt.Println("--- Manage Shared Resources ---")

		cfg, _, err := c.LoadSharedConfig()
		if err != nil {
			return err
		}

		if len(cfg.Resources) > 0 {
			fmt.Println("Current Resources:")
			for i, res := range cfg.Resources {
				fmt.Printf("  %d. %s (%s)\n", i+1, res.Path, res.Strategy)
			}
		} else {
			fmt.Println("No shared resources configured.")
		}
		fmt.Println()
		fmt.Println("Options:")
		fmt.Println("  a. Add new resource")
		fmt.Println("  r. Remove resource")
		fmt.Println("  s. Sync all worktrees now")
		fmt.Println("  q. Quit")
		fmt.Print("\nSelect option: ")

		input, _ := reader.ReadString('\n')
		input = strings.TrimSpace(strings.ToLower(input))

		switch input {
		case "a":
			promptAddResource(c, reader)
		case "r":
			promptRemoveResource(c, reader, cfg)
		case "s":
			report, err := c.SyncAllSharedResources()
			printWorktreeReport(report)
			if err != nil {
				fmt.Printf("Error syncing: %v\n", err)
			} else {
				fmt.Println("Sync complete!")
			}
			promptContinue(reader)
		case "q":
			return nil
		default:
			fmt.Println("Invalid option")
		}
	}
}

func promptAddResource(c *worktree.Client, reader *bufio.Reader) {
	root, _ := c.GetWorktreeRoot()

	// Detect current worktree
	cwd, _ := os.Getwd()
	currentWorktree := ""
	if strings.HasPrefix(cwd, root) {
		rel, _ := filepath.Rel(root, cwd)
		parts := strings.SplitN(rel, string(filepath.Separator), 2)
		if len(parts) > 0 && parts[0] != "." && parts[0] != ".bare" {
			currentWorktree = parts[0]
		}
	}

	fmt.Printf("\nProject root: %s\n", root)
	if currentWorktree != "" {
		fmt.Printf("Current worktree: %s\n", currentWorktree)
	}
	fmt.Print("\nPath: ")
	path, _ := reader.ReadString('\n')
	path = strings.TrimSpace(path)
	if path == "" {
		return
	}

	strategy := promptStrategy(reader)

	report, err := c.AddSharedResource(path, strategy)
	printWorktreeReport(report)
	if err != nil {
		fmt.Printf("Error adding resource: %v\n", err)
	} else {
		fmt.Println("Resource added!")
		// Ask to sync immediately
		fmt.Print("Sync to all existing worktrees now? [Y/n]: ")
		syncInput, _ := reader.ReadString('\n')
		syncInput = strings.TrimSpace(strings.ToLower(syncInput))
		if syncInput == "" || syncInput == "y" || syncInput == "yes" {
			report, err := c.SyncAllSharedResources()
			printWorktreeReport(report)
			if err != nil {
				fmt.Printf("Warning: failed to sync: %v\n", err)
			}
		}
	}
}

func promptRemoveResource(c *worktree.Client, reader *bufio.Reader, cfg *worktree.SharedConfig) {
	if len(cfg.Resources) == 0 {
		return
	}
	fmt.Print("\nEnter number to remove: ")
	numStr, _ := reader.ReadString('\n')
	var num int
	_, err := fmt.Sscanf(strings.TrimSpace(numStr), "%d", &num)
	if err != nil || num < 1 || num > len(cfg.Resources) {
		fmt.Println("Invalid selection")
		return
	}

	res := cfg.Resources[num-1]
	report, err := c.RemoveSharedResource(res.Path)
	printWorktreeReport(report)
	if err != nil {
		fmt.Printf("Error removing resource: %v\n", err)
	} else {
		fmt.Printf("Resource '%s' removed from config.\n", res.Path)
	}
}

func promptStrategy(reader *bufio.Reader) worktree.ResourceStrategy {
	fmt.Println("\nStrategy:")
	fmt.Println("  1. copy - each worktree gets its own copy")
	fmt.Println("  2. link - symlink to shared source")
	fmt.Print("\nSelect [1/2, default: 1]: ")
	input, _ := reader.ReadString('\n')
	input = strings.TrimSpace(strings.ToLower(input))
	if input == "2" || input == "link" || input == "l" {
		return worktree.StrategySymlink
	}
	return worktree.StrategyCopy
}

func promptContinue(reader *bufio.Reader) {
	fmt.Print("\nPress Enter to continue...")
	_, _ = reader.ReadString('\n')
}

func askToSyncAll(c *worktree.Client) error {
	report, err := c.SyncAllSharedResources()
	printWorktreeReport(report)
	return err
}
