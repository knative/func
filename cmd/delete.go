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
	deleteCmd.Flags().BoolP("confirm", "c", false, "Prompt to confirm all configuration options - $FAAS_CONFIRM")
	deleteCmd.Flags().StringP("namespace", "n", "", "Override namespace in which to search for Functions.  Default is to use currently active underlying platform setting - $FAAS_NAMESPACE")
}

var deleteCmd = &cobra.Command{
	Use:               "delete",
	Short:             "Delete a Function deployment",
	Long:              `Removes the deployed Function in the current project directory.  No local files are deleted.`,
	SuggestFor:        []string{"remove", "rm", "del"},
	ValidArgsFunction: CompleteFunctionList,
	PreRunE:           bindEnv("confirm", "namespace"),
	RunE:              runDelete,
}

func runDelete(cmd *cobra.Command, args []string) (err error) {
	config := newDeleteConfig(args).Prompt()
	function, err := faas.LoadFunction(config.Path)
	if err != nil {
		return
	}

	remover := knative.NewRemover(config.Namespace)
	remover.Verbose = config.Verbose

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
		Path:      cwd(),
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
