package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/ory/viper"
	"github.com/spf13/cobra"

	"knative.dev/func/pkg/config"
	"knative.dev/func/pkg/docker"
	fn "knative.dev/func/pkg/functions"
)

func NewRunCmd(newClient ClientFactory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "run",
		Short: "Run the function locally",
		Long: `
NAME
	{{rootCmdUse}} run - Run a function locally

SYNOPSIS
	{{rootCmdUse}} run [-t|--container] [-r|--registry] [-i|--image] [-e|--env]
	             [--build] [-b|--builder] [--builder-image] [-c|--confirm]
	             [-v|--verbose]

DESCRIPTION
	Run the function locally.

	Values provided for flags are not persisted to the function's metadata.

	Containerized Runs
	  The --container flag indicates that the function's container should be
	  run rather than running the source code directly.  This may require that
	  the function's container first be rebuilt.  Building the container on or
	  off can be altered using the --build flag.  The default value --build=auto
	  indicates the system should automatically build the container only if
	  necessary.

	Process Scaffolding
	  This is an Experimental Feature currently available only to Go projects.
	  When running a function with --container=false (host-based runs), the
	  function is first wrapped code which presents it as a process.
	  This "scaffolding" is transient, written for each build or run, and should
	  in most cases be transparent to a function author.  However, to customize,
	  or even completely replace this scafolding code, see the 'scaffold'
	  subcommand.

EXAMPLES

	o Run the function locally from within its container.
	  $ {{rootCmdUse}} run

	o Run the function locally from within its container, forcing a rebuild
	  of the container even if no filesysem changes are detected
	  $ {{rootCmdUse}} run --build

	o Run the function locally on the host with no containerization (Go only).
	  $ {{rootCmdUse}} run --container=false
`,
		SuggestFor: []string{"rnu"},
		PreRunE:    bindEnv("build", "builder", "builder-image", "confirm", "container", "env", "image", "path", "registry", "start-timeout", "verbose"),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRun(cmd, args, newClient)
		},
	}

	// Global Config
	cfg, err := config.NewDefault()
	if err != nil {
		fmt.Fprintf(cmd.OutOrStdout(), "error loading config at '%v'. %v\n", config.File(), err)
	}

	// Function Context
	f, _ := fn.NewFunction(effectivePath())
	if f.Initialized() {
		cfg = cfg.Apply(f)
	}

	// Flags
	//
	// Globally-Configurable Flags:
	cmd.Flags().StringP("builder", "b", cfg.Builder,
		fmt.Sprintf("Builder to use when creating the function's container. Currently supported builders are %s.", KnownBuilders()))
	cmd.Flags().StringP("registry", "r", cfg.Registry,
		"Container registry + registry namespace. (ex 'ghcr.io/myuser').  The full image name is automatically determined using this along with function name. ($FUNC_REGISTRY)")

	// Function-Context Flags:
	//   Options whose value is available on the function with context only
	//   (persisted but not globally configurable)
	builderImage := f.Build.BuilderImages[f.Build.Builder]
	cmd.Flags().String("builder-image", builderImage,
		"Specify a custom builder image for use by the builder other than its default. ($FUNC_BUILDER_IMAGE)")
	cmd.Flags().StringP("image", "i", f.Image,
		"Full image name in the form [registry]/[namespace]/[name]:[tag]. This option takes precedence over --registry. Specifying tag is optional. ($FUNC_IMAGE)")
	cmd.Flags().StringArrayP("env", "e", []string{},
		"Environment variable to set in the form NAME=VALUE. "+
			"You may provide this flag multiple times for setting multiple environment variables. "+
			"To unset, specify the environment variable name followed by a \"-\" (e.g., NAME-).")
	cmd.Flags().Duration("start-timeout", f.Run.StartTimeout, fmt.Sprintf("time this function needs in order to start. If not provided, the client default %v will be in effect. ($FUNC_START_TIMEOUT)", fn.DefaultStartTimeout))

	// TODO: Without the "Host" builder enabled, this code-path is unreachable,
	// so remove hidden flag when either the Host builder path is available,
	// or when containerized runs support start-timeout (and ideally both).
	// Also remember to add it to the command help text's synopsis section.
	_ = cmd.Flags().MarkHidden("start-timeout")

	// Static Flags:
	//  Options which have static defaults only
	//  (not globally configurable nor persisted as function metadata)
	cmd.Flags().String("build", "auto",
		"Build the function. [auto|true|false]. ($FUNC_BUILD)")
	cmd.Flags().Lookup("build").NoOptDefVal = "true" // register `--build` as equivalient to `--build=true`
	cmd.Flags().BoolP("container", "t", true,
		"Run the function in a container. ($FUNC_CONTAINER)")

	// Oft-shared flags:
	addConfirmFlag(cmd, cfg.Confirm)
	addPathFlag(cmd)
	addVerboseFlag(cmd, cfg.Verbose)

	// Tab Completion
	if err := cmd.RegisterFlagCompletionFunc("builder", CompleteBuilderList); err != nil {
		fmt.Println("internal: error while calling RegisterFlagCompletionFunc: ", err)
	}

	if err := cmd.RegisterFlagCompletionFunc("builder-image", CompleteBuilderImageList); err != nil {
		fmt.Println("internal: error while calling RegisterFlagCompletionFunc: ", err)
	}

	return cmd
}

