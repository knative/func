package cmd

import (
	"fmt"

	"github.com/ory/viper"
	"github.com/spf13/cobra"

	fn "github.com/boson-project/func"
	"github.com/boson-project/func/knative"
	"github.com/boson-project/func/prompt"
)

func init() {
	root.AddCommand(deleteCmd)
}

func NewDeleteCmd(newRemover func(ns string, verbose bool) (fn.Remover, error)) *cobra.Command {
	delCmd := &cobra.Command{
		Use:   "delete [NAME]",
		Short: "Undeploy a function",
		Long: `Undeploy a function

This command undeploys a function from the cluster. By default the function from 
the project in the current directory is undeployed. Alternatively either the name 
of the function can be given as argument or the project path provided with --path.

No local files are deleted.
`,
		Example: `
# Undeploy the function defined in the local directory
kn func delete

# Undeploy the function 'myfunc' in namespace 'apps'
kn func delete -n apps myfunc
`,
		SuggestFor:        []string{"remove", "rm", "del"},
		ValidArgsFunction: CompleteFunctionList,
		PreRunE:           bindEnv("path", "confirm", "namespace"),
		RunE: func(cmd *cobra.Command, args []string) (err error) {
			config := newDeleteConfig(args).Prompt()

			var function fn.Function

			// Initialize func with explicit name (when provided)
			if len(args) > 0 && args[0] != "" {
				pathChanged := cmd.Flags().Changed("path")
				if pathChanged {
					return fmt.Errorf("Only one of --path and [NAME] should be provided")
				}
				function = fn.Function{
					Name: args[0],
				}
			} else {
				function, err = fn.NewFunction(config.Path)
				if err != nil {
					return
				}

				// Check if the Function has been initialized
				if !function.Initialized() {
					return fmt.Errorf("the given path '%v' does not contain an initialized function", config.Path)
				}
			}

			ns := config.Namespace
			if ns == "" {
				ns = function.Namespace
			}

			remover, err := newRemover(ns, config.Verbose)
			if err != nil {
				return
			}

			client := fn.New(
				fn.WithVerbose(config.Verbose),
				fn.WithRemover(remover))

			return client.Remove(cmd.Context(), function)
		},
	}

	delCmd.Flags().BoolP("confirm", "c", false, "Prompt to confirm all configuration options (Env: $FUNC_CONFIRM)")
	delCmd.Flags().StringP("path", "p", cwd(), "Path to the function project that should be undeployed (Env: $FUNC_PATH)")
	delCmd.Flags().StringP("namespace", "n", "", "Namespace of the function to undeploy. By default, the namespace in func.yaml is used or the actual active namespace if not set in the configuration. (Env: $FUNC_NAMESPACE)")

	return delCmd
}

var deleteCmd = NewDeleteCmd(func(ns string, verbose bool) (fn.Remover, error) {
	r, err := knative.NewRemover(ns)
	if err != nil {
		return nil, err
	}
	r.Verbose = verbose
	return r, nil
})

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
