package cmd

import (
	"fmt"

	"github.com/AlecAivazis/survey/v2"
	"github.com/ory/viper"
	"github.com/spf13/cobra"

	"knative.dev/func/pkg/config"
	fn "knative.dev/func/pkg/functions"
	"knative.dev/func/pkg/pipelines"
)

func NewConfigGitRemoveCmd(newClient ClientFactory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "remove",
		Short: "Remove Git settings from the function configuration",
		Long: `Remove Git settings from the function configuration

	Interactive prompt to remove Git settings from the function project in the current
	directory or from the directory specified with --path.

	It also removes any generated resources that are used for Git based build and deployment,
	such as local generated Pipelines resources and any resources generated on the cluster.
	`,
		SuggestFor: []string{"rem", "rmeove", "del", "dle"},
		PreRunE:    bindEnv("path", "namespace", "delete-local", "delete-cluster"),
		RunE: func(cmd *cobra.Command, args []string) (err error) {
			return runConfigGitRemoveCmd(cmd, newClient)
		},
	}

	// Global Config
	cfg, err := config.NewDefault()
	if err != nil {
		fmt.Fprintf(cmd.OutOrStdout(), "error loading config at '%v'. %v\n", config.File(), err)
	}

	// Function Context
	f, _ := fn.NewFunction(effectivePath())
	if f.Initialized() {
		cfg = cfg.Apply(f)
	}

	// Flags
	//
	// Globally-Configurable Flags:
	//   Options whose value may be defined globally may also exist on the
	//  contextually relevant function; but sets are flattened via cfg.Apply(f)
	cmd.Flags().StringP("namespace", "n", cfg.Namespace,
		"Deploy into a specific namespace. Will use function's current namespace by default if already deployed, and the currently active namespace if it can be determined. ($FUNC_NAMESPACE)")

	// Resources generated related Flags:
	cmd.Flags().Bool("delete-local", false, "Delete local resources (pipeline templates).")
	cmd.Flags().Bool("delete-cluster", false, "Delete cluster resources (credentials and config on the cluster).")

	addPathFlag(cmd)
	addVerboseFlag(cmd, cfg.Verbose)

	return cmd
}

type configGitRemoveConfig struct {
	// Globals (builder, confirm, registry, verbose)
	config.Global

	// Path of the function implementation on local disk. Defaults to current
	// working directory of the process.
	Path string

	Namespace string

	// informs whether any specific flag for deleting only a subset of resources has been set
	flagSet bool

	metadata pipelines.PacMetadata
}

// newConfigGitRemoveConfig creates a configGitRemoveConfig populated from command flags
func newConfigGitRemoveConfig(_ *cobra.Command) (c configGitRemoveConfig) {
	flagSet := false

	// decide what resources we should delete:
	// - by default all resources
	// - if any parameter is explicitly specified then get value from parameters
	deleteLocal := true
	deleteCluster := true
	if viper.HasChanged("delete-local") || viper.HasChanged("delete-cluster") {
		deleteLocal = viper.GetBool("delete-local")
		deleteCluster = viper.GetBool("delete-cluster")
		flagSet = true
	}

	c = configGitRemoveConfig{
		Namespace: viper.GetString("namespace"),

		flagSet: flagSet,

		metadata: pipelines.PacMetadata{
			ConfigureLocalResources:   deleteLocal,
			ConfigureClusterResources: deleteCluster,
		},
	}

	return c
}

func (c configGitRemoveConfig) Prompt(f fn.Function) (configGitRemoveConfig, error) {
	deleteAll := true
	// prompt if any flag hasn't been set yet
	if !c.flagSet {
		if err := survey.AskOne(&survey.Confirm{
			Message: "Do you want to delete all Git related resources?",
			Help:    "Delete Git config, local Pipeline resourdces and on the cluster resources.",
			Default: deleteAll,
		}, &deleteAll, survey.WithValidator(survey.Required)); err != nil {
			return c, err
		}
	}

	if !deleteAll {
		deleteLocal := true
		if err := survey.AskOne(&survey.Confirm{
			Message: "Do you want to delete all local Git related resources (Pipelines)?",
			Help:    "Delete local Pipeline resources created in the function project directory.",
			Default: deleteLocal,
		}, &deleteLocal, survey.WithValidator(survey.Required)); err != nil {
			return c, err
		}
		c.metadata.ConfigureLocalResources = deleteLocal

		deleteCluster := true
		if err := survey.AskOne(&survey.Confirm{
			Message: "Do you want to delete all Git related resources present on the cluster?",
			Help:    "Delete all Pipeline resources that were created on the cluster.",
			Default: deleteCluster,
		}, &deleteCluster, survey.WithValidator(survey.Required)); err != nil {
			return c, err
		}
		c.metadata.ConfigureClusterResources = deleteCluster
	}

	return c, nil
}

// Configure the given function.  Updates a function struct with all
// configurable values.  Note that the config already includes function's
// current values, as they were passed through via flag defaults.
func (c configGitRemoveConfig) Configure(f fn.Function) (fn.Function, error) {
	var err error

	if c.metadata.ConfigureLocalResources {
		f.Build.Git = fn.Git{}
	}

	// Save the function which has now been updated with flags/config
	if err = f.Write(); err != nil { // TODO: remove when client API uses 'f'
		return f, err
	}

	return f, nil
}

func runConfigGitRemoveCmd(cmd *cobra.Command, newClient ClientFactory) (err error) {
	var (
		cfg configGitRemoveConfig
		f   fn.Function
	)
	if err = config.CreatePaths(); err != nil { // for possible auth.json usage
		return
	}
	cfg = newConfigGitRemoveConfig(cmd)
	if f, err = fn.NewFunction(cfg.Path); err != nil {
		return
	}
	if cfg, err = cfg.Prompt(f); err != nil {
		return
	}
	if f, err = cfg.Configure(f); err != nil { // Updates f with deploy cfg
		return
	}

	client, done := newClient(ClientConfig{Namespace: cfg.Namespace, Verbose: cfg.Verbose})
	defer done()

	return client.RemovePAC(cmd.Context(), f, cfg.metadata)
}
