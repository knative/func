package cmd

import (
	"github.com/ory/viper"
	"github.com/spf13/cobra"

	"github.com/boson-project/faas"
	"github.com/boson-project/faas/buildpacks"
	"github.com/boson-project/faas/prompt"
)

func init() {
	root.AddCommand(buildCmd)
	buildCmd.Flags().StringP("image", "i", "", "Optional full image name, in form [registry]/[namespace]/[name]:[tag] for example quay.io/myrepo/project.name:latest (overrides --repository) - $FAAS_IMAGE")
	buildCmd.Flags().StringP("path", "p", cwd(), "Path to the Function project directory - $FAAS_PATH")
	buildCmd.Flags().StringP("repository", "r", "", "Repository for built images, ex 'docker.io/myuser' or just 'myuser'.  Optional if --image provided. - $FAAS_REPOSITORY")
	buildCmd.Flags().BoolP("yes", "y", false, "When in interactive mode (attached to a TTY) skip prompts. - $FAAS_YES")
}

var buildCmd = &cobra.Command{
	Use:        "build [options]",
	Short:      "Build an existing Function project as an OCI image",
	SuggestFor: []string{"biuld", "buidl", "built"},
	PreRunE:    bindEnv("image", "path", "repository", "yes"),
	RunE:       runBuild,
}

func runBuild(cmd *cobra.Command, _ []string) (err error) {
	config := newBuildConfig().Prompt()

	builder := buildpacks.NewBuilder()
	builder.Verbose = config.Verbose

	client := faas.New(
		faas.WithVerbose(config.Verbose),
		faas.WithRepository(config.Repository), // for deriving image name when --image not provided explicitly.
		faas.WithBuilder(builder))

	// overrideImage name for built images, if --image provided.
	if err = overrideImage(config.Path, config.Image); err != nil {
		return
	}

	return client.Build(config.Path)
}

type buildConfig struct {
	// Image name in full, including registry, repo and tag (overrides
	// image name derivation based on Repository and Function Name)
	Image string

	// Path of the Function implementation on local disk. Defaults to current
	// working directory of the process.
	Path string

	// Push the resultnat image to the repository after building.
	Push bool

	// Repository at which interstitial build artifacts should be kept.
	// Registry is optional and is defaulted to faas.DefaultRegistry.
	// ex: "quay.io/myrepo" or "myrepo"
	// This setting is ignored if Image is specified, which includes the full
	Repository string

	// Verbose logging.
	Verbose bool

	// Yes: agree to values arrived upon from environment plus flags plus defaults,
	// and skip the interactive prompting (only applicable when attached to a TTY).
	Yes bool
}

func newBuildConfig() buildConfig {
	return buildConfig{
		Image:      viper.GetString("image"),
		Path:       viper.GetString("path"),
		Repository: viper.GetString("repository"),
		Verbose:    viper.GetBool("verbose"), // defined on root
		Yes:        viper.GetBool("yes"),
	}
}

// Prompt the user with value of config members, allowing for interaractive changes.
// Skipped if not in an interactive terminal (non-TTY), or if --yes (agree to
// all prompts) was explicitly set.
func (c buildConfig) Prompt() buildConfig {
	if !interactiveTerminal() || c.Yes {
		return c
	}
	return buildConfig{
		Path:    prompt.ForString("Path to project directory", c.Path),
		Image:   prompt.ForString("Resulting image name", deriveImage(c.Image, c.Repository, c.Path), prompt.WithRequired(true)),
		Verbose: prompt.ForBool("Verbose logging", c.Verbose),
		// Repository not prompted for as it would be confusing when combined with explicit image.  Instead it is
		// inferred by the derived default for Image, which uses Repository for derivation.
	}
}
