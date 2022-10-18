package cmd

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"golang.org/x/term"

	"github.com/AlecAivazis/survey/v2"
	"github.com/AlecAivazis/survey/v2/terminal"
	"github.com/ory/viper"
	"github.com/spf13/cobra"
	"knative.dev/client/pkg/util"

	fn "knative.dev/func"
	"knative.dev/func/builders"
	"knative.dev/func/buildpacks"
	"knative.dev/func/config"
	"knative.dev/func/docker"
	"knative.dev/func/docker/creds"
	"knative.dev/func/k8s"
	"knative.dev/func/s2i"
)

func NewDeployCmd(newClient ClientFactory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "deploy",
		Short: "Deploy a Function",
		Long: `
NAME
	{{.Name}} deploy - Deploy a Function

SYNOPSIS
	{{.Name}} deploy [-R|--remote] [-r|--registry] [-i|--image] [-n|--namespace]
	             [-e|env] [-g|--git-url] [-t|git-branch] [-d|--git-dir]
	             [-b|--build] [--builder] [--builder-image] [-p|--push]
	             [--platform] [-c|--confirm] [-v|--verbose]

DESCRIPTION

	Deploys a function to the currently configured Knative-enabled cluster.

	By default the function in the current working directory is deployed, or at
	the path defined by --path.

	A function which was previously deployed will be updated when re-deployed.

	The function is built into a container for transport to the destination
	cluster by way of a registry.  Therefore --registry must be provided or have
	previously been configured for the function. This registry is also used to
	determine the final built image tag for the function.  This final image name
	can be provided explicitly using --image, in which case it is used in place
	of --registry.

	To run deploy using an interactive mode, use the --confirm (-c) option.
	This mode is useful for the first deployment in particular, since subsdequent
	deployments remember most of the settings provided.

	Building
	  By default the function will be built if it has not yet been built, or if
	  changes are detected in the function's source.  The --build flag can be
	  used to override this behavior and force building either on or off.

	Pushing
	  By default the function's image will be pushed to the configured container
	  registry after being successfully built.  The --push flag can be used
	  to disable pushing.  This could be used, for example, to trigger a redeploy
	  of a service without needing to build, or even have the container available
	  locally with '{{.Name}} deploy --build=false --push==false'.

	Remote
	  Building and pushing (deploying) is by default run on localhost.  This
	  process can also be triggered to run remotely in a Tekton-enabled cluster.
	  The --remote flag indicates that a build and deploy pipeline should be
	  invoked in the remote.  Deploying with '{{.Name}} deploy --remote' will
	  send the function's source code to be built and deployed by the cluster,
	  eliminating the need for a local container engine.  To trigger deployment
	  of a git repository instead of local source, combine with '--git-url':
	  '{{.Name}} deploy --remote --git-url=git.example.com/alice/f.git'

EXAMPLES

	o Deploy the function using interactive prompts. This is useful for the first
	  deployment, since most settings will be remembered for future deployments.
	  $ {{.Name}} deploy -c

	o Deploy the function in the current working directory.
	  The function image will be pushed to "ghcr.io/alice/<Function Name>"
	  $ {{.Name}} deploy --registry ghcr.io/alice

	o Deploy the function in the current working directory, manually specifying
	  the final image name and target cluster namespace.
	  $ {{.Name}} deploy --image ghcr.io/alice/myfunc --namespace myns

	o Deploy the current function's source code by sending it to the cluster to
	  be built and deployed:
	  $ {{.Name}} deploy --remote

	o Trigger a remote deploy, which instructs the cluster to build and deploy
	  the function in the specified git repository.
	  $ {{.Name}} deploy --remote --git-url=https://example.com/alice/myfunc.git

	o Deploy the function, rebuilding the image even if no changes have been
	  detected in the local filesystem (source).
	  $ {{.Name}} deploy --build

	o Deploy without rebuilding, even if changes have been detected in the
	  local filesystem.
	  $ {{.Name}} deploy --build=false

	o Redeploy a function which has already been built and pushed. Works without
	  the use of a local container engine.  For example, if the function was
	  manually deleted from the cluster, it can be quickly redeployed with:
	  $ {{.Name}} deploy --build=false --push=false

`,
		SuggestFor: []string{"delpoy", "deplyo"},
		PreRunE:    bindEnv("confirm", "env", "git-url", "git-branch", "git-dir", "remote", "build", "builder", "builder-image", "image", "registry", "push", "platform", "path", "namespace"),
	}

	// Config
	cfg, err := config.NewDefault()
	if err != nil {
		fmt.Fprintf(cmd.OutOrStdout(), "error loading config at '%v'. %v\n", config.ConfigPath(), err)
	}

	// Flags
	cmd.Flags().BoolP("confirm", "c", cfg.Confirm, "Prompt to confirm all configuration options (Env: $FUNC_CONFIRM)")
	cmd.Flags().StringArrayP("env", "e", []string{}, "Environment variable to set in the form NAME=VALUE. "+
		"You may provide this flag multiple times for setting multiple environment variables. "+
		"To unset, specify the environment variable name followed by a \"-\" (e.g., NAME-).")
	cmd.Flags().StringP("git-url", "g", "", "Repo url to push the code to be built (Env: $FUNC_GIT_URL)")
	cmd.Flags().StringP("git-branch", "t", "", "Git branch to be used for remote builds (Env: $FUNC_GIT_BRANCH)")
	cmd.Flags().StringP("git-dir", "d", "", "Directory in the repo where the function is located (Env: $FUNC_GIT_DIR)")
	cmd.Flags().BoolP("remote", "", false, "Trigger a remote deployment.  Default is to deploy and build from the local system: $FUNC_REMOTE)")

	// Flags shared with Build (specifically related to the build step):
	cmd.Flags().StringP("build", "", "auto", "Build the function. [auto|true|false]. [Env: $FUNC_BUILD]")
	cmd.Flags().Lookup("build").NoOptDefVal = "true" // --build is equivalient to --build=true
	cmd.Flags().StringP("builder", "b", builders.Default, fmt.Sprintf("builder to use when creating the underlying image. Currently supported builders are %s.", KnownBuilders()))
	cmd.Flags().StringP("builder-image", "", "", "The image the specified builder should use; either an as an image name or a mapping. ($FUNC_BUILDER_IMAGE)")
	cmd.Flags().StringP("image", "i", "", "Full image name in the form [registry]/[namespace]/[name]:[tag]@[digest]. This option takes precedence over --registry. Specifying digest is optional, but if it is given, 'build' and 'push' phases are disabled. (Env: $FUNC_IMAGE)")
	cmd.Flags().StringP("registry", "r", GetDefaultRegistry(), "Registry + namespace part of the image to build, ex 'ghcr.io/myuser'.  The full image name is automatically determined. (Env: $FUNC_REGISTRY)")
	cmd.Flags().BoolP("push", "u", true, "Push the function image to registry before deploying (Env: $FUNC_PUSH)")
	cmd.Flags().StringP("platform", "", "", "Target platform to build (e.g. linux/amd64).")
	cmd.Flags().StringP("namespace", "n", "", "Deploy into a specific namespace. (Env: $FUNC_NAMESPACE)")
	setPathFlag(cmd)

	if err := cmd.RegisterFlagCompletionFunc("builder", CompleteBuilderList); err != nil {
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

// runDeploy gathers configuration from environment, flags and the user,
// merges these into the function requested, and triggers either a remote or
// local build-and-deploy.
func runDeploy(cmd *cobra.Command, _ []string, newClient ClientFactory) (err error) {
	// Create a deploy config from environment variables and flags
	config, err := newDeployConfig(cmd)
	if err != nil {
		return
	}

	// Prompt the user to potentially change config interactively.
	config, err = config.Prompt()
	if err != nil {
		if err == terminal.InterruptErr {
			return nil
		}
		return
	}

	// Validate the config
	if err = config.Validate(); err != nil {
		return
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
	if f.Registry == "" || cmd.Flags().Changed("registry") {
		// Sets default AND accepts any user-provided overrides
		f.Registry = config.Registry
	}
	if f.Build.Builder == "" || cmd.Flags().Changed("builder") {
		// Sets default AND accepts any user-provided overrides
		f.Build.Builder = config.Builder
	}
	if config.Image != "" {
		f.Image = config.Image
	}
	if config.ImageDigest != "" {
		fmt.Fprintf(cmd.OutOrStdout(), "Deploying image '%v' with digest '%s'. Build and push are disabled.\n", f.Image, f.ImageDigest)
		f.ImageDigest = config.ImageDigest
	}
	if config.Builder != "" {
		f.Build.Builder = config.Builder
	}
	if config.BuilderImage != "" {
		f.Build.BuilderImages[config.Builder] = config.BuilderImage
	}
	if config.GitURL != "" {
		parts := strings.Split(config.GitURL, "#")
		f.Build.Git.URL = parts[0]
		if len(parts) == 2 { // see Validate() where len enforced to be <= 2
			f.Build.Git.Revision = parts[1]
		}
	}
	if config.GitBranch != "" {
		f.Build.Git.Revision = config.GitBranch
	}
	if config.GitDir != "" {
		f.Build.Git.ContextDir = config.GitDir
	}

	f.Deploy.Namespace = namespace(config, f, cmd.ErrOrStderr())
	if err != nil {
		return
	}

	f.Run.Envs, _, err = mergeEnvs(f.Run.Envs, config.EnvToUpdate, config.EnvToRemove)
	if err != nil {
		return
	}

	// Validate that a builder short-name was obtained, whether that be from
	// the function's prior state, or the value of flags/environment.
	if err = ValidateBuilder(f.Build.Builder); err != nil {
		return
	}

	// Choose a builder based on the value of the --builder flag and a possible
	// override for the build image for that builder to use from the optional
	// builder-image flag.
	var builder fn.Builder
	if f.Build.Builder == builders.Pack {
		builder = buildpacks.NewBuilder(
			buildpacks.WithName(builders.Pack),
			buildpacks.WithVerbose(config.Verbose))
	} else if f.Build.Builder == builders.S2I {
		builder = s2i.NewBuilder(
			s2i.WithName(builders.S2I),
			s2i.WithPlatform(config.Platform),
			s2i.WithVerbose(config.Verbose))
	} else {
		err = fmt.Errorf("builder '%v' is not recognized", f.Build.Builder)
		return
	}

	client, done := newClient(ClientConfig{Namespace: f.Deploy.Namespace, Verbose: config.Verbose},
		fn.WithRegistry(config.Registry),
		fn.WithBuilder(builder))
	defer done()

	// Default Client Registry, Function Registry or explicit Image required
	if client.Registry() == "" && f.Registry == "" && f.Image == "" {
		if interactiveTerminal() {
			// to be consistent, this should throw an error, with the registry
			// prompting code placed within config.Prompt and triggered with --confirm
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

	// Perform the deployment either remote or local.
	if config.Remote {
		// Invoke a remote build/push/deploy pipeline
		// Returned is the function with fields like Registry and Image populated.
		if f, err = client.RunPipeline(cmd.Context(), f); err != nil {
			return
		}
	} else {
		if err = f.Write(); err != nil { // TODO: remove when client API uses 'f'
			return
		}
		if build(config.Build, f, client) { // --build or "auto" with FS changes
			if err = client.Build(cmd.Context(), f.Root); err != nil {
				return
			}
		}
		if f, err = fn.NewFunction(f.Root); err != nil { // TODO: remove when client API uses 'f'
			return
		}
		if config.Push {
			if err = client.Push(cmd.Context(), f.Root); err != nil {
				return
			}
		}
		if err = client.Deploy(cmd.Context(), f.Root); err != nil {
			return
		}
		if f, err = fn.NewFunction(f.Root); err != nil { // TODO: remove when client API uses 'f'
			return
		}
	}

	// mutations persisted on success
	return f.Write()
}

// build returns true if the value of buildStr is a truthy value, or if
// it is the literal "auto" and the function reports as being currently
// unbuilt.  Invalid errors are not reported as this is the purview of
// deployConfig.Validate
func build(buildCfg string, f fn.Function, client *fn.Client) bool {
	if buildCfg == "auto" {
		return !client.Built(f.Root) // first build or modified filesystem
	}
	build, _ := strconv.ParseBool(buildCfg)
	return build
}

func NewRegistryValidator(path string) survey.Validator {
	return func(val interface{}) error {

		// if the value passed in is the zero value of the appropriate type
		if len(val.(string)) == 0 {
			return ErrRegistryRequired
		}

		f, err := fn.NewFunction(path)
		if err != nil {
			return err
		}

		// Set the function's registry to that provided
		f.Registry = val.(string)

		_, err = f.ImageName() //image can be derived without any error
		if err != nil {
			return fmt.Errorf("invalid registry [%q]: %w", val.(string), err)
		}
		return nil
	}
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
	buildConfig

	// Perform build using the settings from the embedded buildConfig struct.
	// Acceptable values are the keyword 'auto', or a truthy value such as
	// 'true', 'false, '1' or '0'.
	Build string

	// Remote indicates the deployment (and possibly build) process are to
	// be triggered in a remote environment rather than run locally.
	Remote bool

	// Namespace override for the deployed function.  If provided, the
	// underlying platform will be instructed to deploy the function to the given
	// namespace (if such a setting is applicable; such as for Kubernetes
	// clusters).  If not provided, the currently configured namespace will be
	// used.  For instance, that which would be used by default by `kubectl`
	// (~/.kube/config) in the case of Kubernetes.
	Namespace string

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

	// ImageDigest is automatically split off an --image tag
	ImageDigest string
}

// newDeployConfig creates a buildConfig populated from command flags and
// environment variables; in that precedence.
func newDeployConfig(cmd *cobra.Command) (deployConfig, error) {
	envToUpdate, envToRemove, err := envFromCmd(cmd)
	if err != nil {
		return deployConfig{}, err
	}

	c := deployConfig{
		buildConfig: newBuildConfig(),
		Build:       viper.GetString("build"),
		Remote:      viper.GetBool("remote"),
		Namespace:   viper.GetString("namespace"),
		EnvToUpdate: envToUpdate,
		EnvToRemove: envToRemove,
		GitURL:      viper.GetString("git-url"),
		GitBranch:   viper.GetString("git-branch"),
		GitDir:      viper.GetString("git-dir"),
		ImageDigest: "", // automatically split off --image if provided below
	}

	if c.Image, c.ImageDigest, err = parseImage(c.Image); err != nil {
		return c, err
	}

	return c, nil
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
			Name: "remote",
			Prompt: &survey.Confirm{
				Message: "Trigger a remote (on-cluster) build?",
				Default: c.Remote,
			},
		},
		{
			Name: "GitURL",
			Prompt: &survey.Input{
				Message: "Git URL",
				Default: c.GitURL,
			},
		},
		{
			Name: "namespace",
			Prompt: &survey.Input{
				Message: "Destination namespace:",
				Default: c.Namespace,
			},
		},
		{
			Name: "path",
			Prompt: &survey.Input{
				Message: "Function source path:",
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

// Validate the config passes an initial consistency check
func (c deployConfig) Validate() (err error) {
	// Bubble validation
	if err = c.buildConfig.Validate(); err != nil {
		return
	}

	// Can not enable build when specifying an --image
	truthy := func(s string) bool {
		v, _ := strconv.ParseBool(s)
		return v
	}
	if c.ImageDigest != "" && truthy(c.Build) {
		return errors.New("building can not be enabled when using an image with digest")
	}

	// Can not push when specifying an --image
	if c.ImageDigest != "" && c.Push {
		return errors.New("pushing is not valid when specifying an image with digest")
	}

	// Git settings are only avaolabe with --remote
	if (c.GitURL != "" || c.GitDir != "" || c.GitBranch != "") && !c.Remote {
		return errors.New("git settings (--git-url --git-dir and --git-branch) are currently only available when triggering remote deployments (--remote)")
	}

	// Git URL can contain at maximum one '#'
	urlParts := strings.Split(c.GitURL, "#")
	if len(urlParts) > 2 {
		return fmt.Errorf("invalid --git-url '%v'", c.GitURL)
	}

	// --build can be "auto"|true|false
	if c.Build != "auto" {
		if _, err := strconv.ParseBool(c.Build); err != nil {
			return fmt.Errorf("unrecognized value for --build '%v'.  accepts 'auto', 'true' or 'false' (or similarly truthy value)", c.Build)
		}
	}

	return
}

func parseImage(v string) (name, digest string, err error) {
	vv := strings.Split(v, "@")
	if len(vv) < 2 {
		name = v
		return
	}
	name = vv[0]
	digest = vv[1]

	if !strings.HasPrefix(digest, "sha256:") {
		return v, "", fmt.Errorf("image '%s' has an invalid prefix syntax for digest (should be 'sha256:')", v)
	}

	if len(digest[7:]) != 64 {
		return v, "", fmt.Errorf("sha256 hash in '%s' has the wrong length (%d), should be 64", digest, len(digest[7:]))
	}

	return
}

// namespace returns the final namespace to use when considering
// both provided values (flag or environment variables), the
// namespace at which the function is currently deployed, and the
// default of the currently active namespace.
// Warnings are printed to stderr when the combination may be
// confusing to the user.
func namespace(cfg deployConfig, f fn.Function, stderr io.Writer) (namespace string) {
	var err error

	if cfg.Namespace != "" {
		namespace = cfg.Namespace // --namespace takes precidence
	} else if f.Deploy.Namespace != "" {
		namespace = f.Deploy.Namespace // value from previous deployment (func.yaml) 2nd priority
	} else {
		// Try to derive a default from the current k8s context, if any.
		if namespace, err = k8s.GetNamespace(""); err != nil {
			fmt.Fprintln(stderr, "Warning: no namespace provided, and none currently active. Continuing to attempt deployment")
		}
	}

	// If the Function is not yet deployed, then immediately return the chosen
	// final namespace
	if f.Deploy.Namespace == "" {
		return
	}

	// Warn if in a different namespace than active
	active, err := k8s.GetNamespace("")
	if err == nil && namespace != active {
		fmt.Fprintf(stderr, "Warning: Function is in namespace '%s', but currently active namespace is '%s'. Continuing with redeployment to '%s'.\n", f.Deploy.Namespace, active, namespace)
	}

	// Warn if potentially creating an orphan
	if f.Deploy.Namespace != "" && namespace != f.Deploy.Namespace {
		fmt.Fprintf(stderr, "Warning: function is in namespace '%s', but requested namespace is '%s'. Continuing with deployment to '%v'.\n", f.Deploy.Namespace, namespace, namespace)
	}

	return
}

var ErrRegistryRequired = errors.New(`A container registry is required.  For example:
--registry docker.io/myusername

To run the command in an interactive mode, use --confirm (-c)`)
