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
	createCmd.Flags().BoolP("local", "", false, "create the function service locally only.")
	viper.BindPFlag("local", createCmd.Flags().Lookup("local"))
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
	// Preconditions ensure the command is well-formed.
	if len(args) == 0 {
		return errors.New("'faas create' requires a language argument.")
	}

	// Assemble parameters for use in client method invocation.
	var (
		language = args[0]                  // language is the first argument
		local    = viper.GetBool("local")   // Only perform local creation steps
		verbose  = viper.GetBool("verbose") // Verbose logging
	)

	// The function initializer is presently implemented using appsody.
	initializer := appsody.NewInitializer()
	initializer.Verbose = verbose

	// Instantiate a client, specifying concrete implementations for
	// Initializer and Deployer, as well as setting the optional verbosity param.
	client, err := client.New(
		client.WithInitializer(initializer),
		client.WithVerbose(verbose))
	if err != nil {
		return
	}

	// Invoke the creation of the new Service Function locally.
	if err = client.Create(language); err != nil {
		return
	}

	// If running in local-only mode, execution is complete.
	if local {
		return
	}

	// Deploy the newly initialized Service Function.
	return client.Deploy()
}
