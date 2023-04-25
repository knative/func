package cmd

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/ory/viper"
	"github.com/spf13/cobra"
	"knative.dev/client-pkg/pkg/util"

	"knative.dev/func/pkg/builders"
	"knative.dev/func/pkg/builders/buildpacks"
	"knative.dev/func/pkg/builders/s2i"
	"knative.dev/func/pkg/config"
	fn "knative.dev/func/pkg/functions"
)

func NewRunCmd(newClient ClientFactory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "run",
		Short: "Run the function locally",
		Long: `Run the function locally

Runs the function locally in the current directory or in the directory
specified by --path flag.

Building
By default the function will be built if never built, or if changes are detected
to the function's source.  Use --build to override this behavior.
Also a builder strategy (pack, s2i) can be chosen using the --builder option.
Default builder is pack.

`,
		Example: `
# Run the function locally, building if necessary
{{rootCmdUse}} run

# Run the function locally, building if necessary, with --builder option
{{rootCmdUse}} run --builder s2i

# Run the function, forcing a rebuild of the image.
#   This is useful when the function's image was manually deleted, necessitating
#   A rebuild even when no changes have been made the function's source.
{{rootCmdUse}} run --build

# Run the function, forcing a rebuild of the image with --builder option.
{{rootCmdUse}} run --build --builder s2i

# Run the function's existing image, disabling auto-build.
#   This is useful when filesystem changes have been made, but one wishes to
#   run the previously built image without rebuilding.
{{rootCmdUse}} run --build=false

`,
		SuggestFor: []string{"rnu"},
		PreRunE:    bindEnv("build", "path", "builder", "registry", "verbose"),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRun(cmd, args, newClient)
		},
	}

	cfg, err := config.NewDefault()
	if err != nil {
		fmt.Fprintf(cmd.OutOrStdout(), "error loading config at '%v'. %v\n", config.File(), err)
	}

	cmd.Flags().StringArrayP("env", "e", []string{},
		"Environment variable to set in the form NAME=VALUE. "+
			"You may provide this flag multiple times for setting multiple environment variables. "+
			"To unset, specify the environment variable name followed by a \"-\" (e.g., NAME-).")
	cmd.Flags().StringP("build", "b", "auto", "Build the function. [auto|true|false].")
	cmd.Flags().Lookup("build").NoOptDefVal = "true" // --build is equivalient to --build=true
	cmd.Flags().StringP("builder", "", cfg.Builder,
		fmt.Sprintf("Builder to use when creating the function's container. Currently supported builders are %s. ($FUNC_BUILDER)", KnownBuilders()))
	cmd.Flags().StringP("registry", "r", "", "Registry + namespace part of the image if building, ex 'quay.io/myuser' ($FUNC_REGISTRY)")
	addPathFlag(cmd)
	addVerboseFlag(cmd, cfg.Verbose)

	// Tab Completion
	if err := cmd.RegisterFlagCompletionFunc("builder", CompleteBuilderList); err != nil {
		fmt.Println("internal: error while calling RegisterFlagCompletionFunc: ", err)
	}

	return cmd
}

func runRun(cmd *cobra.Command, args []string, newClient ClientFactory) (err error) {
	cfg, err := newRunConfig(cmd)
	if err != nil {
		return
	}

	function, err := fn.NewFunction(cfg.Path)
	if err != nil {
		return
	}
	if !function.Initialized() {
		return fmt.Errorf("the given path '%v' does not contain an initialized function", cfg.Path)
	}
	var updated int
	function.Run.Envs, updated, err = mergeEnvs(function.Run.Envs, cfg.EnvToUpdate, cfg.EnvToRemove)
	if err != nil {
		return
	}
	if updated > 0 {
		err = function.Write()
		if err != nil {
			return
		}
	}

	// Concrete implementations (ex builder) vary based on final effective config
	var builder fn.Builder
	if cfg.Builder == builders.Pack {
		builder = buildpacks.NewBuilder(
			buildpacks.WithName(builders.Pack),
			buildpacks.WithVerbose(cfg.Verbose))
	} else if cfg.Builder == builders.S2I {
		builder = s2i.NewBuilder(
			s2i.WithName(builders.S2I),
			s2i.WithVerbose(cfg.Verbose))
	} else {
		return builders.ErrUnknownBuilder{Name: cfg.Builder, Known: KnownBuilders()}
	}

	// Client for use running (and potentially building), using the config
	// gathered plus any additional option overrieds (such as for providing
	// mocks when testing for builder and runner)
	client, done := newClient(ClientConfig{Verbose: cfg.Verbose}, fn.WithRegistry(cfg.Registry), fn.WithBuilder(builder))
	defer done()

	// Build?
	// If --build was set to 'auto', only build if client detects the function
	// is stale (has either never been built or has had filesystem modifications
	// since the last build).
	if cfg.Build == "auto" {
		if !function.Built() {
			if function, err = client.Build(cmd.Context(), function); err != nil {
				return
			}
			if err = function.Write(); err != nil {
				return err
			}
		}
		fmt.Println("Function already built.  Use --build to force a rebuild.")
		// Otherwise, --build should parse to a truthy value which indicates an explicit
		// override.
	} else {
		build, err := strconv.ParseBool(cfg.Build)
		if err != nil {
			return fmt.Errorf("unrecognized value for --build '%v'.  accepts 'auto', 'true' or 'false' (or similarly truthy value)", build)
		}
		if build {
			if function, err = client.Build(cmd.Context(), function); err != nil {
				return err
			}
			if err = function.Write(); err != nil {
				return err
			}
		} else {
			fmt.Println("Function build disabled.")
		}

	}

	// Run the function at path
	job, err := client.Run(cmd.Context(), function)
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
	// Path of the function implementation on local disk. Defaults to current
	// working directory of the process.
	Path string

	// Verbose logging.
	Verbose bool

	// Envs passed via cmd to be added/updated
	EnvToUpdate *util.OrderedMap

	// Envs passed via cmd to removed
	EnvToRemove []string

	// Perform build.  Acceptable values are the keyword 'auto', or a truthy
	// value such as 'true', 'false, '1' or '0'.
	Build string

	// Builder strategy if building
	Builder string

	// Registry for the build tag if building
	Registry string
}

func newRunConfig(cmd *cobra.Command) (cfg runConfig, err error) {
	envToUpdate, envToRemove, err := envFromCmd(cmd)
	if err != nil {
		return
	}
	cfg = runConfig{
		Build:       viper.GetString("build"),
		Path:        viper.GetString("path"),
		Verbose:     viper.GetBool("verbose"), // defined on root
		Builder:     viper.GetString("builder"),
		Registry:    viper.GetString("registry"),
		EnvToUpdate: envToUpdate,
		EnvToRemove: envToRemove,
	}
	return
}
