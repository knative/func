package cmd

import (
	"errors"
	"fmt"

	"github.com/AlecAivazis/survey/v2"
	"github.com/AlecAivazis/survey/v2/terminal"
	"github.com/ory/viper"
	"github.com/spf13/cobra"

	"knative.dev/kn-plugin-func/buildpacks"
	"knative.dev/kn-plugin-func/s2i"

	fn "knative.dev/kn-plugin-func"
	"knative.dev/kn-plugin-func/builders"
)

func NewBuildCmd(newClient ClientFactory) *cobra.Command {
	cmd := &cobra.Command{
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
{{.Name}} build --registry quay.io/myuser

# Build from the local directory, specifying the full image name
{{.Name}} build --image quay.io/myuser/myfunc

# Re-build, picking up a previously supplied image name from a local func.yml
{{.Name}} build

# Build using s2i instead of Buildpacks
{{.Name}} build --builder=s2i

# Build with a custom buildpack builder
{{.Name}} build --builder=pack --builder-image cnbs/sample-builder:bionic
`,
		SuggestFor: []string{"biuld", "buidl", "built"},
		PreRunE:    bindEnv("image", "path", "builder", "registry", "confirm", "push", "builder-image", "platform"),
	}

	cmd.Flags().StringP("builder", "b", builders.Default, fmt.Sprintf("build strategy to use when creating the underlying image. Currently supported build strategies are %s.", KnownBuilders()))
	cmd.Flags().StringP("builder-image", "", "", "builder image, either an as a an image name or a mapping name.\nSpecified value is stored in func.yaml (as 'builder' field) for subsequent builds. ($FUNC_BUILDER_IMAGE)")
	cmd.Flags().BoolP("confirm", "c", false, "Prompt to confirm all configuration options (Env: $FUNC_CONFIRM)")
	cmd.Flags().StringP("image", "i", "", "Full image name in the form [registry]/[namespace]/[name]:[tag] (optional). This option takes precedence over --registry (Env: $FUNC_IMAGE)")
	cmd.Flags().StringP("registry", "r", GetDefaultRegistry(), "Registry + namespace part of the image to build, ex 'quay.io/myuser'.  The full image name is automatically determined (Env: $FUNC_REGISTRY)")
	cmd.Flags().BoolP("push", "u", false, "Attempt to push the function image after being successfully built")
	cmd.Flags().Lookup("push").NoOptDefVal = "true" // --push == --push=true
	cmd.Flags().StringP("platform", "", "", "Target platform to build (e.g. linux/amd64).")
	setPathFlag(cmd)

	if err := cmd.RegisterFlagCompletionFunc("builder", CompleteBuilderList); err != nil {
		fmt.Println("internal: error while calling RegisterFlagCompletionFunc: ", err)
	}

	if err := cmd.RegisterFlagCompletionFunc("builder-image", CompleteBuilderImageList); err != nil {
		fmt.Println("internal: error while calling RegisterFlagCompletionFunc: ", err)
	}

	cmd.SetHelpFunc(defaultTemplatedHelp)

	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		return runBuild(cmd, args, newClient)
	}

	return cmd
}

func runBuild(cmd *cobra.Command, _ []string, newClient ClientFactory) (err error) {
	// Populate a command config from environment variables, flags and potentially
	// interactive user prompts if in confirm mode.
	config, err := newBuildConfig().Prompt()
	if err != nil {
		if errors.Is(err, terminal.InterruptErr) {
			return nil
		}
	}

	// Validate the config
	if err = config.Validate(); err != nil {
		return
	}

	// Load the Function at path, and if it is initialized, update it with
	// pertinent values from the config.
	//
	// NOTE: the checks for .Changed and altered conditionals for defaults will
	// be removed when Global Config is integreated, because the config object
	// will at that time contain the final value for the attribute, taking into
	// account whether or not the value was altered via flags or env variables.
	// This condition is also only necessary for config members whose default
	// value deviates from the zero value.
	f, err := fn.NewFunction(config.Path)
	if err != nil {
		return
	}
	if !f.Initialized() {
		return fmt.Errorf("'%v' does not contain an initialized function", config.Path)
	}
	if f.Registry == "" || cmd.Flags().Changed("registry") {
		// Sets default AND accepts any user-provided overrides
		f.Registry = config.Registry
	}
	if f.Builder == "" || cmd.Flags().Changed("builder") {
		// Sets default AND accepts any user-provided overrides
		f.Builder = config.Builder
	}
	if config.Image != "" {
		f.Image = config.Image
	}
	if config.Builder != "" {
		f.Builder = config.Builder
	}
	if config.BuilderImage != "" {
		f.BuilderImages[config.Builder] = config.BuilderImage
	}

	// Validate that a builder short-name was obtained, whether that be from
	// the function's prior state, or the value of flags/environment.
	if err = ValidateBuilder(f.Builder); err != nil {
		return
	}

	// Choose a builder based on the value of the --builder flag
	var builder fn.Builder
	if f.Builder == builders.Pack {
		builder = buildpacks.NewBuilder(
			buildpacks.WithName(builders.Pack),
			buildpacks.WithVerbose(config.Verbose))
	} else if f.Builder == builders.S2I {
		builder = s2i.NewBuilder(
			s2i.WithName(builders.S2I),
			s2i.WithPlatform(config.Platform),
			s2i.WithVerbose(config.Verbose))
	} else {
		err = fmt.Errorf("builder '%v' is not recognized", f.Builder)
		return
	}

	client, done := newClient(ClientConfig{Verbose: config.Verbose},
		fn.WithRegistry(config.Registry),
		fn.WithBuilder(builder))
	defer done()

	// Default Client Registry, Function Registry or explicit Image is required
	if client.Registry() == "" && f.Registry == "" && f.Image == "" {
		// It is not necessary that we validate here, since the client API has
		// its own validation, but it does give us the opportunity to show a very
		// cli-specific and detailed error message and (at least for now) default
		// to an interactive prompt.
		if interactiveTerminal() {
			fmt.Println("A registry for function images is required. For example, 'docker.io/tigerteam'.")
			if err = survey.AskOne(
				&survey.Input{Message: "Registry for function images:"},
				&config.Registry, survey.WithValidator(
					NewRegistryValidator(config.Path))); err != nil {
				return ErrRegistryRequired
			}
			fmt.Println("Note: building a function the first time will take longer than subsequent builds")
		}

		return ErrRegistryRequired
	}

	// This preemptive write call will be unnecessary when the API is updated
	// to use Function instances rather than file paths. For now it must write
	// even if the command fails later.  Not ideal.
	if err = f.Write(); err != nil {
		return
	}

	if err = client.Build(cmd.Context(), config.Path); err != nil {
		return
	}
	if config.Push {
		err = client.Push(cmd.Context(), config.Path)
	}

	return
}

type buildConfig struct {
	// Image name in full, including registry, repo and tag (overrides
	// image name derivation based on registry and function name)
	Image string

	// Path of the function implementation on local disk. Defaults to current
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

	// Builder is the name of the subsystem that will complete the underlying
	// build (Pack, s2i, remote pipeline, etc).  Currently ad-hoc rather than
	// an enumerated field.  See the Client constructory for logic.
	Builder string

	// BuilderImage is the image (name or mapping) to use for building.  Usually
	// set automatically.
	BuilderImage string

	Platform string
}

func newBuildConfig() buildConfig {
	return buildConfig{
		Image:        viper.GetString("image"),
		Path:         getPathFlag(),
		Registry:     viper.GetString("registry"),
		Verbose:      viper.GetBool("verbose"), // defined on root
		Confirm:      viper.GetBool("confirm"),
		Builder:      viper.GetString("builder"),
		BuilderImage: viper.GetString("builder-image"),
		Push:         viper.GetBool("push"),
		Platform:     viper.GetString("platform"),
	}
}

// Prompt the user with value of config members, allowing for interaractive changes.
// Skipped if not in an interactive terminal (non-TTY), or if --confirm false (agree to
// all prompts) was set (default).
func (c buildConfig) Prompt() (buildConfig, error) {
	if !interactiveTerminal() || !c.Confirm {
		return c, nil
	}

	imageName := deriveImage(c.Image, c.Registry, c.Path)

	var qs = []*survey.Question{
		{
			Name: "path",
			Prompt: &survey.Input{
				Message: "Project path:",
				Default: c.Path,
			},
		},
		{
			Name: "image",
			Prompt: &survey.Input{
				Message: "Full image name (e.g. quay.io/boson/node-sample):",
				Default: imageName,
			},
		},
	}
	err := survey.Ask(qs, &c)
	if err != nil {
		return c, err
	}

	// if the result of imageName is empty (unset, underivable),
	// and the value of c.Image is empty (none provided explicitly by flag, env
	// variable or prompt), then try one more time to get enough to to derive an
	// image name: explicitly ask for registry.
	if imageName == "" && c.Image == "" {
		qs = []*survey.Question{
			{
				Name: "registry",
				Prompt: &survey.Input{
					Message: "Registry for function image:",
					Default: c.Registry, // This should be innefectual
				},
			},
		}
	}
	err = survey.Ask(qs, &c)
	return c, err
}

// Validate the config passes an initial consistency check
func (c buildConfig) Validate() (err error) {

	if c.Platform != "" && c.Builder != builders.S2I {
		err = errors.New("Only S2I builds currently support specifying platform")
		return
	}

	return
}
