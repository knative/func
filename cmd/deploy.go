package cmd

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"golang.org/x/term"

	"github.com/AlecAivazis/survey/v2"
	"github.com/AlecAivazis/survey/v2/terminal"
	"github.com/ory/viper"
	"github.com/spf13/cobra"
	"knative.dev/client/pkg/util"

	fn "knative.dev/kn-plugin-func"
	"knative.dev/kn-plugin-func/builders"
	"knative.dev/kn-plugin-func/buildpacks"
	"knative.dev/kn-plugin-func/docker"
	"knative.dev/kn-plugin-func/docker/creds"
	"knative.dev/kn-plugin-func/k8s"
	"knative.dev/kn-plugin-func/s2i"
)

func NewDeployCmd(newClient ClientFactory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "deploy",
		Short: "Deploy a function",
		Long: `Deploy a function

Builds a container image for the function and deploys it to the connected Knative enabled cluster.
The function is picked up from the project in the current directory or from the path provided
with --path.
If not already configured, either --registry or --image has to be provided and is then stored
in the configuration file.

If the function is already deployed, it is updated with a new container image
that is pushed to an image registry, and finally the function's Knative service is updated.
`,
		Example: `
# Build and deploy the function from the current directory's project. The image will be
# pushed to "quay.io/myuser/<function name>" and deployed as Knative service with the
# same name as the function to the currently connected cluster.
{{.Name}} deploy --registry quay.io/myuser

# Same as above but using a full image name, that will create a Knative service "myfunc" in
# the namespace "myns"
{{.Name}} deploy --image quay.io/myuser/myfunc -n myns
`,
		SuggestFor: []string{"delpoy", "deplyo"},
		PreRunE:    bindEnv("image", "path", "registry", "confirm", "build", "push", "git-url", "git-branch", "git-dir", "builder", "builder-image", "platform"),
	}

	cmd.Flags().BoolP("confirm", "c", false, "Prompt to confirm all configuration options (Env: $FUNC_CONFIRM)")
	cmd.Flags().StringArrayP("env", "e", []string{}, "Environment variable to set in the form NAME=VALUE. "+
		"You may provide this flag multiple times for setting multiple environment variables. "+
		"To unset, specify the environment variable name followed by a \"-\" (e.g., NAME-).")
	cmd.Flags().StringP("git-url", "g", "", "Repo url to push the code to be built (Env: $FUNC_GIT_URL)")
	cmd.Flags().StringP("git-branch", "t", "", "Git branch to be used for remote builds (Env: $FUNC_GIT_BRANCH)")
	cmd.Flags().StringP("git-dir", "d", "", "Directory in the repo where the function is located (Env: $FUNC_GIT_DIR)")
	cmd.Flags().StringP("build", "", fn.DefaultBuildType, fmt.Sprintf("Build specifies the way the function should be built. Supported types are %s (Env: $FUNC_BUILD)", fn.SupportedBuildTypes(true)))
	// Flags shared with Build specifically related to building:
	cmd.Flags().StringP("builder", "b", builders.Default, fmt.Sprintf("build strategy to use when creating the underlying image. Currently supported build strategies are %s.", KnownBuilders()))
	cmd.Flags().StringP("builder-image", "", "", "builder image, either an as a an image name or a mapping name.\nSpecified value is stored in func.yaml (as 'builder' field) for subsequent builds. ($FUNC_BUILDER_IMAGE)")
	cmd.Flags().StringP("image", "i", "", "Full image name in the form [registry]/[namespace]/[name]:[tag]@[digest]. This option takes precedence over --registry. Specifying digest is optional, but if it is given, 'build' and 'push' phases are disabled. (Env: $FUNC_IMAGE)")
	cmd.Flags().StringP("registry", "r", GetDefaultRegistry(), "Registry + namespace part of the image to build, ex 'quay.io/myuser'.  The full image name is automatically determined based on the local directory name. If not provided the registry will be taken from func.yaml (Env: $FUNC_REGISTRY)")
	cmd.Flags().BoolP("push", "u", true, "Attempt to push the function image to registry before deploying (Env: $FUNC_PUSH)")
	cmd.Flags().StringP("platform", "", "", "Target platform to build (e.g. linux/amd64).")
	setPathFlag(cmd)

	if err := cmd.RegisterFlagCompletionFunc("build", CompleteDeployBuildType); err != nil {
		fmt.Println("internal: error while calling RegisterFlagCompletionFunc: ", err)
	}

	if err := cmd.RegisterFlagCompletionFunc("builder", CompleteBuildersList); err != nil {
		fmt.Println("internal: error while calling RegisterFlagCompletionFunc: ", err)
	}

	if err := cmd.RegisterFlagCompletionFunc("builder-image", CompleteBuilderImageList); err != nil {
		fmt.Println("internal: error while calling RegisterFlagCompletionFunc: ", err)
	}

	cmd.SetHelpFunc(defaultTemplatedHelp)

	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		return runDeploy(cmd, args, newClient)
	}

	return cmd
}

