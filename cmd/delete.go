package cmd

import (
	"fmt"

	"github.com/AlecAivazis/survey/v2"
	"github.com/ory/viper"
	"github.com/spf13/cobra"

	"knative.dev/func/pkg/config"
	fn "knative.dev/func/pkg/functions"
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
{{rootCmdUse}} delete myfunc --namespace apps
`,
		SuggestFor:        []string{"remove", "del"},
		Aliases:           []string{"rm"},
		ValidArgsFunction: CompleteFunctionList,
		PreRunE:           bindEnv("path", "confirm", "all", "namespace", "verbose"),
		SilenceUsage:      true, // no usage dump on error
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDelete(cmd, args, newClient)
		},
	}

	// Config
	cfg, err := config.NewDefault()
	if err != nil {
		fmt.Fprintf(cmd.OutOrStdout(), "error loading config at '%v'. %v\n", config.File(), err)
	}

	// Flags
	cmd.Flags().StringP("namespace", "n", defaultNamespace(fn.Function{}, false), "The namespace when deleting by name. ($FUNC_NAMESPACE)")
	cmd.Flags().StringP("all", "a", "true", "Delete all resources created for a function, eg. Pipelines, Secrets, etc. ($FUNC_ALL) (allowed values: \"true\", \"false\")")
	addConfirmFlag(cmd, cfg.Confirm)
	addPathFlag(cmd)
	addVerboseFlag(cmd, cfg.Verbose)

	return cmd
}

func runDelete(cmd *cobra.Command, args []string, newClient ClientFactory) (err error) {
	cfg, err := newDeleteConfig(cmd, args)
	if err != nil {
		return
	}
	if cfg, err = cfg.Prompt(); err != nil {
		return
	}

	client, done := newClient(ClientConfig{Verbose: cfg.Verbose})
	defer done()

	if cfg.Name != "" { // Delete by name if provided
		return client.Remove(cmd.Context(), cfg.Name, cfg.Namespace, fn.Function{}, cfg.All)
	} else { // Otherwise; delete the function at path (cwd by default)
		f, err := fn.NewFunction(cfg.Path)
		if err != nil {
			return err
		}
		return client.Remove(cmd.Context(), "", "", f, cfg.All)
	}
}

type deleteConfig struct {
	Name      string
	Namespace string
	Path      string
	All       bool
	Verbose   bool
}

// newDeleteConfig returns a config populated from the current execution context
// (args, flags and environment variables)
func newDeleteConfig(cmd *cobra.Command, args []string) (cfg deleteConfig, err error) {
	var name string
	if len(args) > 0 {
		name = args[0]
	}
	cfg = deleteConfig{
		All:       viper.GetBool("all"),
		Name:      name, // args[0] or derived
		Namespace: viper.GetString("namespace"),
		Path:      viper.GetString("path"),
		Verbose:   viper.GetBool("verbose"), // defined on root
	}
	if cfg.Name == "" && cmd.Flags().Changed("namespace") {
		// logicially inconsistent to supply only a namespace.
		// Either use the function's local state in its entirety, or specify
		// both a name and a namespace to ignore any local function source.
		err = fmt.Errorf("must also specify a name when specifying namespace.")
	}
	if cfg.Name != "" && cmd.Flags().Changed("path") {
		// logically inconsistent to provide both a name and a path to source.
		// Either use the function's local state on disk (--path), or specify
		// a name and a namespace to ignore any local function source.
		err = fmt.Errorf("only one of --path and [NAME] should be provided")
	}
	return
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
				Default: c.All,
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
	dc.All = answers.All

	return dc, err
}
