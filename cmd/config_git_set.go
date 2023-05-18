package cmd

import (
	"fmt"

	"github.com/AlecAivazis/survey/v2"
	"github.com/ory/viper"
	"github.com/spf13/cobra"

	pacgit "github.com/openshift-pipelines/pipelines-as-code/pkg/git"

	"knative.dev/func/pkg/config"
	fn "knative.dev/func/pkg/functions"
	"knative.dev/func/pkg/git"
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
		PreRunE:    bindEnv("path", "builder", "builder-image", "image", "registry", "namespace", "git-provider", "git-url", "git-branch", "git-dir", "gh-access-token", "config-local", "config-cluster", "config-remote"),
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
		"Container registry + registry namespace. (ex 'ghcr.io/myuser').  The full image name is automatically determined using this along with function name. ($FUNC_REGISTRY)")
	cmd.Flags().StringP("namespace", "n", cfg.Namespace,
		"Deploy into a specific namespace. Will use function's current namespace by default if already deployed, and the currently active namespace if it can be determined. ($FUNC_NAMESPACE)")

	// Function-Context Flags:
	// Options whose value is avaolable on the function with context only
	// (persisted but not globally configurable)
	builderImage := f.Build.BuilderImages[f.Build.Builder]
	cmd.Flags().StringP("builder-image", "", builderImage,
		"Specify a custom builder image for use by the builder other than its default. ($FUNC_BUILDER_IMAGE)")
	cmd.Flags().StringP("image", "i", f.Image, "Full image name in the form [registry]/[namespace]/[name]:[tag]@[digest]. This option takes precedence over --registry. Specifying digest is optional, but if it is given, 'build' and 'push' phases are disabled. ($FUNC_IMAGE)")

	// Git related Flags:
	cmd.Flags().String("git-provider", "",
		fmt.Sprintf("The type of the Git platform provider to setup webhook. This value is usually automatically generated from input URL, use this parameter to override this setting. Currently supported providers are %s.", git.SupportedProvidersList.PrettyString()))
	cmd.Flags().StringP("git-url", "g", "",
		"Repository url containing the function to build ($FUNC_GIT_URL)")
	cmd.Flags().StringP("git-branch", "t", "",
		"Git revision (branch) to be used when deploying via the Git repository ($FUNC_GIT_BRANCH)")
	cmd.Flags().StringP("git-dir", "d", "",
		"Directory in the Git repository containing the function (default is the root) ($FUNC_GIT_DIR)")

	// GitHub related Flags:
	cmd.Flags().String("gh-access-token", "",
		"GitHub Personal Access Token. For public repositories the scope is 'public_repo', for private is 'repo'. If you want to configure the webhook automatically, 'admin:repo_hook' is needed as well. Get more details: https://pipelines-as-code.pages.dev/docs/install/github_webhook/.")
	cmd.Flags().String("gh-webhook-secret", "",
		"GitHub Webhook Secret used for payload validation. If not specified, it will be generated automatically.")

	// Resources generated related Flags:
	cmd.Flags().Bool("config-local", false, "Configure local resources (pipeline templates).")
	cmd.Flags().Bool("config-cluster", false, "Configure cluster resources (credentials and config on the cluster).")
	cmd.Flags().Bool("config-remote", false, "Configure remote resources (webhook on the Git provider side).")

	addPathFlag(cmd)
	addVerboseFlag(cmd, cfg.Verbose)

	return cmd
}

type configGitSetConfig struct {
	buildConfig // further embeds config.Global

	Namespace string

	GitProvider   string
	GitURL        string
	GitRevision   string
	GitContextDir string

	ConfigureRemoteResourcesSet bool // whether ConfigureRemoteResources value has been set

	metadata pipelines.PacMetadata
}

// newConfigGitSetConfig creates a buildConfig populated from command flags and
// environment variables; in that precedence.
func newConfigGitSetConfig(cmd *cobra.Command) (c configGitSetConfig) {
	// decide what resources we should configure:
	// - by default all resources
	// - if any parameter is explicitly specified then get value from parameters
	configLocal := true
	configCluster := true
	configRemote := true

	configRemoteSet := false

	if viper.HasChanged("config-local") || viper.HasChanged("config-cluster") || viper.HasChanged("config-remote") {
		configLocal = viper.GetBool("config-local")
		configCluster = viper.GetBool("config-cluster")
		configRemote = viper.GetBool("config-remote")

		configRemoteSet = true
	}

	c = configGitSetConfig{
		buildConfig: newBuildConfig(),
		Namespace:   viper.GetString("namespace"),

		GitURL:        viper.GetString("git-url"),
		GitRevision:   viper.GetString("git-branch"),
		GitContextDir: viper.GetString("git-dir"),

		ConfigureRemoteResourcesSet: configRemoteSet,

		metadata: pipelines.PacMetadata{
			GitProvider:         viper.GetString("git-provider"),
			PersonalAccessToken: viper.GetString("gh-access-token"),
			WebhookSecret:       viper.GetString("gh-webhook-secret"),

			ConfigureLocalResources:   configLocal,
			ConfigureClusterResources: configCluster,
			ConfigureRemoteResources:  configRemote,
		},
	}

	return c
}

func (c configGitSetConfig) Prompt(f fn.Function) (configGitSetConfig, error) {
	var err error
	if c.buildConfig, err = c.buildConfig.Prompt(); err != nil {
		return c, err
	}

	// try to read git url from the local .git settings
	gitInfo := pacgit.GetGitInfo(c.Path)

	// prompt if git URL hasn't been set previously
	if c.GitURL == "" {
		url := f.Build.Git.URL
		if gitInfo.URL != "" {
			url = gitInfo.URL
		}
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
		revision := f.Build.Git.Revision
		if gitInfo.Branch != "" {
			revision = gitInfo.Branch
		}
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
	if !c.ConfigureRemoteResourcesSet {
		trigger := true
		if err := survey.AskOne(&survey.Confirm{
			Message: "Do you want to configure webhook trigger?",
			Help:    "Webhook trigger also running pipeline on a git event, ie: commit, push",
			Default: trigger,
		}, &trigger, survey.WithValidator(survey.Required)); err != nil {
			return c, err
		}
		c.metadata.ConfigureRemoteResources = trigger
		c.ConfigureRemoteResourcesSet = true
	}

	if c.metadata.ConfigureRemoteResources {
		// Configure Git provider
		if c.metadata.GitProvider == "" {
			provider, err := git.GitProviderName(c.GitURL)
			if err != nil {
				msg := "Please select the type of the Git platform provider to setup webhook:"
				if err = survey.AskOne(&survey.Select{
					Message: msg,
					Options: git.SupportedProvidersList,
					Default: 0,
				}, &provider); err != nil {
					return c, err
				}
			}
			c.metadata.GitProvider = provider
		}

		// prompt if PersonalAccessToken hasn't been set previously
		if c.metadata.PersonalAccessToken == "" {
			var personalAccessToken string
			if err := survey.AskOne(&survey.Password{
				Message: "Please enter the GitHub Personal Access Token:",
				Help:    "For public repositories the scope is 'public_repo', for private is 'repo'. If you want to configure the webhook automatically 'admin:repo_hook' is needed as well. Get more details: https://pipelines-as-code.pages.dev/docs/install/github_webhook/.",
			}, &personalAccessToken, survey.WithValidator(survey.Required)); err != nil {
				return c, err
			}
			c.metadata.PersonalAccessToken = personalAccessToken
		}
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
