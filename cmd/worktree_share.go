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
)

var wtShareCmd = &cobra.Command{
	Use:   "share",
	Short: "Manage shared resources for worktrees",
	Long: `Manage shared resources that are automatically synced to all worktrees.

If run without arguments, it opens an interactive mode to manage resources.

Config file is stored at .gmc-shared.yml in the worktree root.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		wtClient := worktree.NewClient(worktree.Options{Verbose: verbose})
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
		wtClient := worktree.NewClient(worktree.Options{Verbose: verbose})

		strategy := worktree.ResourceStrategy(shareStrategy)
		// If strategy not explicitly set via flag, ask interactively
		if !cmd.Flags().Changed("strategy") {
			reader := bufio.NewReader(os.Stdin)
			fmt.Println("Strategy:")
			fmt.Println("  1. copy - each worktree gets its own copy")
			fmt.Println("  2. link - symlink to shared source")
			fmt.Print("Select [1/2, default: 1]: ")
			strategyStr, _ := reader.ReadString('\n')
			strategyStr = strings.TrimSpace(strings.ToLower(strategyStr))
			if strategyStr == "2" || strategyStr == "link" || strategyStr == "l" {
				strategy = worktree.StrategySymlink
			} else {
				strategy = worktree.StrategyCopy
			}
		}

		if err := wtClient.AddSharedResource(args[0], strategy); err != nil {
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
	RunE: func(cmd *cobra.Command, args []string) error {
		wtClient := worktree.NewClient(worktree.Options{Verbose: verbose})
		if err := wtClient.RemoveSharedResource(args[0]); err != nil {
			return err
		}
		fmt.Println("Note: This does not remove the files from existing worktrees, only from the config.")
		return nil
	},
}

var wtShareListCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"ls"},
	Short:   "List configured shared resources",
	RunE: func(cmd *cobra.Command, args []string) error {
		wtClient := worktree.NewClient(worktree.Options{Verbose: verbose})
		cfg, _, err := wtClient.LoadSharedConfig()
		if err != nil {
			return err
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

var wtShareSyncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Manually sync shared resources to all worktrees",
	RunE: func(cmd *cobra.Command, args []string) error {
		wtClient := worktree.NewClient(worktree.Options{Verbose: verbose})
		return wtClient.SyncAllSharedResources()
	},
}

func init() {
	wtCmd.AddCommand(wtShareCmd)
	wtShareCmd.AddCommand(wtShareAddCmd)
	wtShareCmd.AddCommand(wtShareRemoveCmd)
	wtShareCmd.AddCommand(wtShareListCmd)
	wtShareCmd.AddCommand(wtShareSyncCmd)

	wtShareAddCmd.Flags().StringVarP(&shareStrategy, "strategy", "s", "copy", "Sync strategy: copy or link")
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
			if err := c.SyncAllSharedResources(); err != nil {
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

	// If user entered a relative path and we're in a worktree, prepend worktree name
	if currentWorktree != "" && !strings.Contains(path, "/") {
		path = filepath.Join(currentWorktree, path)
		fmt.Printf("Using full path: %s\n", path)
	}

	fmt.Println("\nStrategy:")
	fmt.Println("  1. copy - each worktree gets its own copy")
	fmt.Println("  2. link - symlink to shared source")
	fmt.Print("\nSelect [1/2, default: 1]: ")
	strategyStr, _ := reader.ReadString('\n')
	strategyStr = strings.TrimSpace(strings.ToLower(strategyStr))

	var strategy worktree.ResourceStrategy
	if strategyStr == "2" || strategyStr == "link" || strategyStr == "l" {
		strategy = worktree.StrategySymlink
	} else {
		strategy = worktree.StrategyCopy
	}

	if err := c.AddSharedResource(path, strategy); err != nil {
		fmt.Printf("Error adding resource: %v\n", err)
	} else {
		fmt.Println("Resource added!")
		// Ask to sync immediately
		fmt.Print("Sync to all existing worktrees now? [Y/n]: ")
		syncInput, _ := reader.ReadString('\n')
		syncInput = strings.TrimSpace(strings.ToLower(syncInput))
		if syncInput == "" || syncInput == "y" || syncInput == "yes" {
			c.SyncAllSharedResources()
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
	if err := c.RemoveSharedResource(res.Path); err != nil {
		fmt.Printf("Error removing resource: %v\n", err)
	} else {
		fmt.Printf("Resource '%s' removed from config.\n", res.Path)
	}
}

func promptContinue(reader *bufio.Reader) {
	fmt.Print("\nPress Enter to continue...")
	reader.ReadString('\n')
}

func askToSyncAll(c *worktree.Client) error {
	// In CLI mode, we might not want to interactively ask unless it's a TTY
	// But the user request specifically asked for "sync to all existing worktrees"
	// Let's do it automatically or log a suggestion if not interactive?
	// For now, let's just do it automatically as implied by the user request "Every time I modify it..."
	fmt.Println("Syncing changes to all worktrees...")
	return c.SyncAllSharedResources()
}
