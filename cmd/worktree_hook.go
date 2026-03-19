package cmd

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/samzong/gmc/internal/worktree"
	"github.com/spf13/cobra"
)

var wtHookCmd = &cobra.Command{
	Use:   "hook",
	Short: "Manage hooks executed after worktree creation",
	Long: `Manage hooks that are executed after shared resources are synced to a new worktree.

Hooks run in the target worktree directory and are useful for running
package installation commands like 'pnpm install'.

Config file is stored at the repository's shared git common dir (for example .git/gmc-share.yml or .bare/gmc-share.yml).`,
	RunE: func(_ *cobra.Command, _ []string) error {
		wtClient := newWorktreeClient()
		return runHookList(wtClient)
	},
}

var wtHookAddCmd = &cobra.Command{
	Use:   "add <command>",
	Short: "Add a hook to run after worktree creation",
	Long: `Add a hook command that will be executed after shared resources are synced to a worktree.

Hooks are executed in the target worktree directory after resources are synced.
This is useful for running package installation commands like 'pnpm install'.

Example:
  gmc wt hook add "pnpm install" -d "Install dependencies"
  gmc wt hook add "pnpm install && pnpm build"`,
	Args: cobra.ExactArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		wtClient := newWorktreeClient()
		hook := worktree.Hook{
			Cmd:  args[0],
			Desc: hookDesc,
		}
		report, err := wtClient.AddHook(hook)
		printWorktreeReport(report)
		return err
	},
}

var wtHookRemoveCmd = &cobra.Command{
	Use:   "remove <index>",
	Short: "Remove a hook by index",
	Args:  cobra.ExactArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		wtClient := newWorktreeClient()
		index, err := strconv.Atoi(strings.TrimSpace(args[0]))
		if err != nil {
			return fmt.Errorf("invalid hook index: %s", args[0])
		}
		report, err := wtClient.RemoveHook(index - 1)
		printWorktreeReport(report)
		return err
	},
}

func runHookList(c *worktree.Client) error {
	cfg, _, err := c.LoadSharedConfig()
	if err != nil {
		return err
	}

	if len(cfg.Hooks) == 0 {
		fmt.Println("No hooks configured.")
		return nil
	}

	fmt.Println("Hooks:")
	for i, hook := range cfg.Hooks {
		if hook.Desc != "" {
			fmt.Printf("  %d. %s (%s)\n", i+1, hook.Cmd, hook.Desc)
		} else {
			fmt.Printf("  %d. %s\n", i+1, hook.Cmd)
		}
	}
	return nil
}

var (
	hookDesc string
)

func init() {
	wtCmd.AddCommand(wtHookCmd)
	wtHookCmd.AddCommand(wtHookAddCmd)
	wtHookCmd.AddCommand(wtHookRemoveCmd)
	wtHookCmd.SetHelpFunc(func(cmd *cobra.Command, args []string) {
		fmt.Println(cmd.Long)
		fmt.Println("\nAvailable commands:")
		fmt.Println("  add     Add a hook to run after worktree creation")
		fmt.Println("  remove  Remove a hook by index")
		fmt.Println("\nUse \"gmc wt hook [command] --help\" for more info.")
	})

	wtHookAddCmd.Flags().StringVarP(&hookDesc, "desc", "d", "", "Description for the hook")
}
