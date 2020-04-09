package cmd

import (
	"errors"

	"github.com/ory/viper"
	"github.com/spf13/cobra"

	"github.com/lkingland/faas/appsody"
	"github.com/lkingland/faas/client"
)

func init() {
	// Add the `create` command as a subcommand to root.
	root.AddCommand(createCmd)

	// register command flags and bind them to environment variables.
	createCmd.Flags().BoolP("local", "l", false, "create the function service locally only.")
	viper.BindPFlag("local", createCmd.Flags().Lookup("local"))

	createCmd.Flags().StringP("registry", "r", "quay.io", "image registry.")
	viper.BindPFlag("registry", createCmd.Flags().Lookup("registry"))

	createCmd.Flags().StringP("namespace", "n", "", "namespace at image registry (usually username or org name). $FAAS_NAMESPACE")
	viper.BindPFlag("namespace", createCmd.Flags().Lookup("namespace"))
}

// The create command invokes the Service Funciton Client to create a new,
// functional, deployed service function with a noop implementation.  It
// can be optionally created only locally (no deploy) using --local.
var createCmd = &cobra.Command{
	Use:        "create <language>",
	Short:      "Create a Service Function",
	SuggestFor: []string{"init", "new"},
	RunE:       create,
}

func create(cmd *cobra.Command, args []string) (err error) {
	// Assert a language parameter was provided
	if len(args) == 0 {
		return errors.New("'faas create' requires a language argument.")
	}

	// Assemble parameters for use in client method invocation.
	var (
		language  = args[0]                      // language is the first argument
		local     = viper.GetBool("local")       // Only perform local creation steps
		verbose   = viper.GetBool("verbose")     // Verbose logging
		registry  = viper.GetString("registry")  // Registry (ex: docker.io)
		namespace = viper.GetString("namespace") // namespace at registry (user or org name)
	)

	if namespace == "" {
		return errors.New("image registry namespace (--namespace or FAAS_NAMESPACE is required)")
	}

	// Initializer creates a deployable noop function implementation in the
	// configured path.
	initializer := appsody.NewInitializer()
	initializer.Verbose = verbose

	// Builder creates images from function source.
	builder := appsody.NewBuilder(registry, namespace)
	builder.Verbose = verbose

	// Pusher of images to a registry
	// pusher := appsody.NewPusher()
	// pusher.Verbose = verbose

	// Deployer of built images.
	// deployer := kn.NewDeployer()
	// deployer.Verbose = verbose

	// Instantiate a client, specifying concrete implementations for
	// Initializer and Deployer, as well as setting the optional verbosity param.
	client, err := client.New(
		client.WithInitializer(initializer),
		client.WithBuilder(builder),
		client.WithVerbose(verbose),
	)
	if err != nil {
		return
	}

	// Set the client to be local-only (default false)
	client.SetLocal(local)

	// Invoke the creation of the new Service Function locally.
	return client.Create(language)
}
