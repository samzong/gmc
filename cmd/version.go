package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

// 默认版本信息，会被编译时的链接参数覆盖
var (
	Version   = "dev"
	versionCmd = &cobra.Command{
		Use:   "version",
		Short: "显示GMA版本信息",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("GMA 版本 v%s\n", Version)
		},
	}
)

func init() {
	rootCmd.AddCommand(versionCmd)
} 