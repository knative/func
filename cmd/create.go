package cmd

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ory/viper"
	"github.com/spf13/cobra"

	"github.com/boson-project/faas"
	"github.com/boson-project/faas/buildpacks"
	"github.com/boson-project/faas/docker"
	"github.com/boson-project/faas/embedded"
	"github.com/boson-project/faas/knative"
	"github.com/boson-project/faas/progress"
)

func init() {
	cwd, err := os.Getwd()
	if err != nil {
		panic(err)
	}

	// Add the `create` command as a subcommand to root.
	root.AddCommand(createCmd)
	// createCmd.Flags().BoolP("internal", "i", false, "Create a cluster-local function without a publicly accessible route - $FAAS_INTERNAL")
	createCmd.Flags().StringP("name", "n", "", "Specify an explicit name for the serive, overriding path-derivation - $FAAS_NAME")
	createCmd.Flags().StringP("namespace", "s", "default", "namespace at image registry (usually username or org name) - $FAAS_NAMESPACE")
	createCmd.Flags().StringP("path", "p", cwd, "Path to the new project directory")
	createCmd.Flags().StringP("tag", "t", "", "Specify an image tag, for example quay.io/myrepo/project.name:latest")
	createCmd.Flags().StringP("templates", "", filepath.Join(configPath(), "faas", "templates"), "Extensible templates path. $FAAS_TEMPLATES")
	createCmd.Flags().StringP("trigger", "g", embedded.DefaultTemplate, "Function template (ex: 'http','events') - $FAAS_TEMPLATE")
	createCmd.Flags().BoolP("local", "l", false, "Create the function locally only. Do not push to a cluster")

	err = createCmd.MarkFlagRequired("tag")
	if err != nil {
		fmt.Println("Error while marking 'tag' required")
	}
	err = createCmd.RegisterFlagCompletionFunc("tag", CompleteRegistryList)
	if err != nil {
		fmt.Println("Error while calling RegisterFlagCompletionFunc: ", err)
	}
}

// The create command invokes the Funciton Client to create a new,
// functional, deployed service function with a noop implementation.  It
// can be optionally created only locally (no deploy) using --local.
var createCmd = &cobra.Command{
	Use:               "create <runtime>",
	Short:             "Create a new Function project, build it, and push it to a cluster",
	SuggestFor:        []string{"cerate", "new"},
	ValidArgsFunction: CompleteRuntimeList,
	Args:              cobra.ExactArgs(1),
	RunE:              create,
	PreRun: func(cmd *cobra.Command, args []string) {
		flags := []string{"local", "name", "namespace", "tag", "path", "trigger", "templates"}
		for _, f := range flags {
			err := viper.BindPFlag(f, cmd.Flags().Lookup(f))
			if err != nil {
				panic(err)
			}
		}
	},
}

// The create command expects several parameters, most of which can be
// defaulted.  When an interactive terminal is detected, these parameters,
// which are gathered into this config object, are passed through the shell
// allowing the user to interactively confirm and optionally modify values.
type createConfig struct {
	initConfig
	// Namespace on the cluster where the function will be deployed
	Namespace string

	// Local mode flag only builds a function locally, with no deployed counterpart
	Local bool
}

// create a new service function using the client about config.
func create(cmd *cobra.Command, args []string) (err error) {
	// Assert a runtime parameter was provided
	if len(args) == 0 {
		return errors.New("'faas create' requires a runtime argument")
	}

	// Create a deafult configuration populated first with environment variables,
	// followed by overrides by flags.
	var config = createConfig{
		Namespace: viper.GetString("namespace"),
	}
	config.Verbose = viper.GetBool("verbose")
	config.Local = viper.GetBool("local")
	config.Name = viper.GetString("name")
	config.Tag = viper.GetString("tag")
	config.Trigger = viper.GetString("trigger")
	config.Templates = viper.GetString("templates")
	config.Path = viper.GetString("path")
	config.Runtime = args[0]

	// If we are running as an interactive terminal, allow the user
	// to mutate default config prior to execution.
	if interactiveTerminal() {
		config.initConfig, err = promptWithDefaults(config.initConfig)
		if err != nil {
			return err
		}
	}

	// Initializer creates a deployable noop function implementation in the
	// configured path.
	initializer := embedded.NewInitializer(config.Templates)
	initializer.Verbose = config.Verbose

	// Builder creates images from function source.
	builder := buildpacks.NewBuilder(config.Tag)
	builder.Verbose = config.Verbose

	// Pusher of images
	pusher := docker.NewPusher()
	pusher.Verbose = config.Verbose

	// Deployer of built images.
	deployer := knative.NewDeployer()
	deployer.Verbose = config.Verbose
	deployer.Namespace = config.Namespace

	// Progress bar
	listener := progress.New(progress.WithVerbose(config.Verbose))

	// Instantiate a client, specifying concrete implementations for
	// Initializer and Deployer, as well as setting the optional verbosity param.
	client, err := faas.New(
		faas.WithVerbose(config.Verbose),
		faas.WithInitializer(initializer),
		faas.WithBuilder(builder),
		faas.WithPusher(pusher),
		faas.WithDeployer(deployer),
		faas.WithLocal(config.Local),
		faas.WithProgressListener(listener),
	)
	if err != nil {
		return
	}

	// Invoke the creation of the new Function locally.
	// Returns the final address.
	// Name can be empty string (path-dervation will be attempted)
	// Path can be empty, defaulting to current working directory.
	return client.Create(config.Runtime, config.Trigger, config.Name, config.Tag, config.Path)
}
