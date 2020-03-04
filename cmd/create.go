package cmd

import (
	"errors"

	"github.com/ory/viper"
	"github.com/spf13/cobra"

	"github.com/lkingland/faas/client"
)

func init() {
	root.AddCommand(createCmd)

	createCmd.Flags().BoolP("local", "", false, "create the function locally only.")
	viper.BindPFlag("local", createCmd.Flags().Lookup("local"))
}

var createCmd = &cobra.Command{
	Use:        "create <language>",
	Short:      "Create a Service Function",
	SuggestFor: []string{"init", "new"},
	RunE:       create,
}

func create(cmd *cobra.Command, args []string) (err error) {
	// Preconditions
	if len(args) == 0 {
		return errors.New("'faas create' requires a language argument.")
	}

	// Assemble parameters for use in client method invocation.
	var (
		language = args[0]                  // language is the first argument
		local    = viper.GetBool("local")   // Only perform local creation steps
		verbose  = viper.GetBool("verbose") // Verbose logging
	)

	// Instantiate a client, specifying optional verbosity.
	client := client.New(client.WithVerbose(verbose))

	// Invoke Service Funcation creation.
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
