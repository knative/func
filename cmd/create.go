package cmd

import (
	"errors"

	"github.com/ory/viper"
	"github.com/spf13/cobra"

	"github.com/lkingland/faas/appsody"
	"github.com/lkingland/faas/client"
	"github.com/lkingland/faas/docker"
	"github.com/lkingland/faas/kubectl"
)

func init() {
	// Add the `create` command as a subcommand to root.
	root.AddCommand(createCmd)

	// register command flags and bind them to environment variables.
	createCmd.Flags().BoolP("local", "l", false, "create the service function locally only.")
	viper.BindPFlag("local", createCmd.Flags().Lookup("local"))

	createCmd.Flags().StringP("name", "n", "", "optionally specify an explicit name for the serive, overriding path-derivation. $FAAS_NAME")
	viper.BindPFlag("name", createCmd.Flags().Lookup("name"))

	createCmd.Flags().StringP("registry", "r", "quay.io", "image registry (ex: quay.io). $FAAS_REGISTRY")
	viper.BindPFlag("registry", createCmd.Flags().Lookup("registry"))

	createCmd.Flags().StringP("namespace", "s", "", "namespace at image registry (usually username or org name). $FAAS_NAMESPACE")
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

	// Assemble parameters for use in client's 'create' invocation.
	var (
		language  = args[0]                      // language is the first argument
		local     = viper.GetBool("local")       // Only perform local creation steps
		name      = viper.GetString("name")      // Explicit name override (by default path-derives)
		verbose   = viper.GetBool("verbose")     // Verbose logging
		registry  = viper.GetString("registry")  // Registry (ex: docker.io)
		namespace = viper.GetString("namespace") // namespace at registry (user or org name)
	)

	// Namespace can not be defaulted.
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

	// Pusher of images
	pusher := docker.NewPusher()
	pusher.Verbose = verbose

	// Deployer of built images.
	deployer := kubectl.NewDeployer()
	deployer.Verbose = verbose

	// Instantiate a client, specifying concrete implementations for
	// Initializer and Deployer, as well as setting the optional verbosity param.
	client, err := client.New(
		client.WithVerbose(verbose),
		client.WithName(name),
		client.WithInitializer(initializer),
		client.WithBuilder(builder),
		client.WithPusher(pusher),
		client.WithDeployer(deployer),
	)
	if err != nil {
		return
	}

	// Set the client to potentially be local-only (default false)
	client.SetLocal(local)

	// Invoke the creation of the new Service Function locally.
	// Returns the final address.
	return client.Create(language)
}