func runDeploy(cmd *cobra.Command, _ []string, newClient ClientFactory) (err error) {
	config, err := newDeployConfig(cmd)
	if err != nil {
		return
	}

	config, err = config.Prompt()
	if err != nil {
		if err == terminal.InterruptErr {
			return nil
		}
		return
	}

	//if --image contains '@', validate image digest and disable build and push if not set, otherwise return an error
	// TODO(lkingland): this logic could be assimilated into the Deploy Config
	// struct and it's constructor.
	imageSplit := strings.Split(config.Image, "@")
	imageDigestProvided := false

	if len(imageSplit) == 2 {
		if config, err = parseImageDigest(imageSplit, config, cmd); err != nil {
			return
		}
		imageDigestProvided = true
	}

	// Load the function, and if it exists (path initialized as a function), merge
	// in any updates from flags/env vars (namespace, explicit image name, envs).
	f, err := fn.NewFunction(config.Path)
	if err != nil {
		return
	}
	if !f.Initialized() {
		return fmt.Errorf("'%v' does not contain an initialized function", config.Path)
	}
	if config.Registry != "" {
		f.Registry = config.Registry
	}
	if config.Image != "" {
		f.Image = config.Image
	}
	if config.Builder != "" {
		f.Builder = config.Builder
	}

	f.Namespace, err = checkNamespaceDeploy(f.Namespace, config.Namespace)
	if err != nil {
		return
	}
	f.Envs, _, err = mergeEnvs(f.Envs, config.EnvToUpdate, config.EnvToRemove)
	if err != nil {
		return
	}

	currentBuildType := config.BuildType

	// if build type has been explicitly set as flag, validate it and override function config
	if config.BuildType != "" {
		err = validateBuildType(config.BuildType)
		if err != nil {
			return err
		}
	} else {
		currentBuildType = f.BuildType
	}

	if imageDigestProvided {
		// TODO(lkingland):  This could instead be part of the config, relying on
		// zero values rather than a flag indicating "Image Digest was Provided"
		f.ImageDigest = imageSplit[1] // save image digest if provided in --image
	}

	// Choose a builder based on the value of the --builder flag
	var builder fn.Builder
	if f.Builder == "" || cmd.Flags().Changed("builder") {
		f.Builder = config.Builder
	} else {
		config.Builder = f.Builder
	}
	if err = ValidateBuilder(config.Builder); err != nil {
		return err
	}
	if config.Builder == builders.Pack {
		if config.Platform != "" {
			err = fmt.Errorf("the --platform flag works only with s2i build")
			return
		}
		builder = buildpacks.NewBuilder(buildpacks.WithVerbose(config.Verbose))
	} else if config.Builder == builders.S2I {
		builder = s2i.NewBuilder(s2i.WithVerbose(config.Verbose), s2i.WithPlatform(config.Platform))
	}

	// Use the user-provided builder image, if supplied
	if config.BuilderImage != "" {
		f.BuilderImages[config.Builder] = config.BuilderImage
	}

	client, done := newClient(ClientConfig{Namespace: f.Namespace, Verbose: config.Verbose},
		fn.WithRegistry(config.Registry),
		fn.WithBuilder(builder))
	defer done()

	// Default Client Registry, Function Registry or explicit Image required
	if client.Registry() == "" && f.Registry == "" && f.Image == "" {
		return ErrRegistryRequired
	}

	// NOTE: curently need to preemptively write out function state until
	// the API is updated to use instances, at which point we will only
	// write on success.
	if err = f.Write(); err != nil {
		return
	}

	switch currentBuildType {
	case fn.BuildTypeLocal, "":
		if config.GitURL != "" || config.GitDir != "" || config.GitBranch != "" {
			return fmt.Errorf("remote git arguments require the --build=git flag")
		}
		if err := client.Build(cmd.Context(), config.Path); err != nil {
			return err
		}
	case fn.BuildTypeGit:
		git := f.Git

		if config.GitURL != "" {
			git.URL = &config.GitURL
			if strings.Contains(config.GitURL, "#") {
				parts := strings.Split(config.GitURL, "#")
				git.URL = &parts[0]
				git.Revision = &parts[1]
			}
		}

		if config.GitBranch != "" {
			git.Revision = &config.GitBranch
		}

		if config.GitDir != "" {
			git.ContextDir = &config.GitDir
		}

		return client.RunPipeline(cmd.Context(), config.Path, git)
	case fn.BuildTypeDisabled:
		// nothing needed to be done for `build=disabled`
	default:
		return ErrInvalidBuildType(fmt.Errorf("unknown build type: %s", currentBuildType))
	}

	if config.Push {
		if err := client.Push(cmd.Context(), config.Path); err != nil {
			return err
		}
	}

	return client.Deploy(cmd.Context(), config.Path)
}

