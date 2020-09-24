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
	Short: "Runs the Function locally",
	Long: `Runs the Function locally

Runs the project in the deployable image. The project must already have been
built as an OCI container image using the 'build' command.
`,
	RunE: runRun,
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
