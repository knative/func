package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

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
