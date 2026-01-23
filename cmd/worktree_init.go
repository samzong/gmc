package cmd

import (
	"fmt"

	"github.com/samzong/gmc/internal/shell"
	"github.com/spf13/cobra"
)

var wtInitCmd = &cobra.Command{
	Use:   "init <bash|zsh|fish>",
	Short: "Generate shell integration script",
	Long: `Generate a shell integration script for gmc.

Add the following to your shell's rc file:

  # For bash (~/.bashrc)
  eval "$(gmc wt init bash)"

  # For zsh (~/.zshrc)
  eval "$(gmc wt init zsh)"

  # For fish (~/.config/fish/config.fish)
  gmc wt init fish | source

After this, 'gmc wt switch' will be able to change your working directory.`,
	Args: cobra.ExactArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		return runWorktreeInit(args[0])
	},
}

func runWorktreeInit(shellType string) error {
	wrapper := shell.GenerateWrapper(shellType)
	if wrapper == "" {
		return fmt.Errorf("unsupported shell: %s (supported: bash, zsh, fish)", shellType)
	}
	fmt.Print(wrapper)
	return nil
}
