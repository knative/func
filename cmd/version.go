package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

// Version
// Printed on subcommand `version` or flag `--version`
const Version = "v0.2.1"

func init() {
	root.AddCommand(versionCmd)
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version",
	Run:   version,
}

func version(cmd *cobra.Command, args []string) {
	fmt.Println(Version)
}
