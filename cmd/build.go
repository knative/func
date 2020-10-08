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
	buildCmd.Flags().StringP("builder", "b", "default", "Buildpacks builder")
	buildCmd.Flags().BoolP("confirm", "c", false, "Prompt to confirm all configuration options - $FAAS_CONFIRM")
	buildCmd.Flags().StringP("image", "i", "", "Optional full image name, in form [registry]/[namespace]/[name]:[tag] for example quay.io/myrepo/project.name:latest (overrides --registry) - $FAAS_IMAGE")
	buildCmd.Flags().StringP("path", "p", cwd(), "Path to the Function project directory - $FAAS_PATH")
	buildCmd.Flags().StringP("registry", "r", "", "Registry for built images, ex 'docker.io/myuser' or just 'myuser'.  Optional if --image provided. - $FAAS_REGISTRY")

	err := buildCmd.RegisterFlagCompletionFunc("builder", CompleteBuilderList)
	if err != nil {
		fmt.Println("Error while calling RegisterFlagCompletionFunc: ", err)
	}
}

var buildCmd = &cobra.Command{
	Use:   "build",
	Short: "Build an existing Function project as an OCI image",
	Long: `Build an existing Function project as an OCI image

Builds the Function project in the current directory or in the directory
specified by the --path flag. The faas.yaml file is read to determine the
image name and registry. If both of these values are unset in the
configuration file the --registry or -r flag should be provided and an image
name will be derived from the project name.

Any value provided for the --image flag will be persisted in the faas.yaml
configuration file. On subsequent invocations of the "build" command
these values will be read from the configuration file.

It's possible to use a custom Buildpack builder with the --builder flag.
The value may be image name e.g. "cnbs/sample-builder:bionic",
or reference to builderMaps in the config file e.g. "default".
`,
	SuggestFor: []string{"biuld", "buidl", "built"},
	PreRunE:    bindEnv("image", "path", "builder", "registry", "confirm"),
	RunE:       runBuild,
}

func runBuild(cmd *cobra.Command, _ []string) (err error) {
	config := newBuildConfig().Prompt()

	function, err := functionWithOverrides(config.Path, functionOverrides{Builder: config.Builder, Image: config.Image})
	if err != nil {
		return
	}

	// Check if the Function has been initialized
	if !function.Initialized() {
		return fmt.Errorf("the given path '%v' does not contain an initialized Function. Please create one at this path before deploying.", config.Path)
	}

	// If the Function does not yet have an image name and one was not provided on the command line
	if function.Image == "" {
		//  AND a --registry was not provided, then we need to
		// prompt for a registry from which we can derive an image name.
		if config.Registry == "" {
			fmt.Print("A registry for Function images is required. For example, 'docker.io/tigerteam'.\n\n")
			config.Registry = prompt.ForString("Registry for Function images", "")
			if config.Registry == "" {
				return fmt.Errorf("Unable to determine Function image name")
			}
		}

		// We have the registry, so let's use it to derive the Function image name
		config.Image = deriveImage(config.Image, config.Registry, config.Path)
		function.Image = config.Image
	}

	// All set, let's write changes in the config to the disk
	err = function.WriteConfig()
	if err != nil {
		return
	}

	builder := buildpacks.NewBuilder()
	builder.Verbose = config.Verbose

	client := faas.New(
		faas.WithVerbose(config.Verbose),
		faas.WithRegistry(config.Registry), // for deriving image name when --image not provided explicitly.
		faas.WithBuilder(builder))

	return client.Build(config.Path)
}

type buildConfig struct {
	// Image name in full, including registry, repo and tag (overrides
	// image name derivation based on Registry and Function Name)
	Image string

	// Path of the Function implementation on local disk. Defaults to current
	// working directory of the process.
	Path string

	// Push the resulting image to the registry after building.
	Push bool

	// Registry at which interstitial build artifacts should be kept.
	// This setting is ignored if Image is specified, which includes the full
	Registry string

	// Verbose logging.
	Verbose bool

	// Confirm: confirm values arrived upon from environment plus flags plus defaults,
	// with interactive prompting (only applicable when attached to a TTY).
	Confirm bool
	Builder string
}

func newBuildConfig() buildConfig {
	return buildConfig{
		Image:    viper.GetString("image"),
		Path:     viper.GetString("path"),
		Registry: viper.GetString("registry"),
		Verbose:  viper.GetBool("verbose"), // defined on root
		Confirm:  viper.GetBool("confirm"),
		Builder:  viper.GetString("builder"),
	}
}

// Prompt the user with value of config members, allowing for interaractive changes.
// Skipped if not in an interactive terminal (non-TTY), or if --confirm false (agree to
// all prompts) was set (default).
func (c buildConfig) Prompt() buildConfig {
	imageName := deriveImage(c.Image, c.Registry, c.Path)
	if !interactiveTerminal() || !c.Confirm {
		// If --confirm false or non-interactive, just print the image name
		fmt.Printf("Building image: %v\n", imageName)
		return c
	}
	return buildConfig{
		Path:    prompt.ForString("Path to project directory", c.Path),
		Image:   prompt.ForString("Image name", imageName, prompt.WithRequired(true)),
		Verbose: c.Verbose,
		// Registry not prompted for as it would be confusing when combined with explicit image.  Instead it is
		// inferred by the derived default for Image, which uses Registry for derivation.
	}
}
