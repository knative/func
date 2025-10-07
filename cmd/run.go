package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"strconv"
	"time"

	"github.com/ory/viper"
	"github.com/spf13/cobra"

	"knative.dev/func/pkg/config"
	"knative.dev/func/pkg/docker"
	fn "knative.dev/func/pkg/functions"
	"knative.dev/func/pkg/oci"
)

func NewRunCmd(newClient ClientFactory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "run",
		Short: "Run the function locally",
		Long: `
NAME
	{{rootCmdUse}} run - Run a function locally

SYNOPSIS
	{{rootCmdUse}} run [-r|--registry] [-i|--image] [-e|--env] [--build]
				 [-b|--builder] [--builder-image] [-c|--confirm]
	             [--address] [--json] [-v|--verbose]

DESCRIPTION
	Run the function locally.

	Values provided for flags are not persisted to the function's metadata.

	Containerized Runs
	  You can build your function in a container using the Pack or S2i builders.
	  On the contrary, non-containerized run is achieved via Host builder which
	  will use your host OS' environment to build the function. This builder is
	  currently enabled for Go and Python. Building defaults to using the Host
	  builder when available. You can alter this by using the --builder flag
	  eg: --builder=s2i.

	Process Scaffolding
	  This is an Experimental Feature currently available only to Go and Python
	  projects. When running a function with --builder=host, the function is
	  first wrapped with code which presents it as a process. This "scaffolding"
	  is transient, written for each build or run, and should in most cases be
	  transparent to a function author.

EXAMPLES

	o Run the function locally from within its container.
	  $ {{rootCmdUse}} run

	o Run the function locally from within its container, forcing a rebuild
	  of the container even if no filesystem changes are detected. There are 2
	  builders available for containerized build - 'pack' and 's2i'.
	  $ {{rootCmdUse}} run --build=<builder>

	o Run the function locally on the host with no containerization (Go/Python only).
	  $ {{rootCmdUse}} run --builder=host

	o Run the function locally on a specific address.
	  $ {{rootCmdUse}} run --address='[::]:8081'

	o Run the function locally and output JSON with the service address.
	  $ {{rootCmdUse}} run --json
`,
		SuggestFor: []string{"rnu"},
		PreRunE: bindEnv("build", "builder", "builder-image", "base-image",
			"confirm", "env", "image", "path", "registry",
			"start-timeout", "verbose", "address", "json"),
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runRun(cmd, newClient)
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
	cmd.Flags().StringP("base-image", "", f.Build.BaseImage,
		"Override the base image for your function (host builder only)")
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
	cmd.Flags().String("address", "",
		"Interface and port on which to bind and listen. Default is 127.0.0.1:8080, or an available port if 8080 is not available. ($FUNC_ADDRESS)")
	cmd.Flags().Bool("json", false, "Output as JSON. ($FUNC_JSON)")

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

func runRun(cmd *cobra.Command, newClient ClientFactory) (err error) {
	var (
		cfg runConfig
		f   fn.Function
	)
	cfg = newRunConfig(cmd) // Will add Prompt on upcoming UX refactor

	if f, err = fn.NewFunction(cfg.Path); err != nil {
		return
	}
	if !f.Initialized() {
		return fmt.Errorf(`no function found in current directory.
You need to be inside a function directory to run it.

Try this:
  func create --language go myfunction    Create a new function
  cd myfunction                          Go into the function directory
  func run                               Run the function locally

Or if you have an existing function:
  cd path/to/your/function              Go to your function directory
  func run                              Run the function locally`)
	}

	if err = cfg.Validate(cmd, f); err != nil {
		return
	}

	if f, err = cfg.Configure(f); err != nil { // Updates f with deploy cfg
		return
	}

	container := f.Build.Builder != "host"

	// Ignore the verbose flag if JSON output
	if cfg.JSON {
		cfg.Verbose = false
	}

	// Client
	clientOptions, err := cfg.clientOptions()
	if err != nil {
		return
	}
	if container {
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
	if container {
		var digested bool

		buildOptions, err := cfg.buildOptions()
		if err != nil {
			return err
		}

		// if image was specified, check if its digested and do basic validation
		if cfg.Image != "" {
			digested, err = isDigested(cfg.Image)
			if err != nil {
				return err
			}
			// image was parsed and both digested AND undigested imgs are valid
			f.Build.Image = cfg.Image
		}

		// actual build step
		if !digested {
			if f, _, err = build(cmd, cfg.Build, f, client, buildOptions); err != nil {
				return err
			}
		}
	} else { // if !container
		// dont run digested image without a container
		if cfg.Image != "" {
			digested, err := isDigested(cfg.Image)
			if err != nil {
				return err
			}
			if digested {
				return fmt.Errorf("cannot use digested image with non-containerized builds (--builder=host)")
			}
		}
	}

	// Run
	//
	// Runs the code either via a container or the default host-based runner.
	// For the former, build is required and a container runtime.  For the
	// latter, scaffolding is first applied and the local host must be
	// configured to build/run the language of the function.
	job, err := client.Run(cmd.Context(), f, fn.RunWithAddress(cfg.Address))
	if err != nil {
		return
	}
	defer func() {
		if err = job.Stop(); err != nil {
			fmt.Fprintf(cmd.OutOrStderr(), "Job stop error. %v", err)
		}
	}()

	// Output based on format
	if cfg.JSON {
		// Create JSON output structure
		output := struct {
			Address string `json:"address"`
			Host    string `json:"host"`
			Port    string `json:"port"`
		}{
			Address: fmt.Sprintf("http://%s:%s", job.Host, job.Port),
			Host:    job.Host,
			Port:    job.Port,
		}

		jsonData, err := json.Marshal(output)
		if err != nil {
			return fmt.Errorf("failed to marshal JSON output: %w", err)
		}
		fmt.Fprintln(cmd.OutOrStdout(), string(jsonData))
	} else {
		fmt.Fprintf(cmd.OutOrStderr(), "Function running on %s\n", net.JoinHostPort(job.Host, job.Port))
	}

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

	// Env variables.  may include removals using a "-"
	Env []string

	// StartTimeout optionally adjusts the startup timeout from the client's
	// default of fn.DefaultStartTimeout.
	StartTimeout time.Duration

	// Address is the interface and port to bind (e.g. "0.0.0.0:8081")
	Address string

	// JSON output format
	JSON bool
}

func newRunConfig(cmd *cobra.Command) (c runConfig) {
	c = runConfig{
		buildConfig:  newBuildConfig(),
		Build:        viper.GetString("build"),
		Env:          viper.GetStringSlice("env"),
		StartTimeout: viper.GetDuration("start-timeout"),
		Address:      viper.GetString("address"),
		JSON:         viper.GetBool("json"),
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

	// The other members; build and path; are not part of function
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
	if err = c.buildConfig.Validate(cmd); err != nil {
		return
	}

	// --build can be "auto"|true|false
	if c.Build != "auto" {
		if _, err := strconv.ParseBool(c.Build); err != nil {
			return fmt.Errorf("unrecognized value for --build '%v'.  Accepts 'auto', 'true' or 'false' (or similarly truthy value)", c.Build)
		}
	}

	if f.Build.Builder == "host" && !oci.IsSupported(f.Runtime) {
		return fmt.Errorf("the %q runtime currently requires being run in a container", f.Runtime)
	}

	// When the docker runner respects the StartTimeout, this validation check
	// can be removed
	if c.StartTimeout != 0 && f.Build.Builder != "host" {
		return errors.New("the ability to specify the startup timeout for containerized runs is coming soon")
	}

	return
}
