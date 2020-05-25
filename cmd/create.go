package cmd

import (
	"errors"
	"regexp"

	"github.com/ory/viper"
	"github.com/spf13/cobra"

	"github.com/boson-project/faas"
	"github.com/boson-project/faas/appsody"
	"github.com/boson-project/faas/docker"
	"github.com/boson-project/faas/kubectl"
	"github.com/boson-project/faas/prompt"
)

func init() {
	// Add the `create` command as a subcommand to root.
	root.AddCommand(createCmd)
	createCmd.Flags().BoolP("local", "l", false, "create the service function locally only.")
	createCmd.Flags().BoolP("internal", "i", false, "Create a cluster-local service without a publicly accessible route. $FAAS_INTERNAL")
	createCmd.Flags().StringP("name", "n", "", "optionally specify an explicit name for the serive, overriding path-derivation. $FAAS_NAME")
	createCmd.Flags().StringP("registry", "r", "quay.io", "image registry (ex: quay.io). $FAAS_REGISTRY")
	createCmd.Flags().StringP("namespace", "s", "", "namespace at image registry (usually username or org name). $FAAS_NAMESPACE")
}

// The create command invokes the Service Funciton Client to create a new,
// functional, deployed service function with a noop implementation.  It
// can be optionally created only locally (no deploy) using --local.
var createCmd = &cobra.Command{
	Use:        "create <language>",
	Short:      "Create a Service Function",
	SuggestFor: []string{"init", "new"},
	RunE:       create,
	PreRun: func(cmd *cobra.Command, args []string) {
		viper.BindPFlag("local", cmd.Flags().Lookup("local"))
		viper.BindPFlag("internal", cmd.Flags().Lookup("internal"))
		viper.BindPFlag("name", cmd.Flags().Lookup("name"))
		viper.BindPFlag("registry", cmd.Flags().Lookup("registry"))
		viper.BindPFlag("namespace", cmd.Flags().Lookup("namespace"))
	},
}

// The create command expects several parameters, most of which can be
// defaulted.  When an interactive terminal is detected, these parameters,
// which are gathered into this config object, are passed through the shell
// allowing the user to interactively confirm and optionally modify values.
type createConfig struct {
	// Verbose mode instructs the system to output detailed logs as the command
	// progresses.
	Verbose bool

	// Local mode flag only builds a function locally, with no deployed
	// counterpart.
	Local bool

	// Internal only flag.  A public route will not be allocated and the domain
	// suffix will be a .local (depending on underlying cluster configuration)
	Internal bool

	// Name of the service in DNS-compatible format (ex myfunc.example.com)
	Name string

	// Registry of containers (ex. quay.io or hub.docker.com)
	Registry string

	// Namespace within the container registry within which interstitial built
	// images will be stored by their canonical name.
	Namespace string

	// Language is the first argument, and specifies the resultant Function
	// implementation language.
	Language string

	// Path of the Function implementation on local disk. Defaults to current
	// working directory of the process.
	Path string
}

// create a new service function using the client about config.
func create(cmd *cobra.Command, args []string) (err error) {
	// Assert a language parameter was provided
	if len(args) == 0 {
		return errors.New("'faas create' requires a language argument.")
	}

	// Create a deafult configuration populated first with environment variables,
	// followed by overrides by flags.
	var config = createConfig{
		Verbose:   viper.GetBool("verbose"),
		Local:     viper.GetBool("local"),
		Internal:  viper.GetBool("internal"),
		Name:      viper.GetString("name"),
		Registry:  viper.GetString("registry"),
		Namespace: viper.GetString("namespace"),
		Language:  args[0],
		Path:      ".", // will be expanded to process current working dir.
	}

	// If path is provided
	if len(args) == 2 {
		config.Path = args[1]
	}

	// Namespace can not be defaulted.
	if config.Namespace == "" {
		return errors.New("image registry namespace (--namespace or FAAS_NAMESPACE is required)")
	}

	// If we are running as an interactive terminal, allow the user
	// to mutate default config prior to execution.
	if isInteractive() {
		config, err = gatherFromUser(config)
		if err != nil {
			return err
		}
	}

	// Initializer creates a deployable noop function implementation in the
	// configured path.
	initializer := appsody.NewInitializer()
	initializer.Verbose = config.Verbose

	// Builder creates images from function source.
	builder := appsody.NewBuilder(config.Registry, config.Namespace)
	builder.Verbose = config.Verbose

	// Pusher of images
	pusher := docker.NewPusher()
	pusher.Verbose = config.Verbose

	// Deployer of built images.
	deployer := kubectl.NewDeployer()
	deployer.Verbose = config.Verbose

	// Instantiate a client, specifying concrete implementations for
	// Initializer and Deployer, as well as setting the optional verbosity param.
	client, err := faas.New(
		faas.WithVerbose(config.Verbose),
		faas.WithInitializer(initializer),
		faas.WithBuilder(builder),
		faas.WithPusher(pusher),
		faas.WithDeployer(deployer),
		faas.WithLocal(config.Local),       // local template only (no cluster deployment)
		faas.WithInternal(config.Internal), // if deployed, no publicly accessible route.
	)
	if err != nil {
		return
	}

	// Invoke the creation of the new Service Function locally.
	// Returns the final address.
	// Name can be empty string (path-dervation will be attempted)
	// Path can be empty, defaulting to current working directory.
	return client.Create(config.Language, config.Name, config.Path)
}

func gatherFromUser(config createConfig) (c createConfig, err error) {
	config.Path = prompt.ForString("Local source path", config.Path)
	config.Name, err = promptForName("name of service function", config)
	if err != nil {
		return config, err
	}
	config.Local = prompt.ForBool("Local; no remote deployment", config.Local)
	config.Internal = prompt.ForBool("Internal; no public route", config.Internal)
	config.Registry = prompt.ForString("Image registry", config.Registry)
	config.Namespace = prompt.ForString("Namespace at registry", config.Namespace)
	config.Language = prompt.ForString("Language of source", config.Language)
	return config, nil
}

// Prompting for Service Name with Default
// Early calclation of service function name is required to provide a sensible
// default.  If the user did not provide a --name parameter or FAAS_NAME,
// this funciton sets the default to the value that the client would have done
// on its own if non-interactive: by creating a new function rooted at config.Path
// and then calculate from that path.
func promptForName(label string, config createConfig) (string, error) {
	// Pre-calculate the function name derived from path
	if config.Name == "" {
		f, err := faas.NewFunction(config.Path)
		if err != nil {
			return "", err
		}
		maxRecursion := 5 // TODO synchronize with that used in actual initialize step.
		return prompt.ForString("Name of service function", f.DerivedName(maxRecursion), prompt.WithRequired(true)), nil
	}

	// The user provided a --name or FAAS_NAME; just confirm it.
	return prompt.ForString("Name of service function", config.Name, prompt.WithRequired(true)), nil
}

// acceptable answers: y,yes,Y,YES,1
var confirmExp = regexp.MustCompile("(?i)y(?:es)?|1")

func fromYN(s string) bool {
	return confirmExp.MatchString(s)
}
