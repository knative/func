package cmd

import (
	"fmt"

	"github.com/ory/viper"
	"github.com/spf13/cobra"

	"github.com/boson-project/faas"
	"github.com/boson-project/faas/buildpacks"
	"github.com/boson-project/faas/prompt"
)

func init() {
	root.AddCommand(buildCmd)
	buildCmd.Flags().BoolP("confirm", "c", false, "Prompt to confirm all configuration options - $FAAS_CONFIRM")
	buildCmd.Flags().StringP("image", "i", "", "Optional full image name, in form [registry]/[namespace]/[name]:[tag] for example quay.io/myrepo/project.name:latest (overrides --repository) - $FAAS_IMAGE")
	buildCmd.Flags().StringP("path", "p", cwd(), "Path to the Function project directory - $FAAS_PATH")
	buildCmd.Flags().StringP("repository", "r", "", "Repository for built images, ex 'docker.io/myuser' or just 'myuser'.  Optional if --image provided. - $FAAS_REPOSITORY")
}

var buildCmd = &cobra.Command{
	Use:   "build",
	Short: "Build an existing Function project as an OCI image",
	Long: `Build an existing Function project as an OCI image

Builds the Function project in the current directory or in the directory
specified by the --path flag. The faas.yaml file is read to determine the
image name and repository. If both of these values are unset in the
configuration file the --repository or -r flag should be provided and an image
name will be derived from the project name.

Any value provided for --image or --repository will be persisted in the
faas.yaml configuration file. On subsequent invocations of the "build" command
these values will be read from the configuration file.
`,
	SuggestFor: []string{"biuld", "buidl", "built"},
	PreRunE:    bindEnv("image", "path", "repository", "confirm"),
	RunE:       runBuild,
}

func runBuild(cmd *cobra.Command, _ []string) (err error) {
	config := newBuildConfig()
	function, err := functionWithOverrides(config.Path, "", config.Image)
	if err != nil {
		return
	}

	// If the Function does not yet have an image name, and one was not provided
	// on the command line AND a --repository was not provided, then we need to
	// prompt for a repository from which we can derive an image name.
	if function.Image == "" && config.Repository == "" {
		fmt.Print("A repository for Function images is required. For example, 'docker.io/tigerteam'.\n\n")
		config.Repository = prompt.ForString("Repository for Function images", "")
		if config.Repository == "" {
			return fmt.Errorf("Unable to determine Function image name")
		}
	}

	builder := buildpacks.NewBuilder()
	builder.Verbose = config.Verbose

	client := faas.New(
		faas.WithVerbose(config.Verbose),
		faas.WithRepository(config.Repository), // for deriving image name when --image not provided explicitly.
		faas.WithBuilder(builder))

	config.Prompt()

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

	// Confirm: confirm values arrived upon from environment plus flags plus defaults,
	// with interactive prompting (only applicable when attached to a TTY).
	Confirm bool
}

func newBuildConfig() buildConfig {
	return buildConfig{
		Image:      viper.GetString("image"),
		Path:       viper.GetString("path"),
		Repository: viper.GetString("repository"),
		Verbose:    viper.GetBool("verbose"), // defined on root
		Confirm:    viper.GetBool("confirm"),
	}
}

// Prompt the user with value of config members, allowing for interaractive changes.
// Skipped if not in an interactive terminal (non-TTY), or if --confirm false (agree to
// all prompts) was set (default).
func (c buildConfig) Prompt() buildConfig {
	imageName := deriveImage(c.Image, c.Repository, c.Path)
	if !interactiveTerminal() || !c.Confirm {
		// If --confirm false or non-interactive, just print the image name
		fmt.Printf("Building image: %v\n", imageName)
		return c
	}
	return buildConfig{
		Path:    prompt.ForString("Path to project directory", c.Path),
		Image:   prompt.ForString("Image name", imageName, prompt.WithRequired(true)),
		Verbose: c.Verbose,
		// Repository not prompted for as it would be confusing when combined with explicit image.  Instead it is
		// inferred by the derived default for Image, which uses Repository for derivation.
	}
}
