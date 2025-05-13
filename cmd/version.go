package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var (
	// Version information, will be set during build
	Version   = "dev"
	BuildTime = "unknown"

	versionCmd = &cobra.Command{
		Use:   "version",
		Short: "Show gma version information",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("gma version %s (built at %s)\n", Version, BuildTime)
		},
	}
)

func init() {
	rootCmd.AddCommand(versionCmd)
}
