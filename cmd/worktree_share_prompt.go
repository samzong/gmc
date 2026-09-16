package cmd

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/samzong/gmc/internal/worktree"
)

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
			if err := syncSharedResources(c); err != nil {
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
			if err := syncSharedResources(c); err != nil {
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