// ValidateBuilder ensures that the given builder is one that the CLI
// knows how to instantiate, returning a builkder.ErrUnknownBuilder otherwise.
func ValidateBuilder(name string) (err error) {
	for _, known := range KnownBuilders() {
		if name == known {
			return
		}
	}
	return builders.ErrUnknownBuilder{Name: name, Known: KnownBuilders()}
}

// KnownBuilders are a typed string slice of builder short names which this
// CLI understands.  Includes a customized String() representation intended
// for use in flags and help text.
func KnownBuilders() builders.Known {
	// The set of builders supported by this CLI will likely always equate to
	// the set of builders enumerated in the builders pacakage.
	// However, future third-party integrations may support less than, or more
	// builders, and certain environmental considerations may alter this list.
	return builders.All()
}

func newPromptForCredentials(in io.Reader, out, errOut io.Writer) func(registry string) (docker.Credentials, error) {
	firstTime := true
	return func(registry string) (docker.Credentials, error) {
		var result docker.Credentials

		if firstTime {
			firstTime = false
			fmt.Fprintf(out, "Please provide credentials for image registry (%s).\n", registry)
		} else {
			fmt.Fprintln(out, "Incorrect credentials, please try again.")
		}

		var qs = []*survey.Question{
			{
				Name: "username",
				Prompt: &survey.Input{
					Message: "Username:",
				},
				Validate: survey.Required,
			},
			{
				Name: "password",
				Prompt: &survey.Password{
					Message: "Password:",
				},
				Validate: survey.Required,
			},
		}

		var (
			fr terminal.FileReader
			ok bool
		)

		isTerm := false
		if fr, ok = in.(terminal.FileReader); ok {
			isTerm = term.IsTerminal(int(fr.Fd()))
		}

		if isTerm {
			err := survey.Ask(qs, &result, survey.WithStdio(fr, out.(terminal.FileWriter), errOut))
			if err != nil {
				return docker.Credentials{}, err
			}
		} else {
			reader := bufio.NewReader(in)

			fmt.Fprintf(out, "Username: ")
			u, err := reader.ReadString('\n')
			if err != nil {
				return docker.Credentials{}, err
			}
			u = strings.Trim(u, "\r\n")

			fmt.Fprintf(out, "Password: ")
			p, err := reader.ReadString('\n')
			if err != nil {
				return docker.Credentials{}, err
			}
			p = strings.Trim(p, "\r\n")

			result = docker.Credentials{Username: u, Password: p}
		}

		return result, nil
	}
}

