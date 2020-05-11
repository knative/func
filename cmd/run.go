package cmd

import (
	"github.com/ory/viper"
	"github.com/spf13/cobra"

	"github.com/boson-project/faas"
	"github.com/boson-project/faas/appsody"
)

func init() {
	// Add the run command as a subcommand of root.
	root.AddCommand(runCmd)
}

var runCmd = &cobra.Command{
	Use:   "run",
	Short: "Run Service Function locally",
	Long:  "Runs the function locally within an isolated environment.  Modifications to the function trigger a reload.  This holds open the current window with the logs from the running function, and the run is canceled on interrupt.",
	RunE:  run,
}

func run(cmd *cobra.Command, args []string) (err error) {
	var (
		path    = "" // defaults to current working directory
		verbose = viper.GetBool("verbose")
	)

	if len(args) == 1 {
		path = args[0]
	}

	runner := appsody.NewRunner()
	runner.Verbose = verbose

	client, err := faas.New(
		faas.WithRunner(runner),
		faas.WithVerbose(verbose))
	if err != nil {
		return
	}

	return client.Run(path)
}
