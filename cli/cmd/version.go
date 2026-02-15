package cmd

import (
	"fmt"
	"runtime"

	"github.com/spf13/cobra"
)

// Version is set at build time via ldflags.
var Version = "dev"

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the kindling CLI version",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("kindling %s (%s/%s)\n", Version, runtime.GOOS, runtime.GOARCH)
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}
