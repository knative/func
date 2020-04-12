package cmd

import (
	"github.com/lkingland/faas/client"
	"github.com/lkingland/faas/kubectl"
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

	client, err := client.New(
		client.WithVerbose(verbose),
		client.WithRemover(remover),
	)
	if err != nil {
		return
	}

	// Determine the name of the service to remove
	// (considers local configuration or flag)
	name, err := serviceToRemove()
	if err != nil {
		return
	}

	// Derive the name of the servie to be removed from the
	// local configuration.
	return client.Remove(name)
}

// Determine the service name to remove.
// If the flag `name` was provided, this takes precedence.
// Otherwise the name is pulled from the local directory's
// service function configurtion file, which was created when
// the service was initialized.
// If there is no local configuration, there is either not
// a remote running, or the config was manually removed
// severing the connection between the local codebase and
// the service function.
func serviceToRemove() (string, error) {
	// If empty, the default value is used, which causes the client to assume the
	// name of the service rooted at it's currently associated directory.
	return viper.GetString("name"), nil

	/* TODO: begin using a .faas.yaml when we have anything more than the
	 * (derivable) name to store.
	bb, err := ioutil.ReadFile(".faas.yaml")
	if err != nil {
		return errors.New("unable to determine service to remove.  Confirm you are in the function directory, and that it was previously deployed.  To specify a name explicitly, use --name.")
		if verbose {
			fmt.Println(err)
		}
	}

	config := struct { Name string `yaml:"project-name"`}
	err := yaml.Unmarshal(bb, config)
	if err != nil {
		return errors.New("The config is unable to be read.  To specify a service function to remove explicitly, use the --name flag.")
		if verbose {
			fmt.Println(err)
		}
	}
	*/
}
