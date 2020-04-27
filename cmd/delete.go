package cmd

import (
	"github.com/boson-project/faas/client"
	"github.com/boson-project/faas/kubectl"
	"github.com/ory/viper"
	"github.com/spf13/cobra"
)

func init() {
	root.AddCommand(deleteCmd)

	deleteCmd.Flags().StringP("name", "n", "",
		"Optionally specify the name of the Service Function to remove.")

}

var deleteCmd = &cobra.Command{
	Use:        "delete",
	Short:      "Delete deployed Service Function",
	Long:       `Removes the deployed Service Function for the current directory, but does not delete anything locally.  If no code updates have been made beyond the defaults, this would bring the current codebase back to a state equivalent to having run "create --local".`,
	SuggestFor: []string{"remove", "rm"},
	RunE:       delete,
}

func delete(cmd *cobra.Command, args []string) (err error) {
	var (
		verbose = viper.GetBool("verbose")
		remover = kubectl.NewRemover()
	)

	client, err := client.New(".",
		client.WithVerbose(verbose),
		client.WithRemover(remover),
	)
	if err != nil {
		return
	}

	// Remove the service specified by the current direcory's config.
	return client.Remove()
}
