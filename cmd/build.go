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
		Short: "Build a Function",
		Long: `
NAME
	{{.Name}} build - Build a Function

SYNOPSIS
	{{.Name}} build [-r|--registry] [--builder] [--builder-image] [--push]
	             [--palatform] [-p|--path] [-c|--confirm] [-v|--verbose]

DESCRIPTION

	Builds a function's container image and optionally pushes it to the
	configured container registry.

	By default building is handled automatically when deploying (see the deploy
	subcommand). However, sometimes it is useful to build a function container
	outside of this normal deployment process, for example for testing or during
	composition when integrationg with other systems. Additionally, the container
	can be pushed to the configured registry using the --push option.

	When building a function for the first time, either a registry or explicit
	image name is required.  Subsequent builds will reuse these option values.

EXAMPLES

	o Build a function container using the given registry.
	  The full image name will be calculated using the registry and function name.
	  $ {{.Name}} build --registry registry.example.com/alice

	o Build a function container using an explicit image name, ignoring registry
	  and function name.
		$ {{.Name}} build --image registry.example.com/alice/f:latest

	o Rebuild a function using prior values to determine container name.
	  $ {{.Name}} build

	o Build a function specifying the Source-to-Image (S2I) builder
	  $ {{.Name}} build --builder=s2i

	o Build a function specifying the Pack builder with a custom Buildpack
	  builder image.
		$ {{.Name}} build --builder=pack --builder-image=cnbs/sample-builder:bionic

`,
		SuggestFor: []string{"biuld", "buidl", "built"},
		PreRunE:    bindEnv("image", "path", "builder", "registry", "confirm", "push", "builder-image", "platform"),
	}

	// Config
	cfg, err := config.NewDefault()
	if err != nil {
		fmt.Fprintf(cmd.OutOrStdout(), "error loading config at '%v'. %v\n", config.File(), err)
	}

	// Function Context
	// Load the value of the builder from the function at the effective path
	// if it exists.
	// This value takes precedence over the global config value, which encapsulates
	// both the static default (builders.default) and any extant user setting in
	// their global config file.
	// The defaulting of path to cwd() can be removed when the open PR #
	// is merged which updates the system to treat an empty path as indicating
	// CWD by default.
	builder := cfg.Builder
	path := effectivePath()
	if path == "" {
		path = cwd()
	}
	if f, err := fn.NewFunction(path); err == nil && f.Build.Builder != "" {
		// no errors loading the function at path, and it has a builder specified:
		// The "function with context" takes precedence determining flag defaults
		builder = f.Build.Builder
	}

	cmd.Flags().StringP("builder", "b", builder, fmt.Sprintf("build strategy to use when creating the underlying image. Currently supported build strategies are %s.", KnownBuilders()))
	cmd.Flags().StringP("builder-image", "", "", "builder image, either an as a an image name or a mapping name.\nSpecified value is stored in func.yaml (as 'builder' field) for subsequent builds. ($FUNC_BUILDER_IMAGE)")
	cmd.Flags().BoolP("confirm", "c", cfg.Confirm, "Prompt to confirm all configuration options (Env: $FUNC_CONFIRM)")
	cmd.Flags().StringP("image", "i", "", "Full image name in the form [registry]/[namespace]/[name]:[tag] (optional). This option takes precedence over --registry (Env: $FUNC_IMAGE)")
	cmd.Flags().StringP("registry", "r", "", "Registry + namespace part of the image to build, ex 'quay.io/myuser'.  The full image name is automatically determined (Env: $FUNC_REGISTRY)")
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
	if err = config.CreatePaths(); err != nil {
		return // see docker/creds potential mutation of auth.json
	}

	// Populate a command config from environment variables, flags and potentially
	// interactive user prompts if in confirm mode.
	cfg, err := newBuildConfig().Prompt()
	if err != nil {
		return
	}

	// Validate the config
	if err = cfg.Validate(); err != nil {
		return
	}

	// Load the Function at path, and if it is initialized, update it with
	// pertinent values from the config.
	//
	// NOTE: the checks for .Changed and altered conditionals for defaults will
	// be removed when Global Config is integrated, because the config object
	// will at that time contain the final value for the attribute, taking into
	// account whether or not the value was altered via flags or env variables.
	// This condition is also only necessary for config members whose default
	// value deviates from the zero value.
	f, err := fn.NewFunction(cfg.Path)
	if err != nil {
		return
	}
	if !f.Initialized() {
		return fmt.Errorf("'%v' does not contain an initialized function", cfg.Path)
	}
	if f.Registry == "" || cmd.Flags().Changed("registry") {
		// Sets default AND accepts any user-provided overrides
		f.Registry = cfg.Registry
	}
	if cfg.Image != "" {
		f.Image = cfg.Image
	}
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
	if f.Build.Builder == "" || cmd.Flags().Changed("builder") {
		// Sets default AND accepts any user-provided overrides
		f.Build.Builder = cfg.Builder
	}
	if cfg.Builder != "" {
		f.Build.Builder = cfg.Builder
	}
	if cfg.BuilderImage != "" {
		f.Build.BuilderImages[cfg.Builder] = cfg.BuilderImage
	}

	// Validate that a builder short-name was obtained, whether that be from
	// the function's prior state, or the value of flags/environment.
	if err = ValidateBuilder(f.Build.Builder); err != nil {
		return
	}

	// Choose a builder based on the value of the --builder flag
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
		err = fmt.Errorf("builder '%v' is not recognized", f.Build.Builder)
		return
	}

	client, done := newClient(ClientConfig{Verbose: cfg.Verbose},
		fn.WithRegistry(cfg.Registry),
		fn.WithBuilder(builder))
	defer done()

	// This preemptive write call will be unnecessary when the API is updated
	// to use Function instances rather than file paths. For now it must write
	// even if the command fails later.  Not ideal.
	if err = f.Write(); err != nil {
		return
	}

	if err = client.Build(cmd.Context(), cfg.Path); err != nil {
		return
	}
	if cfg.Push {
		err = client.Push(cmd.Context(), cfg.Path)
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
		Path:         viper.GetString("path"),
		Registry:     registry(),
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

	if c.Platform != "" && c.Builder != builders.S2I {
		err = errors.New("Only S2I builds currently support specifying platform")
		return
	}

	return
}