func newPromptForCredentialStore() creds.ChooseCredentialHelperCallback {
	return func(availableHelpers []string) (string, error) {
		if len(availableHelpers) < 1 {
			fmt.Fprintf(os.Stderr, `Credentials will not be saved.
If you would like to save your credentials in the future,
you can install docker credential helper https://github.com/docker/docker-credential-helpers.
`)
			return "", nil
		}

		isTerm := term.IsTerminal(int(os.Stdin.Fd()))

		var resp string

		if isTerm {
			err := survey.AskOne(&survey.Select{
				Message: "Choose credentials helper",
				Options: append(availableHelpers, "None"),
			}, &resp, survey.WithValidator(survey.Required))
			if err != nil {
				return "", err
			}
			if resp == "None" {
				fmt.Fprintf(os.Stderr, "No helper selected. Credentials will not be saved.\n")
				return "", nil
			}
		} else {
			fmt.Fprintf(os.Stderr, "Available credential helpers:\n")
			for _, helper := range availableHelpers {
				fmt.Fprintf(os.Stderr, "%s\n", helper)
			}
			fmt.Fprintf(os.Stderr, "Choose credentials helper: ")

			reader := bufio.NewReader(os.Stdin)

			var err error
			resp, err = reader.ReadString('\n')
			if err != nil {
				return "", err
			}
			resp = strings.Trim(resp, "\r\n")
			if resp == "" {
				fmt.Fprintf(os.Stderr, "No helper selected. Credentials will not be saved.\n")
			}
		}

		return resp, nil
	}
}

type deployConfig struct {
	// Image name in full, including registry, repo and tag (overrides
	// image name derivation based on registry and function name)
	Image string

	// Namespace override for the deployed function.  If provided, the
	// underlying platform will be instructed to deploy the function to the given
	// namespace (if such a setting is applicable; such as for Kubernetes
	// clusters).  If not provided, the currently configured namespace will be
	// used.  For instance, that which would be used by default by `kubectl`
	// (~/.kube/config) in the case of Kubernetes.
	Namespace string

	// Path of the function implementation on local disk. Defaults to current
	// working directory of the process.
	Path string

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

	// Build the associated function before deploying.
	BuildType string

	// Push function image to the registry before deploying.
	Push bool

	// Registry at which interstitial build artifacts should be kept.
	// This setting is ignored if Image is specified, which includes the full
	Registry string

	// Envs passed via cmd to be added/updated
	EnvToUpdate *util.OrderedMap

	// Envs passed via cmd to removed
	EnvToRemove []string

	// Git repo url for remote builds
	GitURL string

	// Git branch for remote builds
	GitBranch string

	// Directory in the git repo where the function is located
	GitDir string
}

// newDeployConfig creates a buildConfig populated from command flags and
// environment variables; in that precedence.
func newDeployConfig(cmd *cobra.Command) (deployConfig, error) {
	envToUpdate, envToRemove, err := envFromCmd(cmd)
	if err != nil {
		return deployConfig{}, err
	}

	// We need to know whether the `build`` flag had been explicitly set,
	// to distinguish between unset and default value.
	var buildType string
	if viper.IsSet("build") {
		buildType = viper.GetString("build")
	}

	return deployConfig{
		Image:        viper.GetString("image"),
		Namespace:    viper.GetString("namespace"),
		Path:         getPathFlag(),
		Verbose:      viper.GetBool("verbose"), // defined on root
		Confirm:      viper.GetBool("confirm"),
		Builder:      viper.GetString("builder"),
		BuilderImage: viper.GetString("builder-image"),
		Platform:     viper.GetString("platform"),
		BuildType:    buildType,
		Push:         viper.GetBool("push"),
		Registry:     viper.GetString("registry"),
		EnvToUpdate:  envToUpdate,
		EnvToRemove:  envToRemove,
		GitURL:       viper.GetString("git-url"),
		GitBranch:    viper.GetString("git-branch"),
		GitDir:       viper.GetString("git-dir"),
	}, nil
}

