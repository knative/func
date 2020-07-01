package cmd

import (
	"github.com/boson-project/faas"
	"github.com/boson-project/faas/knative"
	"github.com/ory/viper"
	"github.com/spf13/cobra"
)

func init() {
	root.AddCommand(deleteCmd)
	deleteCmd.Flags().StringP("name", "n", "", "optionally specify an explicit name to remove, overriding path-derivation. $FAAS_NAME")
	deleteCmd.RegisterFlagCompletionFunc("name", CompleteFunctionList)
}

var deleteCmd = &cobra.Command{
	Use:        "delete",
	Short:      "Delete deployed Service Function",
	Long:       `Removes the deployed Service Function for the current directory, but does not delete anything locally.  If no code updates have been made beyond the defaults, this would bring the current codebase back to a state equivalent to having run "create --local".`,
	SuggestFor: []string{"remove", "rm"},
	RunE:       delete,
	PreRun: func(cmd *cobra.Command, args []string) {
		viper.BindPFlag("name", cmd.Flags().Lookup("name"))
	},
}

func delete(cmd *cobra.Command, args []string) (err error) {
	var (
		verbose = viper.GetBool("verbose")
		remover = knative.NewRemover()
		name    = viper.GetString("name") // Explicit name override (by default uses path argument)
		path    = ""                      // defaults to current working directory
	)
	remover.Verbose = verbose
	// If provided use the path as the first argument.
	if len(args) == 1 {
		name = args[0]
	}

	client, err := faas.New(
		faas.WithVerbose(verbose),
		faas.WithRemover(remover),
	)
	if err != nil {
		return
	}

	// Remove name (if provided), or the (initialized) function at path.
	return client.Remove(name, path)
}
