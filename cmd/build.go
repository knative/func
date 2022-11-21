package cmd

import (
	"errors"
	"fmt"
	"strings"

	"github.com/AlecAivazis/survey/v2"
	"github.com/ory/viper"
	"github.com/spf13/cobra"

	"knative.dev/func/buildpacks"
	"knative.dev/func/config"
	"knative.dev/func/s2i"

	fn "knative.dev/func"
	"knative.dev/func/builders"
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
		RunE: func(cmd *cobra.Command, args []string) error {
			return runBuild(cmd, args, newClient)
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
		cfg = cfg.Apply(f) // defined values on f take precidence over cfg defaults
	}

	// Flags
	//
	// NOTE on falag defaults:
	// Use the config value when available, as this will include global static
	// defaults, user settings and the value from the function with context.
	// Use the function struct for flag flags which are not globally configurable
	//
	// Globally-Configurable Flags:
	// Options whose value may be defined globally may also exist on the
	// contextually relevant function; sets are flattened above via cfg.Apply(f)
	cmd.Flags().StringP("builder", "b", cfg.Builder,
		fmt.Sprintf("build strategy to use when creating the underlying image. Currently supported build strategies are %s.", KnownBuilders()))
	cmd.Flags().BoolP("confirm", "c", cfg.Confirm,
		"Prompt to confirm all configuration options (Env: $FUNC_CONFIRM)")
	cmd.Flags().StringP("registry", "r", cfg.Registry,
		"Registry + namespace part of the image to build, ex 'quay.io/myuser'.  The full image name is automatically determined (Env: $FUNC_REGISTRY)")

	// Function-Context Flags:
	// Options whose value is available on the function with context only
	// (persisted but not globally configurable)
	builderImage := f.Build.BuilderImages[f.Build.Builder]
	cmd.Flags().StringP("builder-image", "", builderImage,
		"Specify a custom builder image for use by the builder other than its default. (Env: $FUNC_BUILDER_IMAGE)")
	cmd.Flags().StringP("image", "i", f.Image,
		"Full image name in the form [registry]/[namespace]/[name]:[tag] (optional). This option takes precedence over --registry (Env: $FUNC_IMAGE)")

	// Static Flags:
	// Options which have static defaults only (not globally configurable nor
	// persisted with the function)
	cmd.Flags().BoolP("push", "u", false,
		"Attempt to push the function image to the configured registry after being successfully built")
	cmd.Flags().StringP("platform", "", "",
		"Optionally specify a specific platform to build for (e.g. linux/amd64).")
	setPathFlag(cmd)

	// Tab Completion
	if err := cmd.RegisterFlagCompletionFunc("builder", CompleteBuilderList); err != nil {
		fmt.Println("internal: error while calling RegisterFlagCompletionFunc: ", err)
	}
	if err := cmd.RegisterFlagCompletionFunc("builder-image", CompleteBuilderImageList); err != nil {
		fmt.Println("internal: error while calling RegisterFlagCompletionFunc: ", err)
	}

	// Help Text
	cmd.SetHelpFunc(defaultTemplatedHelp)

	return cmd
}

func runBuild(cmd *cobra.Command, _ []string, newClient ClientFactory) (err error) {
	if err = config.CreatePaths(); err != nil {
		return // see docker/creds potential mutation of auth.json
	}

	cfg, err := newBuildConfig().Prompt()
	if err != nil {
		return
	}

	if err = cfg.Validate(); err != nil {
		return
	}

	f, err := fn.NewFunction(cfg.Path)
	if err != nil {
		return
	}
	f = cfg.Configure(f) // Updates f at path to include buil request values

	// Checks if there is a difference between defined registry and its value used as a prefix in the image tag
	// In case of a mismatch a new image tag is created and used for build
	// Do not react if image tag has been changed outside configuration
	if f.Registry != "" && !cmd.Flags().Changed("image") && strings.Index(f.Image, "/") > 0 && !strings.HasPrefix(f.Image, f.Registry) {
		prfx := f.Registry
		if prfx[len(prfx)-1:] != "/" {
			prfx = prfx + "/"
		}
		sps := strings.Split(f.Image, "/")
		updImg := prfx + sps[len(sps)-1]
		fmt.Fprintf(cmd.ErrOrStderr(), "Warning: function has current image '%s' which has a different registry than the currently configured registry '%s'. The new image tag will be '%s'.  To use an explicit image, use --image.\n", f.Image, f.Registry, updImg)
		f.Image = updImg
	}

	// Client
	// Concrete implementations (ex builder) vary based on final effective config
	var builder fn.Builder
	if f.Build.Builder == builders.Pack {
		builder = buildpacks.NewBuilder(
			buildpacks.WithName(builders.Pack),
			buildpacks.WithVerbose(cfg.Verbose))
	} else if f.Build.Builder == builders.S2I {
		builder = s2i.NewBuilder(
			s2i.WithName(builders.S2I),
			s2i.WithPlatform(cfg.Platform),
			s2i.WithVerbose(cfg.Verbose))
	} else {
		return builders.ErrUnknownBuilder{Name: f.Build.Builder, Known: KnownBuilders()}
	}

	client, done := newClient(ClientConfig{Verbose: cfg.Verbose},
		fn.WithRegistry(cfg.Registry),
		fn.WithBuilder(builder))
	defer done()

	// TODO(lkingland): this write will be unnecessary when the client API is
	// udated to accept function structs rather than a path as argument.
	if err = f.Write(); err != nil {
		return
	}

	// Build and (optionally) push
	if err = client.Build(cmd.Context(), cfg.Path); err != nil {
		return
	}
	if cfg.Push {
		err = client.Push(cmd.Context(), cfg.Path)
	}

	// TODO(lkingland): when the above Build and Push calls are refactored to not
	// write the function but instead take and return a function struct, use
	// `reuturn f.Write()` below and remove from above such that function on disk
	// is only written on success and thus is always in a known valid state unless
	// manually edited.
	// return f.Write()
	return
}