func runRun(cmd *cobra.Command, args []string, newClient ClientFactory) (err error) {
	var (
		cfg runConfig
		f   fn.Function
	)
	if cfg, err = newRunConfig(cmd).Prompt(); err != nil {
		return
	}
	if f, err = fn.NewFunction(cfg.Path); err != nil {
		return
	}
	if err = cfg.Validate(cmd, f); err != nil {
		return
	}
	if !f.Initialized() {
		return fn.NewErrNotInitialized(f.Root)
	}
	if f, err = cfg.Configure(f); err != nil { // Updates f with deploy cfg
		return
	}

	// Client
	clientOptions, err := cfg.clientOptions()
	if err != nil {
		return
	}
	if cfg.Container {
		clientOptions = append(clientOptions, fn.WithRunner(docker.NewRunner(cfg.Verbose, os.Stdout, os.Stderr)))
	}
	if cfg.StartTimeout != 0 {
		clientOptions = append(clientOptions, fn.WithStartTimeout(cfg.StartTimeout))
	}

	client, done := newClient(ClientConfig{Verbose: cfg.Verbose}, clientOptions...)
	defer done()

	// Build
	//
	// If requesting to run via the container, build the container if it is
	// either out-of-date or a build was explicitly requested.
	if cfg.Container {
		buildOptions, err := cfg.buildOptions()
		if err != nil {
			return err
		}
		if f, err = build(cmd, cfg.Build, f, client, buildOptions); err != nil {
			return err
		}
	}

	// Run
	//
	// Runs the code either via a container or the default host-based runner.
	// For the former, build is required and a container runtime.  For the
	// latter, scaffolding is first applied and the local host must be
	// configured to build/run the language of the function.
	job, err := client.Run(cmd.Context(), f)
	if err != nil {
		return
	}
	defer func() {
		if err = job.Stop(); err != nil {
			fmt.Fprintf(cmd.OutOrStderr(), "Job stop error. %v", err)
		}
	}()

	fmt.Fprintf(cmd.OutOrStderr(), "Running on host port %v\n", job.Port)

	select {
	case <-cmd.Context().Done():
		if !errors.Is(cmd.Context().Err(), context.Canceled) {
			err = cmd.Context().Err()
		}
	case err = <-job.Errors:
		return
		// Bubble up runtime errors on the optional channel used for async job
		// such as docker containers.
	}

	// NOTE: we do not f.Write() here unlike deploy (and build).
	// running is ephemeral: a run is not affecting the function itself,
	// as opposed to deploy commands, which are actually mutating the current
	// state of the function as it exists on the network.
	// Another way to think of this is that runs are development-centric tests,
	// and thus most likely values changed such as environment variables,
	// builder, etc. would not be expected to persist and affect the next deploy.
	// Run is ephemeral, deploy is persistent.
	return
}

type runConfig struct {
	buildConfig // further embeds config.Global

	// Built instructs building to happen or not
	// Can be 'auto' or a truthy value.
	Build string

	// Container indicates the function should be run in a container.
	// Requires the container be built.
	Container bool

	// Env variables.  may include removals using a "-"
	Env []string

	// StartTimeout optionally adjusts the startup timeout from the client's
	// default of fn.DefaultStartTimeout.
	StartTimeout time.Duration
}

func newRunConfig(cmd *cobra.Command) (c runConfig) {
	c = runConfig{
		buildConfig:  newBuildConfig(),
		Build:        viper.GetString("build"),
		Env:          viper.GetStringSlice("env"),
		Container:    viper.GetBool("container"),
		StartTimeout: viper.GetDuration("start-timeout"),
	}
	// NOTE: .Env should be viper.GetStringSlice, but this returns unparsed
	// results and appears to be an open issue since 2017:
	// https://github.com/spf13/viper/issues/380
	var err error
	if c.Env, err = cmd.Flags().GetStringArray("env"); err != nil {
		fmt.Fprintf(cmd.OutOrStdout(), "error reading envs: %v", err)
	}
	return
}

// Configure the given function.  Updates a function struct with all
// configurable values.  Note that the config alrady includes function's
// current state, as they were passed through via flag defaults.
func (c runConfig) Configure(f fn.Function) (fn.Function, error) {
	var err error
	f = c.buildConfig.Configure(f)

	f.Run.StartTimeout = c.StartTimeout

	f.Run.Envs, err = applyEnvs(f.Run.Envs, c.Env)

	// The other members; build, path, and container; are not part of function
	// state, so are not mentioned here in Configure.
	return f, err
}

func (c runConfig) Prompt() (runConfig, error) {
	var err error

	if c.buildConfig, err = c.buildConfig.Prompt(); err != nil {
		return c, err
	}

	if !interactiveTerminal() || !c.Confirm {
		return c, nil
	}

	// TODO:  prompt for additional settings here
	return c, nil
}

func (c runConfig) Validate(cmd *cobra.Command, f fn.Function) (err error) {
	// Bubble
	if err = c.buildConfig.Validate(); err != nil {
		return
	}

	// --build can be "auto"|true|false
	if c.Build != "auto" {
		if _, err := strconv.ParseBool(c.Build); err != nil {
			return fmt.Errorf("unrecognized value for --build '%v'.  Accepts 'auto', 'true' or 'false' (or similarly truthy value)", c.Build)
		}
	}

	// There is currently no local host runner implemented, so specifying
	// --container=false should return an informative error for runtimes other
	// than Go  that is more helpful than the cryptic, though correct, error
	// from the Client that it was instantated without a runner.
	// TODO: modify this check when the local host runner is available to
	// only generate this error when --container==false && the --language is
	// not yet implemented.
	if !c.Container && f.Runtime != "go" {
		return errors.New("the ability to run functions outside of a container via 'func run' is coming soon.")
	}

	// When the docker runner respects the StartTimeout, this validation check
	// can be removed
	if c.StartTimeout != 0 && c.Container {
		return errors.New("the ability to specify the startup timeout for containerized runs is coming soon")
	}

	return
}
