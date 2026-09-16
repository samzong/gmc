package cmd

import (
	"errors"
	"fmt"

	"github.com/charmbracelet/huh"
	"github.com/samzong/gmc/internal/shell"
	"github.com/samzong/gmc/internal/worktree"
	"github.com/spf13/cobra"
)

var wtSwitchCmd = &cobra.Command{
	Use:   "switch",
	Short: "Interactively switch to another worktree",
	Long: `Interactively select a worktree and change into it.
Requires the shell integration from 'gmc wt init'; without it only the path is printed.`,
	RunE: func(_ *cobra.Command, _ []string) error {
		return runWorktreeSwitch(newWorktreeClient())
	},
}

func runWorktreeSwitch(wtClient *worktree.Client) error {
	worktrees, err := wtClient.List()
	if err != nil {
		return err
	}

	filtered := filterBareWorktrees(worktrees)
	if len(filtered) == 0 {
		return errors.New("no worktrees found")
	}

	root := getDisplayRoot(wtClient)

	options := make([]huh.Option[string], 0, len(filtered))
	for _, wt := range filtered {
		name := displayWorktreeName(root, wt.Path)
		options = append(options, huh.NewOption(fmt.Sprintf("%s (%s)", name, wt.Branch), wt.Path))
	}

	var selected string
	if err := huh.NewSelect[string]().Title("Select Worktree").Options(options...).Value(&selected).Run(); err != nil {
		return err
	}

	if err := shell.ChangeDirectory(selected); err != nil {
		return err
	}

	fmt.Println(selected)
	return nil
}
