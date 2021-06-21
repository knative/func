package cmd

import (
	"fmt"

	"github.com/AlecAivazis/survey/v2"
	"github.com/AlecAivazis/survey/v2/terminal"
	"github.com/ory/viper"
	"github.com/spf13/cobra"

	bosonFunc "github.com/boson-project/func"
	"github.com/boson-project/func/buildpacks"
	"github.com/boson-project/func/progress"
)

func init() {
	root.AddCommand(buildCmd)
	buildCmd.Flags().StringP("builder", "b", "", "Buildpack builder, either an as a an image name or a mapping name.\nSpecified value is stored in func.yaml for subsequent builds.")
	buildCmd.Flags().BoolP("confirm", "c", false, "Prompt to confirm all configuration options (Env: $FUNC_CONFIRM)")
	buildCmd.Flags().StringP("image", "i", "", "Full image name in the orm [registry]/[namespace]/[name]:[tag] (optional). This option takes precedence over --registry (Env: $FUNC_IMAGE")
	buildCmd.Flags().StringP("path", "p", cwd(), "Path to the project directory (Env: $FUNC_PATH)")
	buildCmd.Flags().StringP("registry", "r", "", "Registry + namespace part of the image to build, ex 'quay.io/myuser'.  The full image name is automatically determined based on the local directory name. If not provided the registry will be taken from func.yaml (Env: $FUNC_REGISTRY)")

	err := buildCmd.RegisterFlagCompletionFunc("builder", CompleteBuilderList)
	if err != nil {
		fmt.Println("internal: error while calling RegisterFlagCompletionFunc: ", err)
	}
}

var buildCmd = &cobra.Command{
	Use:   "build",
	Short: "Build a function project as a container image",
	Long: `Build a function project as a container image

This command builds the function project in the current directory or in the directory
specified by --path. The result will be a container image that is pushed to a registry.
The func.yaml file is read to determine the image name and registry. 
If the project has not already been built, either --registry or --image must be provided 
and the image name is stored in the configuration file.
`,
	Example: `
# Build from the local directory, using the given registry as target.
# The full image name will be determined automatically based on the
# project directory name
kn func build --registry quay.io/myuser

# Build from the local directory, specifying the full image name
kn func build --image quay.io/myuser/myfunc

# Re-build, picking up a previously supplied image name from a local func.yml
kn func build

# Build with a custom buildpack builder
kn func build --builder cnbs/sample-builder:bionic
`,
	SuggestFor: []string{"biuld", "buidl", "built"},
	PreRunE:    bindEnv("image", "path", "builder", "registry", "confirm"),
	RunE:       runBuild,
}

func runBuild(cmd *cobra.Command, _ []string) (err error) {
	config, err := newBuildConfig().Prompt()
	if err != nil {
		if err == terminal.InterruptErr {
			return nil
		}
		return
	}

	function, err := functionWithOverrides(config.Path, functionOverrides{Builder: config.Builder, Image: config.Image})
	if err != nil {
		return
	}

	// Check if the Function has been initialized
	if !function.Initialized() {
		return fmt.Errorf("the given path '%v' does not contain an initialized function. Please create one at this path before deploying", config.Path)
	}

	// If the Function does not yet have an image name and one was not provided on the command line
	if function.Image == "" {
		//  AND a --registry was not provided, then we need to
		// prompt for a registry from which we can derive an image name.
		if config.Registry == "" {
			fmt.Println("A registry for Function images is required. For example, 'docker.io/tigerteam'.")

			err = survey.AskOne(
				&survey.Input{Message: "Registry for Function images:"},
				&config.Registry, survey.WithValidator(survey.Required))
			if err != nil {
				if err == terminal.InterruptErr {
					return nil
				}
				return
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

	listener := progress.New()
	listener.Verbose = config.Verbose
	defer listener.Done()

	context := cmd.Context()
	go func() {
		<-context.Done()
		listener.Done()
	}()

	client := bosonFunc.New(
		bosonFunc.WithVerbose(config.Verbose),
		bosonFunc.WithRegistry(config.Registry), // for deriving image name when --image not provided explicitly.
		bosonFunc.WithBuilder(builder),
		bosonFunc.WithProgressListener(listener))

	return client.Build(context, config.Path)
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
func (c buildConfig) Prompt() (buildConfig, error) {
	imageName := deriveImage(c.Image, c.Registry, c.Path)
	if !interactiveTerminal() || !c.Confirm {
		return c, nil
	}

	bc := buildConfig{Verbose: c.Verbose}

	var qs = []*survey.Question{
		{
			Name: "path",
			Prompt: &survey.Input{
				Message: "Project path:",
				Default: c.Path,
			},
			Validate: survey.Required,
		},
		{
			Name: "image",
			Prompt: &survey.Input{
				Message: "Full image name (e.g. quay.io/boson/node-sample):",
				Default: imageName,
			},
			Validate: survey.Required,
		},
	}
	err := survey.Ask(qs, &bc)

	return bc, err
}
