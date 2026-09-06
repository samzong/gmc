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
	Short: "Manage hooks after worktree creation",
	Long: `Manage commands run in a new worktree after shared resources are prepared.

Global defaults live under worktree.hooks in the selected gmc config file.
Repository hooks live alongside shared resources in gmc-share.yml. Give a hook an
ID to replace or disable it in a repository. Hooks run in effective list order.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		return runHookListCommand(cmd, newWorktreeClient())
	},
}

var wtHookAddCmd = &cobra.Command{
	Use:   "add <command>",
	Short: "Add or replace a worktree hook",
	Long: `Configure a command executed after sharing during worktree creation.

A repository hook with the same ID replaces an inherited global hook. Global
commands run in every repository; use shell conditions for project-specific work.
Adding a hook does not execute it.`,
	Example: "  gmc wt hook add 'pnpm install' --id install --desc 'Install dependencies'\n" +
		"  gmc wt hook add 'test ! -f uv.lock || uv sync' --id python --global",
	Args:              cobra.ExactArgs(1),
	ValidArgsFunction: cobra.NoFileCompletions,
	RunE: func(cmd *cobra.Command, args []string) error {
		client := newWorktreeClient()
		id, _ := cmd.Flags().GetString("id")
		hook := worktree.Hook{ID: id, Cmd: args[0], Desc: hookDesc}
		global, _ := cmd.Flags().GetBool("global")
		var report worktree.Report
		var err error
		if global {
			report, err = client.AddGlobalHook(hook)
		} else {
			report, err = client.AddHook(hook)
		}
		printPreparationReport(report)
		return err
	},
}

var wtHookRemoveCmd = &cobra.Command{
	Use:     "remove <index-or-id>",
	Aliases: []string{"rm"},
	Short:   "Remove or disable a hook",
	Long: `Remove a hook by its ID or the one-based index shown by hook list.

Repository removal also disables an inherited global hook. With --global, use
the index from hook list --global to remove a global default.`,
	Example:           "  gmc wt hook remove install\n  gmc wt hook remove 1 --global",
	Args:              cobra.ExactArgs(1),
	ValidArgsFunction: completeHooks,
	RunE: func(cmd *cobra.Command, args []string) error {
		client := newWorktreeClient()
		global, _ := cmd.Flags().GetBool("global")
		value := strings.TrimSpace(args[0])
		index, parseErr := strconv.Atoi(value)
		var report worktree.Report
		var err error
		if parseErr != nil {
			report, err = client.RemoveHookByID(value, global)
		} else if global {
			report, err = client.RemoveGlobalHook(index - 1)
		} else {
			report, err = client.RemoveHook(index - 1)
		}
		printPreparationReport(report)
		return err
	},
}

var wtHookListCmd = &cobra.Command{
	Use:               "list",
	Aliases:           []string{"ls"},
	Short:             "List effective hooks in execution order",
	Args:              cobra.NoArgs,
	ValidArgsFunction: cobra.NoFileCompletions,
	RunE: func(cmd *cobra.Command, _ []string) error {
		return runHookListCommand(cmd, newWorktreeClient())
	},
}

type HookJSON struct {
	Index    int    `json:"index"`
	ID       string `json:"id,omitempty"`
	Cmd      string `json:"cmd"`
	Desc     string `json:"desc,omitempty"`
	Origin   string `json:"origin"`
	Disabled bool   `json:"disabled"`
}

func hookJSON(hooks []worktree.Hook) []HookJSON {
	items := make([]HookJSON, len(hooks))
	for i, hook := range hooks {
		items[i] = HookJSON{
			Index: i + 1, ID: hook.ID, Cmd: hook.Cmd, Desc: hook.Desc, Origin: hook.Origin, Disabled: hook.Disabled,
		}
	}
	return items
}

func runHookListCommand(cmd *cobra.Command, c *worktree.Client) error {
	global, _ := cmd.Flags().GetBool("global")
	cfg, err := loadListedSharedConfig(c, global)
	if err != nil {
		return err
	}
	if outputFormat() == "json" {
		return printJSON(cmd.OutOrStdout(), hookJSON(cfg.Hooks))
	}
	printHooks(cmd, cfg.Hooks)
	return nil
}

func printHooks(cmd *cobra.Command, hooks []worktree.Hook) {
	if len(hooks) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "No hooks configured.")
		return
	}
	for i, hook := range hooks {
		fmt.Fprintf(cmd.OutOrStdout(), "  %d. %s [%s%s]", i+1, hook.Cmd, hook.Origin, disabledSuffix(hook.Disabled))
		if hook.ID != "" {
			fmt.Fprintf(cmd.OutOrStdout(), " id=%s", hook.ID)
		}
		if hook.Desc != "" {
			fmt.Fprintf(cmd.OutOrStdout(), " (%s)", hook.Desc)
		}
		fmt.Fprintln(cmd.OutOrStdout())
	}
}

func completeHooks(cmd *cobra.Command, args []string, prefix string) ([]string, cobra.ShellCompDirective) {
	if len(args) != 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	global, _ := cmd.Flags().GetBool("global")
	cfg, err := loadListedSharedConfig(newWorktreeClient(), global)
	if err != nil {
		return nil, cobra.ShellCompDirectiveError
	}
	var values []string
	for i, hook := range cfg.Hooks {
		value := hook.ID
		if value == "" {
			value = strconv.Itoa(i + 1)
		}
		if strings.HasPrefix(value, prefix) {
			values = append(values, value)
		}
	}
	return values, cobra.ShellCompDirectiveNoFileComp
}

var hookDesc string

func init() {
	wtCmd.AddCommand(wtHookCmd)
	wtHookCmd.AddCommand(wtHookAddCmd, wtHookRemoveCmd, wtHookListCmd)
	wtHookCmd.PersistentFlags().Bool("global", false, "Use defaults in the selected gmc config file")
	wtHookAddCmd.Flags().StringVarP(&hookDesc, "desc", "d", "", "Description for the hook")
	wtHookAddCmd.Flags().String("id", "", "Stable hook ID for repository overrides and removal")
}