type buildConfig struct {
	// Globals (builder, confirm, registry, verbose)
	config.Global

	// BuilderImage is the image (name or mapping) to use for building.  Usually
	// set automatically.
	BuilderImage string

	// Image name in full, including registry, repo and tag (overrides
	// image name derivation based on registry and function name)
	Image string

	// Path of the function implementation on local disk. Defaults to current
	// working directory of the process.
	Path string

	// Platform ofr resultant image (s2i builder only)
	Platform string

	// Push the resulting image to the registry after building.
	Push bool
}

// newBuildConfig gathers options into a single build request.
func newBuildConfig() buildConfig {
	return buildConfig{
		Global: config.Global{
			Builder:  viper.GetString("builder"),
			Confirm:  viper.GetBool("confirm"),
			Registry: registry(), // deferred defaulting
			Verbose:  viper.GetBool("verbose"),
		},
		BuilderImage: viper.GetString("builder-image"),
		Image:        viper.GetString("image"),
		Path:         viper.GetString("path"),
		Platform:     viper.GetString("platform"),
		Push:         viper.GetBool("push"),
	}
}

// Configure the given function.  Updates a function struct with all
// configurable values.  Note that buildConfig already includes function's
// current values, as they were passed through vi flag defaults, so overwriting
// is a noop.
func (c buildConfig) Configure(f fn.Function) fn.Function {
	f = c.Global.Configure(f)
	if f.Build.Builder != "" && c.BuilderImage != "" {
		f.Build.BuilderImages[f.Build.Builder] = c.BuilderImage
	}
	f.Image = c.Image
	// Path, Platform and Push are not part of a function's state.
	return f
}

// Prompt the user with value of config members, allowing for interaractive changes.
// Skipped if not in an interactive terminal (non-TTY), or if --confirm false (agree to
// all prompts) was set (default).
func (c buildConfig) Prompt() (buildConfig, error) {
	if !interactiveTerminal() {
		return c, nil
	}

	// If there is no registry nor explicit image name defined, the
	// Registry prompt is shown whether or not we are in confirm mode.
	// Otherwise, it is only showin if in confirm mode
	// NOTE: the default in this latter situation will ignore the current function
	// value and will always use the value from the config (flag or env variable).
	// This is not strictly correct and will be fixed when Global Config: Function
	// Context is available (PR#1416)
	f, err := fn.NewFunction(c.Path)
	if err != nil {
		return c, err
	}
	if (f.Registry == "" && c.Registry == "" && c.Image == "") || c.Confirm {
		fmt.Println("A registry for function images is required. For example, 'docker.io/tigerteam'.")
		err := survey.AskOne(
			&survey.Input{Message: "Registry for function images:", Default: c.Registry},
			&c.Registry,
			survey.WithValidator(NewRegistryValidator(c.Path)))
		if err != nil {
			return c, fn.ErrRegistryRequired
		}
		fmt.Println("Note: building a function the first time will take longer than subsequent builds")
	}

	// Remainder of prompts are optional and only shown if in --confirm mode
	if !c.Confirm {
		return c, nil
	}

	// Image Name Override
	// Calculate a better image name mesage which shows the value of the final
	// image name as it will be calclated if an explicit image name is not used.
	var imagePromptMessageSuffix string
	if name := deriveImage(c.Image, c.Registry, c.Path); name != "" {
		imagePromptMessageSuffix = fmt.Sprintf(". if not specified, the default '%v' will be used')", name)
	}

	qs := []*survey.Question{
		{
			Name: "image",
			Prompt: &survey.Input{
				Message: fmt.Sprintf("Image name to use (e.g. quay.io/boson/node-sample)%v:", imagePromptMessageSuffix),
			},
		},
		{
			Name: "path",
			Prompt: &survey.Input{
				Message: "Project path:",
				Default: c.Path,
			},
		},
	}
	//
	// TODO(lkingland): add confirmation prompts for other config members here
	//
	err = survey.Ask(qs, &c)
	return c, err
}

// Validate the config passes an initial consistency check
func (c buildConfig) Validate() (err error) {
	// Builder value must refer to a known builder short name
	if err = ValidateBuilder(c.Builder); err != nil {
		return
	}

	// Platform is only supportd with the S2I builder at this time
	if c.Platform != "" && c.Builder != builders.S2I {
		err = errors.New("Only S2I builds currently support specifying platform")
		return
	}

	return
}
