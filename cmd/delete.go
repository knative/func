package cmd

import (
	"fmt"

	"github.com/ory/viper"
	"github.com/spf13/cobra"

	"github.com/boson-project/faas"
	"github.com/boson-project/faas/knative"
	"github.com/boson-project/faas/prompt"
)

func init() {
	root.AddCommand(deleteCmd)
	deleteCmd.Flags().BoolP("confirm", "c", false, "Prompt to confirm all configuration options - $FAAS_CONFIRM")
	deleteCmd.Flags().StringP("path", "p", cwd(), "Path to the project which should be deleted - $FAAS_PATH")
	deleteCmd.Flags().StringP("namespace", "n", "", "Override namespace in which to search for Functions.  Default is to use currently active underlying platform setting - $FAAS_NAMESPACE")
}

var deleteCmd = &cobra.Command{
	Use:   "delete <name>",
	Short: "Delete a Function deployment",
	Long: `Delete a Function deployment

Removes a deployed function from the cluster. The user may specify a function
by name, path using the --path or -p flag, or if neither of those are provided,
the current directory will be searched for a faas.yaml configuration file to
determine the function to be removed.

The namespace defaults to the value in faas.yaml or the namespace currently
active in the user's Kubernetes configuration. The namespace may be specified
on the command line using the --namespace or -n flag, and if so this will
overwrite the value in faas.yaml.
`,
	SuggestFor:        []string{"remove", "rm", "del"},
	ValidArgsFunction: CompleteFunctionList,
	PreRunE:           bindEnv("path", "confirm", "namespace"),
	RunE:              runDelete,
}

func runDelete(cmd *cobra.Command, args []string) (err error) {
	config := newDeleteConfig(args).Prompt()

	remover := knative.NewRemover(config.Namespace)
	remover.Verbose = config.Verbose
	remover.Namespace = config.Namespace

	function, err := faas.NewFunction(config.Path)
	if err != nil {
		return
	}

	// Check if the Function has been initialized
	if !function.Initialized() {
		return fmt.Errorf("the given path '%v' does not contain an initialized Function.", config.Path)
	}

	client := faas.New(
		faas.WithVerbose(config.Verbose),
		faas.WithRemover(remover))

	return client.Remove(function)
}

type deleteConfig struct {
	Name      string
	Namespace string
	Path      string
	Verbose   bool
}

// newDeleteConfig returns a config populated from the current execution context
// (args, flags and environment variables)
func newDeleteConfig(args []string) deleteConfig {
	var name string
	if len(args) > 0 {
		name = args[0]
	}
	return deleteConfig{
		Path:      viper.GetString("path"),
		Namespace: viper.GetString("namespace"),
		Name:      deriveName(name, viper.GetString("path")), // args[0] or derived
		Verbose:   viper.GetBool("verbose"),                  // defined on root
	}
}

// Prompt the user with value of config members, allowing for interaractive changes.
// Skipped if not in an interactive terminal (non-TTY), or if --yes (agree to
// all prompts) was explicitly set.
func (c deleteConfig) Prompt() deleteConfig {
	if !interactiveTerminal() || !viper.GetBool("confirm") {
		return c
	}
	return deleteConfig{
		// TODO: Path should be prompted for and set prior to name attempting path derivation.  Test/fix this if necessary.
		Name: prompt.ForString("Function to remove", deriveName(c.Name, c.Path), prompt.WithRequired(true)),
	}
}
