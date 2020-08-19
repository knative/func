package cmd

import (
	"github.com/ory/viper"
	"github.com/spf13/cobra"

	"github.com/boson-project/faas"
	"github.com/boson-project/faas/docker"
)

func init() {
	// Add the run command as a subcommand of root.
	root.AddCommand(runCmd)
}

var runCmd = &cobra.Command{
	Use:   "run",
	Short: "Run Function locally",
	Long:  "Runs the function locally within an isolated environment.  Modifications to the Function trigger a reload.  This holds open the current window with the logs from the running Function, and the run is canceled on interrupt.",
	RunE:  runRun,
}

func runRun(cmd *cobra.Command, args []string) (err error) {
	var (
		path    = "" // defaults to current working directory
		verbose = viper.GetBool("verbose")
	)

	if len(args) == 1 {
		path = args[0]
	}

	runner := docker.NewRunner()
	runner.Verbose = verbose

	client := faas.New(
		faas.WithRunner(runner),
		faas.WithVerbose(verbose))

	return client.Run(path)
}
