package cmd

import (
	"fmt"
	builddebug "runtime/debug"
	"strings"

	"github.com/spf13/cobra"
)

var (
	Version       = "dev"
	BuildTime     = "unknown"
	readBuildInfo = builddebug.ReadBuildInfo

	versionCmd = &cobra.Command{
		Use:   "version",
		Short: "Show gmc version information",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			_, err := fmt.Fprintf(outWriter(), "gmc version %s\n", versionString())
			return err
		},
	}
)

func init() {
	rootCmd.AddCommand(versionCmd)
}

func resolvedVersion() string {
	if Version != "" && Version != "dev" {
		return strings.TrimPrefix(Version, "v")
	}
	info, ok := readBuildInfo()
	if !ok {
		return "dev"
	}
	if info.Main.Version != "" && info.Main.Version != "(devel)" {
		return strings.TrimPrefix(info.Main.Version, "v")
	}
	return "dev"
}

func versionString() string {
	return fmt.Sprintf("%s (built at %s)", resolvedVersion(), BuildTime)
}
