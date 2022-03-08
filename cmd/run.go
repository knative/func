package cmd

import (
	"context"
	"errors"
	"fmt"

	"github.com/ory/viper"
	"github.com/spf13/cobra"
	"knative.dev/client/pkg/util"

	fn "knative.dev/kn-plugin-func"
)

func NewRunCmd(options ...fn.Option) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "run",
		Short: "Run the function locally",
		Long: `Run the function locally

Runs the function locally in the current directory or in the directory
specified by --path flag. The function must already have been built with the 'build' command.
`,
		Example: `
# Build function's image first
{{.Name}} build

# Run it locally as a container
{{.Name}} run
`,
		SuggestFor: []string{"rnu"},
		PreRunE:    bindEnv("build", "path", "registry"),
	}

	cmd.Flags().StringArrayP("env", "e", []string{},
		"Environment variable to set in the form NAME=VALUE. "+
			"You may provide this flag multiple times for setting multiple environment variables. "+
			"To unset, specify the environment variable name followed by a \"-\" (e.g., NAME-).")
	cmd.Flags().BoolP("build", "b", false, "Build the function only if the function has not been built before")
	setPathFlag(cmd)

	cmd.SetHelpFunc(defaultTemplatedHelp)

	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		return runRun(cmd, args, options...)
	}

	return cmd
}

func runRun(cmd *cobra.Command, args []string, options ...fn.Option) (err error) {
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

	err = function.Write()
	if err != nil {
		return
	}

	// Check if the Function has been initialized
	if !function.Initialized() {
		return fmt.Errorf("the given path '%v' does not contain an initialized function", config.Path)
	}

	// Client for use running (and potentially building), using the config
	// gathered plus any additional option overrieds (such as for providing
	// mocks when testing for builder and runner)
	options = append([]fn.Option{fn.WithRegistry(config.Registry)}, options...)
	client, done := NewClient(DefaultNamespace, config.Verbose, options...)
	defer done()

	// Build if not built and --build
	if config.Build && !function.Built() {
		if err = client.Build(cmd.Context(), config.Path); err != nil {
			return
		}
	}

	// Run the Function at path
	job, err := client.Run(cmd.Context(), config.Path)
	if err != nil {
		return
	}
	defer job.Stop()

	fmt.Fprintf(cmd.OutOrStderr(), "Function started on port %v\n", job.Port)

	select {
	case <-cmd.Context().Done():
		if !errors.Is(cmd.Context().Err(), context.Canceled) {
			err = cmd.Context().Err()
		}
		return
	case err = <-job.Errors:
		return
	}
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

	// Perform build if function hasn't been built yet
	Build bool

	// Registry for the build tag if building
	Registry string
}

func newRunConfig(cmd *cobra.Command) (c runConfig, err error) {
	envToUpdate, envToRemove, err := envFromCmd(cmd)
	if err != nil {
		return
	}

	return runConfig{
		Build:       viper.GetBool("build"),
		Path:        viper.GetString("path"),
		Verbose:     viper.GetBool("verbose"), // defined on root
		Registry:    viper.GetString("registry"),
		EnvToUpdate: envToUpdate,
		EnvToRemove: envToRemove,
	}, nil
}
