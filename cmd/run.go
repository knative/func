package cmd

import (
	"errors"

	"github.com/spf13/cobra"
)

func init() {
	root.AddCommand(runCmd)
}

var runCmd = &cobra.Command{
	Use:   "run",
	Short: "Run Service Function locally",
	Long:  "Runs the function locally within an isolated environment.  Modifications to the function trigger a reload.  This holds open the current window with the logs from the running function, and the run is canceled on interrupt.",
	RunE:  run,
}

func run(cmd *cobra.Command, args []string) error {
	return errors.New("Run not implemented.")
}
