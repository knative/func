package cmd

import (
	"errors"
	"fmt"
	"strings"

	"github.com/AlecAivazis/survey/v2"
	"github.com/ory/viper"
	"github.com/spf13/cobra"

	"knative.dev/func/pkg/builders"
	pack "knative.dev/func/pkg/builders/buildpacks"
	"knative.dev/func/pkg/builders/s2i"
	"knative.dev/func/pkg/config"
	fn "knative.dev/func/pkg/functions"
	"knative.dev/func/pkg/oci"
)

func NewBuildCmd(newClient ClientFactory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "build",
		Short: "Build a function container",
		Long: `
NAME
	{{rootCmdUse}} build - Build a function container locally without deploying

SYNOPSIS
	{{rootCmdUse}} build [-r|--registry] [--builder] [--builder-image] [--push]
	             [--platform] [-p|--path] [-c|--confirm] [-v|--verbose]
               [--build-timestamp] [--registry-insecure]

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
		PreRunE:    bindEnv("image", "path", "builder", "registry", "confirm", "push", "builder-image", "platform", "verbose", "build-timestamp", "registry-insecure"),
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
	cmd.Flags().Bool("registry-insecure", cfg.RegistryInsecure, "Disable HTTPS when communicating to the registry ($FUNC_REGISTRY_INSECURE)")

	// Function-Context Flags:
	// Options whose value is available on the function with context only
	// (persisted but not globally configurable)
	builderImage := f.Build.BuilderImages[f.Build.Builder]
	cmd.Flags().StringP("builder-image", "", builderImage,
		"Specify a custom builder image for use by the builder other than its default. ($FUNC_BUILDER_IMAGE)")
	cmd.Flags().StringP("image", "i", f.Image,
		"Full image name in the form [registry]/[namespace]/[name]:[tag] (optional). This option takes precedence over --registry ($FUNC_IMAGE)")
	cmd.Flags().BoolP("build-timestamp", "", false, "Use the actual time as the created time for the docker image. This is only useful for buildpacks builder.")

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
	if !f.Initialized() {
		return fn.NewErrNotInitialized(f.Root)
	}
	f = cfg.Configure(f) // Updates f at path to include build request values

	// Client
	clientOptions, err := cfg.clientOptions()
	if err != nil {
		return
	}
	client, done := newClient(ClientConfig{Verbose: cfg.Verbose}, clientOptions...)
	defer done()

	// Build
	buildOptions, err := cfg.buildOptions()
	if err != nil {
		return
	}
	if f, err = client.Build(cmd.Context(), f, buildOptions...); err != nil {
		return
	}
	if cfg.Push {
		if f, err = client.Push(cmd.Context(), f); err != nil {
			return
		}
	}
	if err = f.Write(); err != nil {
		return
	}
	// Stamp is a performance optimization: treat the function as being built
	// (cached) unless the fs changes.
	return f.Stamp()
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

	// Build with the current timestamp as the created time for docker image.
	// This is only useful for buildpacks builder.
	WithTimestamp bool
}

// newBuildConfig gathers options into a single build request.
func newBuildConfig() buildConfig {
	return buildConfig{
		Global: config.Global{
			Builder:          viper.GetString("builder"),
			Confirm:          viper.GetBool("confirm"),
			Registry:         registry(), // deferred defaulting
			Verbose:          viper.GetBool("verbose"),
			RegistryInsecure: viper.GetBool("registry-insecure"),
		},
		BuilderImage:  viper.GetString("builder-image"),
		Image:         viper.GetString("image"),
		Path:          viper.GetString("path"),
		Platform:      viper.GetString("platform"),
		Push:          viper.GetBool("push"),
		WithTimestamp: viper.GetBool("build-timestamp"),
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

	qs := []*survey.Question{
		{
			Name: "image",
			Prompt: &survey.Input{
				Message: "Optionally specify an exact image name to use (e.g. quay.io/boson/node-sample:latest)",
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

// clientOptions returns options suitable for instantiating a client based on
// the current state of the build config object.
// This will be unnecessary and refactored away when the host-based OCI
// builder and pusher are the default implementations and the Pack and S2I
// constructors simplified.
//
// TODO: Platform is currently only used by the S2I builder.  This should be
// a multi-valued argument which passes through to the "host" builder (which
// supports multi-arch/platform images), and throw an error if either trying
// to specify a platform for buildpacks, or trying to specify more than one
// for S2I.
//
// TODO: As a further optimization, it might be ideal to only build the
// image necessary for the target cluster, since the end product of  a function
// deployment is not the contiainer, but rather the running service.
func (c buildConfig) clientOptions() ([]fn.Option, error) {
	o := []fn.Option{fn.WithRegistry(c.Registry)}
	if c.Builder == builders.Host {
		o = append(o,
			fn.WithBuilder(oci.NewBuilder(builders.Host, c.Verbose)),
			fn.WithPusher(oci.NewPusher(c.RegistryInsecure, false, c.Verbose)))
	} else if c.Builder == builders.Pack {
		o = append(o,
			fn.WithBuilder(pack.NewBuilder(
				pack.WithName(builders.Pack),
				pack.WithTimestamp(c.WithTimestamp),
				pack.WithVerbose(c.Verbose))))
	} else if c.Builder == builders.S2I {
		o = append(o,
			fn.WithBuilder(s2i.NewBuilder(
				s2i.WithName(builders.S2I),
				s2i.WithVerbose(c.Verbose))))
	} else {
		return o, builders.ErrUnknownBuilder{Name: c.Builder, Known: KnownBuilders()}
	}
	return o, nil
}

// buildOptions returns options for use with the client.Build request
func (c buildConfig) buildOptions() (oo []fn.BuildOption, err error) {
	oo = []fn.BuildOption{}

	// Platforms
	//
	// TODO: upgrade --platform to a multi-value field.  The individual builder
	// implementations are responsible for bubbling an error if they do
	// not support this.  Pack  supports none, S2I supports one, host builder
	// supports multi.
	if c.Platform != "" {
		parts := strings.Split(c.Platform, "/")
		if len(parts) != 2 {
			return oo, fmt.Errorf("the value for --patform must be in the form [OS]/[Architecture].  eg \"linux/amd64\"")
		}
		oo = append(oo, fn.BuildWithPlatforms([]fn.Platform{{OS: parts[0], Architecture: parts[1]}}))
	}

	return
}
