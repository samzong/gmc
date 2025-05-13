package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var (
	Version   = "dev"
	versionCmd = &cobra.Command{
		Use:   "version",
		Short: "Show gma version information",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("gma version v%s\n", Version)
		},
	}
)

func init() {
	rootCmd.AddCommand(versionCmd)
} 