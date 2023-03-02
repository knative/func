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

func NewConfigGitSetCmd(newClient ClientFactory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "set",
		Short: "Set Git settings in the function configuration",
		Long: `Set Git settings in the function configuration

	Interactive prompt to set Git settings in the function project in the current
	directory or from the directory specified with --path.
	`,
		SuggestFor: []string{"add", "ad", "update", "create", "insert", "append"},
		PreRunE:    bindEnv("path", "builder", "builder-image", "image", "registry"),
		RunE: func(cmd *cobra.Command, args []string) (err error) {
			return runConfigGitSetCmd(cmd, newClient)
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
	cmd.Flags().StringP("builder", "b", cfg.Builder,
		fmt.Sprintf("Builder to use when creating the function's container. Currently supported builders are %s.", KnownBuilders()))
	cmd.Flags().StringP("registry", "r", cfg.Registry,
		"Container registry + registry namespace. (ex 'ghcr.io/myuser').  The full image name is automatically determined using this along with function name. (Env: $FUNC_REGISTRY)")
	cmd.Flags().StringP("namespace", "n", cfg.Namespace,
		"Deploy into a specific namespace. Will use function's current namespace by default if already deployed, and the currently active namespace if it can be determined. (Env: $FUNC_NAMESPACE)")

	// Function-Context Flags:
	// Options whose value is avaolable on the function with context only
	// (persisted but not globally configurable)
	builderImage := f.Build.BuilderImages[f.Build.Builder]
	cmd.Flags().StringP("builder-image", "", builderImage,
		"Specify a custom builder image for use by the builder other than its default. ($FUNC_BUILDER_IMAGE)")
	cmd.Flags().StringP("image", "i", f.Image, "Full image name in the form [registry]/[namespace]/[name]:[tag]@[digest]. This option takes precedence over --registry. Specifying digest is optional, but if it is given, 'build' and 'push' phases are disabled. (Env: $FUNC_IMAGE)")

	addPathFlag(cmd)
	addVerboseFlag(cmd, cfg.Verbose)

	return cmd
}

type configGitSetConfig struct {
	buildConfig // further embeds config.Global

	Namespace string

	GitURL        string
	GitRevision   string
	GitContextDir string

	WebhookTrigger           bool
	WebhookTriggerSet        bool // whether WebhookTrigger value has been set
	WebhookTriggerAutoConfig bool // whether to configure WebhookTrigger automatically

	metadata pipelines.PacMetadata
}

// newConfigGitSetConfig creates a buildConfig populated from command flags and
// environment variables; in that precedence.
func newConfigGitSetConfig(cmd *cobra.Command) (c configGitSetConfig) {
	c = configGitSetConfig{
		buildConfig: newBuildConfig(),
		Namespace:   viper.GetString("namespace"),

		metadata: pipelines.PacMetadata{
			ConfigureLocalResources:   true,
			ConfigureClusterResources: true,
			ConfigureRemoteResources:  true,
		},
	}

	return c
}

func (c configGitSetConfig) Prompt(f fn.Function) (configGitSetConfig, error) {
	var err error
	if c.buildConfig, err = c.buildConfig.Prompt(); err != nil {
		return c, err
	}

	// prompt if git URL hasn't been set previously
	if c.GitURL == "" {
		// TODO we can try to read git url from the local .git settings
		url := f.Build.Git.URL
		if err := survey.AskOne(&survey.Input{
			Message: "The URL to Git repository with the function source code:",
			Default: url,
		}, &url, survey.WithValidator(survey.Required)); err != nil {
			return c, err
		}
		c.GitURL = url
	}

	// prompt if git revision hasn't been set previously
	if c.GitRevision == "" {
		// TODO we can try to read git url from the local .git settings
		revision := f.Build.Git.Revision
		if err := survey.AskOne(&survey.Input{
			Message: "The Git branch or tag we are targeting:",
			Help:    "ie: main, refs/tags/*",
			Default: revision,
		}, &revision); err != nil {
			return c, err
		}
		c.GitRevision = revision
	}

	// prompt if contextDir hasn't been set previously
	if c.GitContextDir == "" {
		contextDir := f.Build.Git.ContextDir
		if err := survey.AskOne(&survey.Input{
			Message: "A subpath within the repository:",
			Help:    "A subpath within the repository where the source code of a function is located.",
			Default: contextDir,
		}, &contextDir); err != nil {
			return c, err
		}
		c.GitContextDir = contextDir
	}

	// prompt if webhook trigger setting hasn't been set previously
	if !c.WebhookTriggerSet {
		trigger := true
		if err := survey.AskOne(&survey.Confirm{
			Message: "Do you want to configure webhook trigger?",
			Help:    "Webhook trigger also running pipeline on a git event, ie: commit, push",
			Default: trigger,
		}, &trigger, survey.WithValidator(survey.Required)); err != nil {
			return c, err
		}
		c.WebhookTrigger = trigger
		c.WebhookTriggerSet = true
	}

	if c.WebhookTrigger {
		// prompt if PersonalAccessToken hasn't been set previously
		if c.metadata.PersonalAccessToken == "" {
			var personalAccessToken string
			if err := survey.AskOne(&survey.Password{
				Message: "Please enter the GitHub access token:",
			}, &personalAccessToken, survey.WithValidator(survey.Required)); err != nil {
				return c, err
			}
			c.metadata.PersonalAccessToken = personalAccessToken
		}

		// TODO prompt here if user want to configure remote webhook automatically (default)
		// or manauly - print neccesary info then
		// ie: c.WebhookTriggerAutoConfig
	}

	return c, nil
}

func (c configGitSetConfig) Validate(cmd *cobra.Command) (err error) {
	// Bubble validation
	if err = c.buildConfig.Validate(); err != nil {
		return
	}

	return
}

// Configure the given function.  Updates a function struct with all
// configurable values.  Note that the config already includes function's
// current values, as they were passed through via flag defaults.
func (c configGitSetConfig) Configure(f fn.Function) (fn.Function, error) {
	var err error

	// Bubble configure request
	//
	// The member values on the config object now take absolute precidence
	// because they include 1) static config 2) user's global config
	// 3) Environment variables and 4) flag values (which were set with their
	// default being 1-3).
	f = c.buildConfig.Configure(f) // also configures .buildConfig.Global

	// Configure basic members
	f.Build.Git.URL = c.GitURL
	f.Build.Git.ContextDir = c.GitContextDir
	f.Build.Git.Revision = c.GitRevision // TODO: should match; perhaps "refSpec"

	// Save the function which has now been updated with flags/config
	if err = f.Write(); err != nil { // TODO: remove when client API uses 'f'
		return f, err
	}

	return f, nil
}

func runConfigGitSetCmd(cmd *cobra.Command, newClient ClientFactory) (err error) {
	var (
		cfg configGitSetConfig
		f   fn.Function
	)
	if err = config.CreatePaths(); err != nil { // for possible auth.json usage
		return
	}
	cfg = newConfigGitSetConfig(cmd)
	if f, err = fn.NewFunction(cfg.Path); err != nil {
		return
	}
	if cfg, err = cfg.Prompt(f); err != nil {
		return
	}
	if err = cfg.Validate(cmd); err != nil {
		return
	}
	if f, err = cfg.Configure(f); err != nil { // Updates f with deploy cfg
		return
	}

	client, done := newClient(ClientConfig{Namespace: cfg.Namespace, Verbose: cfg.Verbose}, fn.WithRegistry(cfg.Registry))
	defer done()

	return client.ConfigurePAC(cmd.Context(), f, cfg.metadata)
}