// Prompt the user with value of config members, allowing for interaractive changes.
// Skipped if not in an interactive terminal (non-TTY), or if --yes (agree to
// all prompts) was explicitly set.
func (c deployConfig) Prompt() (deployConfig, error) {
	if !interactiveTerminal() || !c.Confirm {
		return c, nil
	}

	var qs = []*survey.Question{
		{
			Name: "path",
			Prompt: &survey.Input{
				Message: "Project path:",
				Default: c.Path,
			},
		},
		{
			Name: "registry",
			Prompt: &survey.Input{
				Message: "Registry for function images:",
				Default: c.Registry,
			},
		},
	}
	if err := survey.Ask(qs, &c); err != nil {
		return c, err
	}

	// calculate imageName with potentially new registry/path
	imageName := deriveImage(c.Image, c.Registry, c.Path)

	qs = []*survey.Question{
		{
			Name: "image",
			Prompt: &survey.Input{
				Message: "Full image name (e.g. quay.io/boson/node-sample):",
				Default: imageName,
			},
		},
		{
			Name: "namespace",
			Prompt: &survey.Input{
				Message: "Namespace into which the function is (re)deployed",
				Default: c.Namespace,
			},
		},
	}
	err := survey.Ask(qs, &c)

	return c, err
}

// ErrInvalidBuildType indicates that the passed build type was invalid.
type ErrInvalidBuildType error

// ValidateBuildType validatest that the input Build type is valid for deploy command
func validateBuildType(buildType string) error {
	if errs := fn.ValidateBuildType(buildType, false, true); len(errs) > 0 {
		return ErrInvalidBuildType(errors.New(strings.Join(errs, "")))
	}
	return nil
}

func parseImageDigest(imageSplit []string, config deployConfig, cmd *cobra.Command) (deployConfig, error) {

	if !strings.HasPrefix(imageSplit[1], "sha256:") {
		return config, fmt.Errorf("value '%s' in --image has invalid prefix syntax for digest (should be 'sha256:')", config.Image)
	}

	if len(imageSplit[1][7:]) != 64 {
		return config, fmt.Errorf("sha256 hash in '%s' from --image has the wrong length (%d), should be 64", imageSplit[1], len(imageSplit[1][7:]))
	}

	// if --build was set but not as 'disabled', return an error
	if cmd.Flags().Changed("build") && config.BuildType != "disabled" {
		return config, fmt.Errorf("the --build flag '%s' is not valid when using --image with digest", config.BuildType)
	}

	// if the --push flag was set by a user to 'true', return an error
	if cmd.Flags().Changed("push") && config.Push {
		return config, fmt.Errorf("the --push flag '%v' is not valid when using --image with digest", config.Push)
	}

	fmt.Printf("Deploying existing image with digest %s. Build and push are disabled.\n", imageSplit[1])

	config.BuildType = "disabled"
	config.Push = false
	config.Image = imageSplit[0]

	return config, nil
}

// checkNamespaceDeploy checks current namespace against func.yaml and warns if its different
// or sets namespace to be written in func.yaml if its the first deployment
func checkNamespaceDeploy(funcNamespace string, confNamespace string) (string, error) {
	var namespace string
	var err error

	// Choose target namespace
	if confNamespace != "" {
		namespace = confNamespace // --namespace takes precidence
	} else if funcNamespace != "" {
		namespace = funcNamespace // value from previous deployment (func.yaml) 2nd priority
	} else {
		// Try to derive a default from the current k8s context, if any.
		namespace, err = k8s.GetNamespace("")
		if err != nil {
			return "", nil // configured k8s environment not required
		}
	}

	currNamespace, err := k8s.GetNamespace("")
	if err == nil && namespace != currNamespace {
		fmt.Fprintf(os.Stderr, "Warning: Current namespace '%s' does not match function which is deployed at '%s' namespace\n", currNamespace, funcNamespace)
	}

	if funcNamespace != "" && namespace != funcNamespace {
		fmt.Fprintf(os.Stderr, "Warning: New namespace '%s' does not match current namespace '%s'\n", namespace, funcNamespace)
	}

	return namespace, nil
}

var ErrRegistryRequired = errors.New(`A container registry is required.  For example:
--registry docker.io/myusername

For more advanced usage, it is also possible to specify the exact image to use. For example:

--image docker.io/myusername/myfunc:latest

To run the command in an interactive mode, use --confirm (-c)`)
