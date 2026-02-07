package cmd

import (
	"errors"
	"fmt"
	"path/filepath"

	"github.com/charmbracelet/huh"
	"github.com/samzong/gmc/internal/shell"
	"github.com/samzong/gmc/internal/worktree"
	"github.com/spf13/cobra"
)

var wtSwitchCmd = &cobra.Command{
	Use:   "switch",
	Short: "Interactively switch to another worktree",
	Long: `Interactively select a worktree and switch to it.

Requires shell integration. If not set up, run:
  eval "$(gmc wt init zsh)"  # or bash/fish

Without shell integration, this command will only print the path.`,
	RunE: func(_ *cobra.Command, _ []string) error {
		wtClient := newWorktreeClient()
		return runWorktreeSwitch(wtClient)
	},
}

func runWorktreeSwitch(wtClient *worktree.Client) error {
	if !wtClient.IsBareWorktree() {
		return errors.New("not in a bare worktree setup")
	}

	worktrees, err := wtClient.List()
	if err != nil {
		return err
	}

	filtered := filterBareWorktrees(worktrees)
	if len(filtered) == 0 {
		return errors.New("no worktrees found")
	}

	root, _ := wtClient.GetWorktreeRoot()

	options := make([]huh.Option[string], 0, len(filtered))
	for _, wt := range filtered {
		name := filepath.Base(wt.Path)
		if rel, err := filepath.Rel(root, wt.Path); err == nil && root != "" {
			name = rel
		}
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
