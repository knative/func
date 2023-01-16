package cmd

import (
	"fmt"

	"github.com/AlecAivazis/survey/v2"
	"github.com/ory/viper"
	"github.com/spf13/cobra"

	fn "knative.dev/func"
	"knative.dev/func/config"
)

func NewDeleteCmd(newClient ClientFactory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "delete <name>",
		Short: "Undeploy a function",
		Long: `Undeploy a function

This command undeploys a function from the cluster. By default the function from
the project in the current directory is undeployed. Alternatively either the name
of the function can be given as argument or the project path provided with --path.

No local files are deleted.
`,
		Example: `
# Undeploy the function defined in the local directory
{{rootCmdUse}} delete

# Undeploy the function 'myfunc' in namespace 'apps'
{{rootCmdUse}} delete -n apps myfunc
`,
		SuggestFor:        []string{"remove", "rm", "del"},
		ValidArgsFunction: CompleteFunctionList,
		PreRunE:           bindEnv("path", "confirm", "all", "namespace"),
		SilenceUsage:      true, // no usage dump on error
	}

	// Config
	cfg, err := config.NewDefault()
	if err != nil {
		fmt.Fprintf(cmd.OutOrStdout(), "error loading config at '%v'. %v\n", config.File(), err)
	}

	// Flags
	cmd.Flags().BoolP("confirm", "c", cfg.Confirm, "Prompt to confirm all configuration options (Env: $FUNC_CONFIRM)")
	cmd.Flags().StringP("namespace", "n", cfg.Namespace, "The namespace in which to delete. (Env: $FUNC_NAMESPACE)")
	cmd.Flags().StringP("all", "a", "true", "Delete all resources created for a function, eg. Pipelines, Secrets, etc. (Env: $FUNC_ALL) (allowed values: \"true\", \"false\")")
	setPathFlag(cmd)

	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		return runDelete(cmd, args, newClient)
	}

	return cmd
}

func runDelete(cmd *cobra.Command, args []string, newClient ClientFactory) (err error) {
	cfg, err := newDeleteConfig(args).Prompt()
	if err != nil {
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
		function, err = fn.NewFunction(cfg.Path)
		if err != nil {
			return
		}

		// Check if the function has been initialized
		if !function.Initialized() {
			return fmt.Errorf("the given path '%v' does not contain an initialized function", cfg.Path)
		}

		// If not provided, use the function's extant namespace
		if !cmd.Flags().Changed("namespace") {
			cfg.Namespace = function.Deploy.Namespace
		}

	}

	// Create a client instance from the now-final config
	client, done := newClient(ClientConfig{Namespace: cfg.Namespace, Verbose: cfg.Verbose})
	defer done()

	// Invoke remove using the concrete client impl
	return client.Remove(cmd.Context(), function, cfg.DeleteAll)
}

type deleteConfig struct {
	Name      string
	Namespace string
	Path      string
	DeleteAll bool
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
		DeleteAll: viper.GetBool("all"),
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

	dc := c
	var qs = []*survey.Question{
		{
			Name: "name",
			Prompt: &survey.Input{
				Message: "Function to remove:",
				Default: deriveName(c.Name, c.Path)},
			Validate: survey.Required,
		},
		{
			Name: "all",
			Prompt: &survey.Confirm{
				Message: "Do you want to delete all resources?",
				Default: c.DeleteAll,
			},
		},
	}
	answers := struct {
		Name string
		All  bool
	}{}

	err := survey.Ask(qs, &answers)
	if err != nil {
		return dc, err
	}

	dc.Name = answers.Name
	dc.DeleteAll = answers.All

	return dc, err
}
