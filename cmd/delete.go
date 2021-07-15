package cmd

import (
	"fmt"

	"github.com/AlecAivazis/survey/v2"
	"github.com/AlecAivazis/survey/v2/terminal"
	"github.com/ory/viper"
	"github.com/spf13/cobra"

	fn "knative.dev/kn-plugin-func"
	"knative.dev/kn-plugin-func/knative"
)

func init() {
	// Create a new delete command with a reference to
	// a function which yields an appropriate concrete client instance.
	root.AddCommand(NewDeleteCmd(newDeleteClient))
}

// newDeleteClient returns an instance of a Client using the
// final config state.
// Testing note: This method is swapped out during testing to allow
// mocking the remover or the client itself to fabricate test states.
func newDeleteClient(cfg deleteConfig) (*fn.Client, error) {
	remover, err := knative.NewRemover(cfg.Namespace)
	if err != nil {
		return nil, err
	}

	remover.Verbose = cfg.Verbose

	return fn.New(
		fn.WithRemover(remover),
		fn.WithVerbose(cfg.Verbose)), nil
}

// A deleteClientFn is a function which yields a Client instance from a config
type deleteClientFn func(deleteConfig) (*fn.Client, error)

func NewDeleteCmd(clientFn deleteClientFn) *cobra.Command {
	cmd := &cobra.Command{
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
	}

	cmd.Flags().BoolP("confirm", "c", false, "Prompt to confirm all configuration options (Env: $FUNC_CONFIRM)")
	cmd.Flags().StringP("path", "p", cwd(), "Path to the function project that should be undeployed (Env: $FUNC_PATH)")
	cmd.Flags().StringP("namespace", "n", "", "Namespace of the function to undeploy. By default, the namespace in func.yaml is used or the actual active namespace if not set in the configuration. (Env: $FUNC_NAMESPACE)")

	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		return runDelete(cmd, args, clientFn)
	}

	return cmd
}

func runDelete(cmd *cobra.Command, args []string, clientFn deleteClientFn) (err error) {
	config, err := newDeleteConfig(args).Prompt()
	if err != nil {
		if err == terminal.InterruptErr {
			return nil
		}
		return
	}

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

	// If not provided, use the function's extant namespace
	if config.Namespace == "" {
		config.Namespace = function.Namespace
	}

	// Create a client instance from the now-final config
	client, err := clientFn(config)
	if err != nil {
		return err
	}

	// Invoke remove using the concrete client impl
	return client.Remove(cmd.Context(), function)
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
func (c deleteConfig) Prompt() (deleteConfig, error) {
	if !interactiveTerminal() || !viper.GetBool("confirm") {
		return c, nil
	}

	dc := deleteConfig{}

	return dc, survey.AskOne(
		&survey.Input{
			Message: "Function to remove:",
			Default: deriveName(c.Name, c.Path)},
		&dc.Name, survey.WithValidator(survey.Required))
}
