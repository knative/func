package cmd

import (
	"fmt"

	"github.com/ory/viper"
	"github.com/spf13/cobra"

	"github.com/boson-project/faas"
	"github.com/boson-project/faas/docker"
)

func init() {
	// Add the run command as a subcommand of root.
	root.AddCommand(runCmd)
	runCmd.Flags().StringArrayP("env", "e", []string{}, "Sets environment variables for the Function.")
	runCmd.Flags().StringP("path", "p", cwd(), "Path to the Function project directory - $FAAS_PATH")
}

var runCmd = &cobra.Command{
	Use:   "run",
	Short: "Runs the Function locally",
	Long: `Runs the Function locally

Runs the Function project in the current directory or in the directory
specified by the -p or --path flag in the deployable image. The project must
already have been built as an OCI container image using the 'build' command.
`,
	SuggestFor: []string{"rnu"},
	PreRunE:    bindEnv("path"),
	RunE:       runRun,
}

func runRun(cmd *cobra.Command, args []string) (err error) {
	config := newRunConfig(cmd)

	function, err := faas.NewFunction(config.Path)
	if err != nil {
		return
	}

	function.EnvVars = mergeEnvVarsMaps(function.EnvVars, config.EnvVars)

	err = function.WriteConfig()
	if err != nil {
		return 
	}

	// Check if the Function has been initialized
	if !function.Initialized() {
		return fmt.Errorf("the given path '%v' does not contain an initialized Function.", config.Path)
	}

	runner := docker.NewRunner()
	runner.Verbose = config.Verbose

	client := faas.New(
		faas.WithRunner(runner),
		faas.WithVerbose(config.Verbose))

	return client.Run(config.Path)
}

type runConfig struct {
	// Path of the Function implementation on local disk. Defaults to current
	// working directory of the process.
	Path string

	// Verbose logging.
	Verbose bool

	EnvVars map[string]string
}

func newRunConfig(cmd *cobra.Command) runConfig {
	return runConfig{
		Path:    viper.GetString("path"),
		Verbose: viper.GetBool("verbose"), // defined on root
		EnvVars: envVarsFromCmd(cmd),
	}
}
