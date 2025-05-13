package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var (
	Version   = "dev"
	versionCmd = &cobra.Command{
		Use:   "version",
		Short: "Show GMA version information",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("GMA version v%s\n", Version)
		},
	}
)

func init() {
	rootCmd.AddCommand(versionCmd)
} 