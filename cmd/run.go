package cmd

import (
	"fmt"

	"github.com/AlecAivazis/survey/v2"
	"github.com/ory/viper"
	"github.com/spf13/cobra"
	"knative.dev/client/pkg/util"

	fn "knative.dev/kn-plugin-func"
	"knative.dev/kn-plugin-func/buildpacks"
	"knative.dev/kn-plugin-func/docker"
	"knative.dev/kn-plugin-func/progress"
)

func init() {
	root.AddCommand(NewRunCmd(newRunClient))
}

func newRunClient(cfg runConfig) *fn.Client {
	runner := docker.NewRunner()
	runner.Verbose = cfg.Verbose

	// builder fields
	builder := buildpacks.NewBuilder()
	listener := progress.New()
	builder.Verbose = cfg.Verbose
	listener.Verbose = cfg.Verbose
	return fn.New(
		fn.WithBuilder(builder),
		fn.WithProgressListener(listener),
		fn.WithRegistry(cfg.Registry),
		fn.WithRunner(runner),
		fn.WithVerbose(cfg.Verbose),
	)
}

type runClientFn func(runConfig) *fn.Client

func NewRunCmd(clientFn runClientFn) *cobra.Command {

	cmd := &cobra.Command{
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
	}

	cmd.Flags().StringArrayP("env", "e", []string{},
		"Environment variable to set in the form NAME=VALUE. "+
			"You may provide this flag multiple times for setting multiple environment variables. "+
			"To unset, specify the environment variable name followed by a \"-\" (e.g., NAME-).")
	setPathFlag(cmd)

	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		return runRun(cmd, args, clientFn)
	}

	return cmd
}

func runRun(cmd *cobra.Command, args []string, clientFn runClientFn) (err error) {
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

	client := clientFn(config)

	if !function.Built() {
		ans := struct {
			Build bool
		}{}
		qs := survey.Question{
			Name: "Build",
			Prompt: &survey.Confirm{
				Message: "Looks like the function has not been built yet (building can take a few minutes), would you like to build it now?:",
				Default: false,
			},
			Validate: survey.Required,
		}
		err := survey.Ask([]*survey.Question{&qs}, &ans)
		if err == nil && ans.Build {
			client.Build(cmd.Context(), config.Path)
		} else if err != nil {
			fmt.Printf("Function will not be built: %v\n", err)
		}
	}
	return client.Run(cmd.Context(), config.Path)
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

	// Required for Build to be triggered from Run
	Registry string

	// Required for Build to be triggered from Run
	Builder string
}

func newRunConfig(cmd *cobra.Command) (c runConfig, err error) {
	envToUpdate, envToRemove, err := envFromCmd(cmd)
	if err != nil {
		return
	}

	return runConfig{
		Path:        viper.GetString("path"),
		Verbose:     viper.GetBool("verbose"), // defined on root
		EnvToUpdate: envToUpdate,
		EnvToRemove: envToRemove,
		Registry:    viper.GetString("registry"),
		Builder:     viper.GetString("builder"),
	}, nil
}
