package cmd

import (
	"fmt"

	"github.com/ory/viper"
	"github.com/spf13/cobra"
	"knative.dev/client/pkg/util"

	fn "github.com/boson-project/func"
	"github.com/boson-project/func/docker"
)

func init() {
	// Add the run command as a subcommand of root.
	root.AddCommand(runCmd)
	runCmd.Flags().StringArrayP("env", "e", []string{}, "Environment variable to set in the form NAME=VALUE. "+
		"You may provide this flag multiple times for setting multiple environment variables. "+
		"To unset, specify the environment variable name followed by a \"-\" (e.g., NAME-).")
	runCmd.Flags().StringP("path", "p", cwd(), "Path to the project directory (Env: $FUNC_PATH)")
}

var runCmd = &cobra.Command{
	Use:   "run",
	Short: "Run the function locally",
	Long: `Run the function locally

Runs the function locally in the current directory or in the directory
specified by --path flag. The function must already have been built with the 'build' command.
`,
	Example: `
# Build function's image first
kn func build

# Run it locally as a container
kn func run
`,
	SuggestFor: []string{"rnu"},
	PreRunE:    bindEnv("path"),
	RunE:       runRun,
}

func runRun(cmd *cobra.Command, args []string) (err error) {
	config, err := newRunConfig(cmd)
	if err != nil {
		return
	}

	function, err := fn.NewFunction(config.Path)
	if err != nil {
		return
	}

	function.Envs, err = mergeEnvs(function.Envs, config.EnvToUpdate, config.EnvToRemove)
	if err != nil {
		return
	}

	err = function.WriteConfig()
	if err != nil {
		return
	}

	// Check if the Function has been initialized
	if !function.Initialized() {
		return fmt.Errorf("the given path '%v' does not contain an initialized function", config.Path)
	}

	runner := docker.NewRunner()
	runner.Verbose = config.Verbose

	client := fn.New(
		fn.WithRunner(runner),
		fn.WithVerbose(config.Verbose))

	err = client.Run(cmd.Context(), config.Path)
	return
}

type runConfig struct {
	// Path of the Function implementation on local disk. Defaults to current
	// working directory of the process.
	Path string

	// Verbose logging.
	Verbose bool

	// Envs passed via cmd to be added/updated
	EnvToUpdate *util.OrderedMap

	// Envs passed via cmd to removed
	EnvToRemove []string
}

func newRunConfig(cmd *cobra.Command) (runConfig, error) {
	envToUpdate, envToRemove, err := envFromCmd(cmd)
	if err != nil {
		return runConfig{}, err
	}

	return runConfig{
		Path:        viper.GetString("path"),
		Verbose:     viper.GetBool("verbose"), // defined on root
		EnvToUpdate: envToUpdate,
		EnvToRemove: envToRemove,
	}, nil
}
