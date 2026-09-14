package cmd

import (
	"fmt"

	"github.com/samzong/gmc/internal/shell"
	"github.com/spf13/cobra"
)

var wtInitCmd = &cobra.Command{
	Use:   "init <bash|zsh|fish>",
	Short: "Generate shell integration script",
	Long:  `Generate a shell wrapper that lets 'gmc wt switch' change the current directory.`,
	Example: `  eval "$(gmc wt init bash)"
  eval "$(gmc wt init zsh)"
  gmc wt init fish | source`,
	ValidArgs: []string{"bash", "zsh", "fish"},
	Args:      cobra.MatchAll(cobra.ExactArgs(1), cobra.OnlyValidArgs),
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
