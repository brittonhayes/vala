package cmd

import (
	"fmt"
	"runtime/debug"

	"github.com/spf13/cobra"
)

// Version is set at build time via -ldflags; it falls back to VCS info.
var Version = "dev"

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the vala version",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("vala", resolveVersion())
	},
}

func resolveVersion() string {
	if Version != "dev" {
		return Version
	}
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" && info.Main.Version != "(devel)" {
		return info.Main.Version
	}
	return Version
}
