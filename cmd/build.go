package cmd

import (
	"errors"
	"fmt"
	"strings"

	"github.com/AlecAivazis/survey/v2"
	"github.com/ory/viper"
	"github.com/spf13/cobra"

	"knative.dev/func/pkg/builders"
	"knative.dev/func/pkg/builders/buildpacks"
	"knative.dev/func/pkg/builders/s2i"
	"knative.dev/func/pkg/config"
	fn "knative.dev/func/pkg/functions"
)

func NewBuildCmd(newClient ClientFactory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "build",
		Short: "Build a function container",
		Long: `
NAME
	{{rootCmdUse}} build - Build a function container locally withoud deploying

SYNOPSIS
	{{rootCmdUse}} build [-r|--registry] [--builder] [--builder-image] [--push]
	             [--platform] [-p|--path] [-c|--confirm] [-v|--verbose]

DESCRIPTION

	Builds a function's container image and optionally pushes it to the
	configured container registry.

	By default building is handled automatically when deploying (see the deploy
	subcommand). However, sometimes it is useful to build a function container
	outside of this normal deployment process, for example for testing or during
	composition when integrating with other systems. Additionally, the container
	can be pushed to the configured registry using the --push option.

	When building a function for the first time, either a registry or explicit
	image name is required.  Subsequent builds will reuse these option values.

EXAMPLES

	o Build a function container using the given registry.
	  The full image name will be calculated using the registry and function name.
	  $ {{rootCmdUse}} build --registry registry.example.com/alice

	o Build a function container using an explicit image name, ignoring registry
	  and function name.
	  $ {{rootCmdUse}} build --image registry.example.com/alice/f:latest

	o Rebuild a function using prior values to determine container name.
	  $ {{rootCmdUse}} build

	o Build a function specifying the Source-to-Image (S2I) builder
	  $ {{rootCmdUse}} build --builder=s2i

	o Build a function specifying the Pack builder with a custom Buildpack
	  builder image.
	  $ {{rootCmdUse}} build --builder=pack --builder-image=cnbs/sample-builder:bionic

`,
		SuggestFor: []string{"biuld", "buidl", "built"},
		PreRunE:    bindEnv("image", "path", "builder", "registry", "confirm", "push", "builder-image", "platform", "verbose"),
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
		cfg = cfg.Apply(f) // defined values on f take precedence over cfg defaults
	}

	// Flags
	//
	// NOTE on flag defaults:
	// Use the config value when available, as this will include global static
	// defaults, user settings and the value from the function with context.
	// Use the function struct for flag flags which are not globally configurable
	//
	// Globally-Configurable Flags:
	// Options whose value may be defined globally may also exist on the
	// contextually relevant function; sets are flattened above via cfg.Apply(f)
	cmd.Flags().StringP("builder", "b", cfg.Builder,
		fmt.Sprintf("Builder to use when creating the function's container. Currently supported builders are %s. ($FUNC_BUILDER)", KnownBuilders()))
	cmd.Flags().StringP("registry", "r", cfg.Registry,
		"Container registry + registry namespace. (ex 'ghcr.io/myuser').  The full image name is automatically determined using this along with function name. ($FUNC_REGISTRY)")

	// Function-Context Flags:
	// Options whose value is available on the function with context only
	// (persisted but not globally configurable)
	builderImage := f.Build.BuilderImages[f.Build.Builder]
	cmd.Flags().StringP("builder-image", "", builderImage,
		"Specify a custom builder image for use by the builder other than its default. ($FUNC_BUILDER_IMAGE)")
	cmd.Flags().StringP("image", "i", f.Image,
		"Full image name in the form [registry]/[namespace]/[name]:[tag] (optional). This option takes precedence over --registry ($FUNC_IMAGE)")

	// Static Flags:
	// Options which have static defaults only (not globally configurable nor
	// persisted with the function)
	cmd.Flags().BoolP("push", "u", false,
		"Attempt to push the function image to the configured registry after being successfully built")
	cmd.Flags().StringP("platform", "", "",
		"Optionally specify a target platform, for example \"linux/amd64\" when using the s2i build strategy")

	// Oft-shared flags:
	addConfirmFlag(cmd, cfg.Confirm)
	addPathFlag(cmd)
	addVerboseFlag(cmd, cfg.Verbose)

	// Tab Completion
	if err := cmd.RegisterFlagCompletionFunc("builder", CompleteBuilderList); err != nil {
		fmt.Println("internal: error while calling RegisterFlagCompletionFunc: ", err)
	}
	if err := cmd.RegisterFlagCompletionFunc("builder-image", CompleteBuilderImageList); err != nil {
		fmt.Println("internal: error while calling RegisterFlagCompletionFunc: ", err)
	}

	return cmd
}

func runBuild(cmd *cobra.Command, _ []string, newClient ClientFactory) (err error) {
	var (
		cfg buildConfig
		f   fn.Function
	)
	if err = config.CreatePaths(); err != nil { // for possible auth.json usage
		return
	}
	if cfg, err = newBuildConfig().Prompt(); err != nil {
		return
	}
	if err = cfg.Validate(); err != nil {
		return
	}
	if f, err = fn.NewFunction(cfg.Path); err != nil {
		return
	}
	f = cfg.Configure(f) // Updates f at path to include build request values

	// TODO: this logic is duplicated with runDeploy.  Shouild be in buildConfig
	// constructor.
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

	// Build and (optionally) push
	if f, err = client.Build(cmd.Context(), f); err != nil {
		return
	}
	if cfg.Push {
		if f, err = client.Push(cmd.Context(), f); err != nil {
			return
		}
	}

	return f.Write()
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
// current values, as they were passed through via flag defaults, so overwriting
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

// Prompt the user with value of config members, allowing for interactive changes.
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
	// Calculate a better image name message which shows the value of the final
	// image name as it will be calculated if an explicit image name is not used.
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

	// Platform is only supported with the S2I builder at this time
	if c.Platform != "" && c.Builder != builders.S2I {
		err = errors.New("Only S2I builds currently support specifying platform")
		return
	}

	return
}
