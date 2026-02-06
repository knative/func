package cmd

import (
	"errors"
	"fmt"
	"strings"

	"github.com/AlecAivazis/survey/v2"
	"github.com/ory/viper"
	"github.com/spf13/cobra"
	"knative.dev/func/pkg/builders"
	"knative.dev/func/pkg/buildpacks"
	"knative.dev/func/pkg/config"
	"knative.dev/func/pkg/docker"
	fn "knative.dev/func/pkg/functions"
	"knative.dev/func/pkg/oci"
	"knative.dev/func/pkg/s2i"
)

func NewBuildCmd(newClient ClientFactory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "build",
		Short: "Build a function container",
		Long: `
NAME
	{{rootCmdUse}} build - Build a function container locally without deploying

SYNOPSIS
	{{rootCmdUse}} build [-r|--registry] [--builder] [--builder-image]
		         [--push] [--username] [--password] [--token]
	             [--platform] [-p|--path] [-c|--confirm] [-v|--verbose]
		         [--build-timestamp] [--registry-insecure] [--registry-authfile]

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
		PreRunE: bindEnv("image", "path", "builder", "registry", "confirm",
			"push", "builder-image", "base-image", "platform", "verbose",
			"build-timestamp", "registry-insecure", "registry-authfile", "username", "password", "token"),
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
	cmd.Flags().Bool("registry-insecure", cfg.RegistryInsecure, "Skip TLS certificate verification when communicating in HTTPS with the registry ($FUNC_REGISTRY_INSECURE)")
	cmd.Flags().String("registry-authfile", "", "Path to a authentication file containing registry credentials ($FUNC_REGISTRY_AUTHFILE)")

	// Function-Context Flags:
	// Options whose value is available on the function with context only
	// (persisted but not globally configurable)
	builderImage := f.Build.BuilderImages[f.Build.Builder]
	cmd.Flags().StringP("builder-image", "", builderImage,
		"Specify a custom builder image for use by the builder other than its default. ($FUNC_BUILDER_IMAGE)")
	cmd.Flags().StringP("base-image", "", f.Build.BaseImage,
		"Override the base image for your function (host builder only)")
	cmd.Flags().StringP("image", "i", f.Image,
		"Full image name in the form [registry]/[namespace]/[name]:[tag] (optional). This option takes precedence over --registry ($FUNC_IMAGE)")

	// Static Flags:
	// Options which are either empty or have static defaults only (not
	// globally configurable nor persisted with the function)
	cmd.Flags().BoolP("push", "u", false,
		"Attempt to push the function image to the configured registry after being successfully built")
	cmd.Flags().StringP("platform", "", "",
		"Optionally specify a target platform, for example \"linux/amd64\" when using the s2i build strategy")
	cmd.Flags().StringP("username", "", "",
		"Username to use when pushing to the registry. ($FUNC_USERNAME)")
	cmd.Flags().StringP("password", "", "",
		"Password to use when pushing to the registry. ($FUNC_PASSWORD)")
	cmd.Flags().StringP("token", "", "",
		"Token to use when pushing to the registry. ($FUNC_TOKEN)")
	cmd.Flags().BoolP("build-timestamp", "", false, "Use the actual time as the created time for the docker image. This is only useful for buildpacks builder.")

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
	if cfg, err = newBuildConfig().Prompt(); err != nil {
		return wrapPromptError(err, "build")
	}
	if err = cfg.Validate(cmd); err != nil { // Perform any pre-validation
		return wrapValidateError(err, "build")
	}
	if f, err = fn.NewFunction(cfg.Path); err != nil { // Read in the Function
		return
	}
	if !f.Initialized() {
		return NewErrNotInitializedFromPath(f.Root, "build")
	}
	f = cfg.Configure(f) // Returns an f updated with values from the config (flags, envs, etc)

	// Client
	clientOptions, err := cfg.clientOptions()
	if err != nil {
		return
	}
	client, done := newClient(ClientConfig{Verbose: cfg.Verbose}, clientOptions...)
	defer done()

	// Build
	buildOptions, err := cfg.buildOptions() // build-specific options from the finalized cfg
	if err != nil {
		return
	}

	if err = client.Scaffold(cmd.Context(), f, ""); err != nil {
		return
	}

	if f, err = client.Build(cmd.Context(), f, buildOptions...); err != nil {
		return
	}
	if cfg.Push {
		if f, _, err = client.Push(cmd.Context(), f); err != nil {
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

	// BaseImage is an image to build a function upon (host builder only)
	// TODO: gauron99 -- make option to add a path to dockerfile ?
	BaseImage string

	// Path of the function implementation on local disk. Defaults to current
	// working directory of the process.
	Path string

	// Platform ofr resultant image (s2i builder only)
	Platform string

	// Push the resulting image to the registry after building.
	Push bool

	// Username when specifying optional basic auth.
	Username string

	// Password when using optional basic auth.  Should be provided along
	// with Username.
	Password string

	// Token when performing basic auth using a bearer token.  Should be
	// exclusive with Username and Password.
	Token string

	// Build with the current timestamp as the created time for docker image.
	// This is only useful for buildpacks builder.
	WithTimestamp bool

	// RegistryAuthfile is the path to a docker-config file containing registry credentials.
	RegistryAuthfile string
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
		BuilderImage:     viper.GetString("builder-image"),
		BaseImage:        viper.GetString("base-image"),
		Image:            viper.GetString("image"),
		Path:             viper.GetString("path"),
		Platform:         viper.GetString("platform"),
		Push:             viper.GetBool("push"),
		Username:         viper.GetString("username"),
		Password:         viper.GetString("password"),
		Token:            viper.GetString("token"),
		WithTimestamp:    viper.GetBool("build-timestamp"),
		RegistryAuthfile: viper.GetString("registry-authfile"),
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
	f.Build.BaseImage = c.BaseImage
	// Path, Platform and Push are not part of a function's state.
	return f
}

// Prompt the user with value of config members, allowing for interactive changes.
// Skipped if not in an interactive terminal (non-TTY), or if --confirm false (agree to
// all prompts) was set (default).
func (c buildConfig) Prompt() (buildConfig, error) {
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

	// Check if function exists first
	if !f.Initialized() {
		// Return a specific error for uninitialized function
		return c, fn.NewErrNotInitialized(f.Root)
	}

	if !interactiveTerminal() {
		return c, nil
	}

	// If function IS initialized AND registry/image is missing
	if f.Registry == "" && c.Registry == "" && c.Image == "" {
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

	if !c.Confirm {
		return c, nil
	}

	// Remainder of prompts are optional and only shown if in --confirm mode
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
		{
			Name: "builder",
			Prompt: &survey.Select{
				Message: "Select builder:",
				Options: []string{"pack", "s2i", "host"},
				Default: c.Builder,
			},
		},
		{
			Name: "push",
			Prompt: &survey.Confirm{
				Message: "Push image to your registry after build?",
				Default: c.Push,
			},
		},
	}

	err = survey.Ask(qs, &c)
	if err != nil {
		return c, err
	}

	if c.Builder == "host" {
		hostQs := []*survey.Question{
			{
				Name: "baseImage",
				Prompt: &survey.Input{
					Message: "Optional base image for your function (empty for default):",
					Default: c.BaseImage,
				},
			},
		}
		err = survey.Ask(hostQs, &c)
		if err != nil {
			return c, err
		}
	}

	return c, nil
}

// Validate the config passes an initial consistency check
func (c buildConfig) Validate(cmd *cobra.Command) (err error) {
	// Builder value must refer to a known builder short name
	if err = ValidateBuilder(c.Builder); err != nil {
		return
	}

	// Check for conflicting --image and --registry flags
	// Simply reject if both are explicitly provided - user should choose one or the other
	if cmd.Flags().Changed("image") && cmd.Flags().Changed("registry") {
		return fn.ErrConflictingImageAndRegistry
	}

	// Platform is only supported with the S2I builder at this time
	if c.Platform != "" && c.Builder != builders.S2I {
		err = fn.ErrPlatformNotSupported
		return
	}

	// BaseImage is only supported with the host builder
	if c.BaseImage != "" && c.Builder != "host" {
		err = errors.New("only host builds support specifying the base image")
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
// image necessary for the target cluster, since the end product of a function
// deployment is not the container, but rather the running service.
func (c buildConfig) clientOptions() ([]fn.Option, error) {
	o := []fn.Option{fn.WithRegistry(c.Registry)}

	t := newTransport(c.RegistryInsecure)
	creds := newCredentialsProvider(config.Dir(), t, c.RegistryAuthfile)

	switch c.Builder {
	case builders.Host:
		o = append(o,
			fn.WithScaffolder(oci.NewScaffolder(c.Verbose)),
			fn.WithBuilder(oci.NewBuilder(builders.Host, c.Verbose)),
			fn.WithPusher(oci.NewPusher(c.RegistryInsecure, false, c.Verbose,
				oci.WithTransport(newTransport(c.RegistryInsecure)),
				oci.WithCredentialsProvider(creds),
				oci.WithVerbose(c.Verbose))),
		)
	case builders.Pack:
		o = append(o,
			fn.WithScaffolder(buildpacks.NewScaffolder(c.Verbose)),
			fn.WithBuilder(buildpacks.NewBuilder(
				buildpacks.WithName(builders.Pack),
				buildpacks.WithTimestamp(c.WithTimestamp),
				buildpacks.WithVerbose(c.Verbose))),
			fn.WithPusher(docker.NewPusher(
				docker.WithCredentialsProvider(creds),
				docker.WithTransport(t),
				docker.WithVerbose(c.Verbose))))
	case builders.S2I:
		o = append(o,
			fn.WithScaffolder(s2i.NewScaffolder(c.Verbose)),
			fn.WithBuilder(s2i.NewBuilder(
				s2i.WithName(builders.S2I),
				s2i.WithVerbose(c.Verbose))),
			fn.WithPusher(docker.NewPusher(
				docker.WithCredentialsProvider(creds),
				docker.WithTransport(t),
				docker.WithVerbose(c.Verbose))))
	default:
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
