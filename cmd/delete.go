package cmd

import (
	"github.com/ory/viper"
	"github.com/spf13/cobra"

	"github.com/boson-project/faas"
	"github.com/boson-project/faas/knative"
	"github.com/boson-project/faas/prompt"
)

func init() {
	root.AddCommand(deleteCmd)
	deleteCmd.Flags().StringP("path", "p", cwd(), "Path to the project which should be deleted - $FAAS_PATH")
	deleteCmd.Flags().StringP("namespace", "n", "", "Override namespace in which to search for Functions.  Default is to use currently active underlying platform setting - $FAAS_NAMESPACE")
	deleteCmd.Flags().BoolP("yes", "y", false, "When in interactive mode (attached to a TTY), skip prompting the user. - $FAAS_YES")
}

var deleteCmd = &cobra.Command{
	Use:               "delete <name>",
	Short:             "Delete a Function deployment",
	Long:              `Removes the deployed Function by name, by explicit path, or by default for the current directory.  No local files are deleted.`,
	SuggestFor:        []string{"remove", "rm", "del"},
	ValidArgsFunction: CompleteFunctionList,
	PreRunE:           bindEnv("path", "yes", "namespace"),
	RunE:              runDelete,
}

func runDelete(cmd *cobra.Command, args []string) (err error) {
	config := newDeleteConfig(args).Prompt()

	remover := knative.NewRemover(config.Namespace)
	remover.Verbose = config.Verbose

	function := faas.Function{Root: config.Path, Name: config.Name}

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
	Yes       bool
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
		Yes:       viper.GetBool("yes"),
	}
}

// Prompt the user with value of config members, allowing for interaractive changes.
// Skipped if not in an interactive terminal (non-TTY), or if --yes (agree to
// all prompts) was explicitly set.
func (c deleteConfig) Prompt() deleteConfig {
	if !interactiveTerminal() || c.Yes {
		return c
	}
	return deleteConfig{
		// TODO: Path should be prompted for and set prior to name attempting path derivation.  Test/fix this if necessary.
		Name: prompt.ForString("Function to remove", deriveName(c.Name, c.Path), prompt.WithRequired(true)),
	}
}
