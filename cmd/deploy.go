package cmd

import (
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/AlecAivazis/survey/v2"
	"github.com/ory/viper"
	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/api/resource"
	"knative.dev/client-pkg/pkg/util"
	"knative.dev/func/pkg/builders"
	"knative.dev/func/pkg/builders/buildpacks"
	"knative.dev/func/pkg/builders/s2i"
	"knative.dev/func/pkg/config"
	fn "knative.dev/func/pkg/functions"
	"knative.dev/func/pkg/k8s"
)

func NewDeployCmd(newClient ClientFactory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "deploy",
		Short: "Deploy a function",
		Long: `
NAME
	{{rootCmdUse}} deploy - Deploy a function

SYNOPSIS
	{{rootCmdUse}} deploy [-R|--remote] [-r|--registry] [-i|--image] [-n|--namespace]
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
	  locally with '{{rootCmdUse}} deploy --build=false --push==false'.

	Remote
	  Building and pushing (deploying) is by default run on localhost.  This
	  process can also be triggered to run remotely in a Tekton-enabled cluster.
	  The --remote flag indicates that a build and deploy pipeline should be
	  invoked in the remote.  Deploying with '{{rootCmdUse}} deploy --remote' will
	  send the function's source code to be built and deployed by the cluster,
	  eliminating the need for a local container engine.  To trigger deployment
	  of a git repository instead of local source, combine with '--git-url':
	  '{{rootCmdUse}} deploy --remote --git-url=git.example.com/alice/f.git'

EXAMPLES

	o Deploy the function
	  $ {{rootCmdUse}} deploy

	o Deploy the function using interactive prompts. This is useful for the first
	  deployment, since most settings will be remembered for future deployments.
	  $ {{rootCmdUse}} deploy -c

	o Deploy the function in the current working directory.
	  The function image will be pushed to "ghcr.io/alice/<Function Name>"
	  $ {{rootCmdUse}} deploy --registry ghcr.io/alice

	o Deploy the function in the current working directory, manually specifying
	  the final image name and target cluster namespace.
	  $ {{rootCmdUse}} deploy --image ghcr.io/alice/myfunc --namespace myns

	o Deploy the current function's source code by sending it to the cluster to
	  be built and deployed:
	  $ {{rootCmdUse}} deploy --remote

	o Trigger a remote deploy, which instructs the cluster to build and deploy
	  the function in the specified git repository.
	  $ {{rootCmdUse}} deploy --remote --git-url=https://example.com/alice/myfunc.git

	o Deploy the function, rebuilding the image even if no changes have been
	  detected in the local filesystem (source).
	  $ {{rootCmdUse}} deploy --build

	o Deploy without rebuilding, even if changes have been detected in the
	  local filesystem.
	  $ {{rootCmdUse}} deploy --build=false

	o Redeploy a function which has already been built and pushed. Works without
	  the use of a local container engine.  For example, if the function was
	  manually deleted from the cluster, it can be quickly redeployed with:
	  $ {{rootCmdUse}} deploy --build=false --push=false

`,
		SuggestFor: []string{"delpoy", "deplyo"},
		PreRunE:    bindEnv("confirm", "env", "git-url", "git-branch", "git-dir", "remote", "build", "builder", "builder-image", "image", "registry", "push", "platform", "namespace", "path", "verbose", "pvc-size"),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDeploy(cmd, newClient)
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
		cfg = cfg.Apply(f)
	}

	// Flags
	//
	// Globally-Configurable Flags:
	// Options whose value may be defined globally may also exist on the
	// contextually relevant function; but sets are flattened via cfg.Apply(f)
	cmd.Flags().StringP("builder", "b", cfg.Builder,
		fmt.Sprintf("Builder to use when creating the function's container. Currently supported builders are %s.", KnownBuilders()))
	cmd.Flags().StringP("registry", "r", cfg.Registry,
		"Container registry + registry namespace. (ex 'ghcr.io/myuser').  The full image name is automatically determined using this along with function name. ($FUNC_REGISTRY)")
	cmd.Flags().StringP("namespace", "n", cfg.Namespace,
		"Deploy into a specific namespace. Will use function's current namespace by default if already deployed, and the currently active namespace if it can be determined. ($FUNC_NAMESPACE)")

	// Function-Context Flags:
	// Options whose value is available on the function with context only
	// (persisted but not globally configurable)
	builderImage := f.Build.BuilderImages[f.Build.Builder]
	cmd.Flags().String("builder-image", builderImage,
		"Specify a custom builder image for use by the builder other than its default. ($FUNC_BUILDER_IMAGE)")
	cmd.Flags().StringP("image", "i", f.Image, "Full image name in the form [registry]/[namespace]/[name]:[tag]@[digest]. This option takes precedence over --registry. Specifying digest is optional, but if it is given, 'build' and 'push' phases are disabled. ($FUNC_IMAGE)")

	cmd.Flags().StringArrayP("env", "e", []string{}, "Environment variable to set in the form NAME=VALUE. "+
		"You may provide this flag multiple times for setting multiple environment variables. "+
		"To unset, specify the environment variable name followed by a \"-\" (e.g., NAME-).")
	cmd.Flags().StringP("git-url", "g", f.Build.Git.URL,
		"Repository url containing the function to build ($FUNC_GIT_URL)")
	cmd.Flags().StringP("git-branch", "t", f.Build.Git.Revision,
		"Git revision (branch) to be used when deploying via the Git repository ($FUNC_GIT_BRANCH)")
	cmd.Flags().StringP("git-dir", "d", f.Build.Git.ContextDir,
		"Directory in the Git repository containing the function (default is the root) ($FUNC_GIT_DIR)")
	cmd.Flags().BoolP("remote", "R", f.Deploy.Remote,
		"Trigger a remote deployment. Default is to deploy and build from the local system ($FUNC_REMOTE)")
	cmd.Flags().String("pvc-size", fn.DefaultPersistentVolumeClaimSize,
		"Configure the PVC size used by a pipeline during remote build.")

	// Static Flags:
	// Options which have static defaults only (not globally configurable nor
	// persisted with the function)
	cmd.Flags().String("build", "auto",
		"Build the function. [auto|true|false]. ($FUNC_BUILD)")
	cmd.Flags().Lookup("build").NoOptDefVal = "true" // register `--build` as equivalient to `--build=true`
	cmd.Flags().BoolP("push", "u", true,
		"Push the function image to registry before deploying. ($FUNC_PUSH)")
	cmd.Flags().String("platform", "",
		"Optionally specify a specific platform to build for (e.g. linux/amd64). ($FUNC_PLATFORM)")

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

func runDeploy(cmd *cobra.Command, newClient ClientFactory) (err error) {
	var (
		cfg deployConfig
		f   fn.Function
	)
	if err = config.CreatePaths(); err != nil { // for possible auth.json usage
		return
	}
	if cfg, err = newDeployConfig(cmd).Prompt(); err != nil {
		return
	}
	if err = cfg.Validate(cmd); err != nil {
		return
	}
	if f, err = fn.NewFunction(cfg.Path); err != nil {
		return
	}
	if f, err = cfg.Configure(f); err != nil { // Updates f with deploy cfg
		return
	}

	// TODO: this is duplicate logic with runBuild.
	// Refactor both to have this logic part of creating the buildConfig and thus
	// shared because newDeployConfig uses newBuildConfig for its embedded struct.
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

	// Informative non-error messages regarding the final deployment request
	printDeployMessages(cmd.OutOrStdout(), cfg)

	// Client
	// Concrete implementations (ex builder) vary  based on final effective cfg.
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

	client, done := newClient(ClientConfig{Namespace: f.Deploy.Namespace, Verbose: cfg.Verbose},
		fn.WithRegistry(cfg.Registry),
		fn.WithBuilder(builder))
	defer done()

	// Deploy
	if cfg.Remote {
		// Invoke a remote build/push/deploy pipeline
		// Returned is the function with fields like Registry and Image populated.
		if f, err = client.RunPipeline(cmd.Context(), f); err != nil {
			return
		}
	} else {
		if shouldBuild(cfg.Build, f, client) { // --build or "auto" with FS changes
			if f, err = client.Build(cmd.Context(), f); err != nil {
				return
			}
		}
		if cfg.Push {
			if f, err = client.Push(cmd.Context(), f); err != nil {
				return
			}
		}
		if f, err = client.Deploy(cmd.Context(), f, fn.WithDeploySkipBuildCheck(cfg.Build == "false")); err != nil {
			return
		}
	}

	// mutations persisted on success
	return f.Write()
}

// shouldBuild returns true if the value of the build option is a truthy value,
// or if it is the literal "auto" and the function reports as being currently
// unbuilt.  Invalid errors are not reported as this is the purview of
// deployConfig.Validate
func shouldBuild(buildCfg string, f fn.Function, client *fn.Client) bool {
	if buildCfg == "auto" {
		return !f.Built() // first build or modified filesystem
	}
	build, _ := strconv.ParseBool(buildCfg)
	return build
}

func NewRegistryValidator(path string) survey.Validator {
	return func(val interface{}) error {

		// if the value passed in is the zero value of the appropriate type
		if len(val.(string)) == 0 {
			return fn.ErrRegistryRequired
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

type deployConfig struct {
	buildConfig // further embeds config.Global

	// Perform build using the settings from the embedded buildConfig struct.
	// Acceptable values are the keyword 'auto', or a truthy value such as
	// 'true', 'false, '1' or '0'.
	Build string

	// Env variables.  May include removals using a "-"
	Env []string

	// Git branch for remote builds
	GitBranch string

	// Directory in the git repo where the function is located
	GitDir string

	// Git repo url for remote builds
	GitURL string

	// Namespace override for the deployed function.  If provided, the
	// underlying platform will be instructed to deploy the function to the given
	// namespace (if such a setting is applicable; such as for Kubernetes
	// clusters).  If not provided, the currently configured namespace will be
	// used.  For instance, that which would be used by default by `kubectl`
	// (~/.kube/config) in the case of Kubernetes.
	Namespace string

	// Remote indicates the deployment (and possibly build) process are to
	// be triggered in a remote environment rather than run locally.
	Remote bool

	// PVCSize configures the PVC size used by the pipeline if --remote flag is set.
	PVCSize string
}

// newDeployConfig creates a buildConfig populated from command flags and
// environment variables; in that precedence.
func newDeployConfig(cmd *cobra.Command) (c deployConfig) {
	c = deployConfig{
		buildConfig: newBuildConfig(),
		Build:       viper.GetString("build"),
		Env:         viper.GetStringSlice("env"),
		GitBranch:   viper.GetString("git-branch"),
		GitDir:      viper.GetString("git-dir"),
		GitURL:      viper.GetString("git-url"),
		Namespace:   viper.GetString("namespace"),
		Remote:      viper.GetBool("remote"),
		PVCSize:     viper.GetString("pvc-size"),
	}
	// NOTE: .Env should be viper.GetStringSlice, but this returns unparsed
	// results and appears to be an open issue since 2017:
	// https://github.com/spf13/viper/issues/380
	var err error
	if c.Env, err = cmd.Flags().GetStringArray("env"); err != nil {
		fmt.Fprintf(cmd.OutOrStdout(), "error reading envs: %v", err)
	}
	return
}

// Configure the given function.  Updates a function struct with all
// configurable values.  Note that the config already includes function's
// current values, as they were passed through via flag defaults.
func (c deployConfig) Configure(f fn.Function) (fn.Function, error) {
	var err error

	// Bubble configure request
	//
	// The member values on the config object now take absolute precedence
	// because they include 1) static config 2) user's global config
	// 3) Environment variables and 4) flag values (which were set with their
	// default being 1-3).
	f = c.buildConfig.Configure(f) // also configures .buildConfig.Global

	// Configure basic members
	f.Build.Git.URL = c.GitURL
	f.Build.Git.ContextDir = c.GitDir
	f.Build.Git.Revision = c.GitBranch // TODO: should match; perhaps "refSpec"
	f.Deploy.Namespace = c.Namespace
	f.Deploy.Remote = c.Remote
	// Validate if PVC size can be parsed to quantity
	_, err = resource.ParseQuantity(c.PVCSize)
	if err != nil {
		return f, fmt.Errorf("cannot parse the provided PVC size '%s' due to: %w", c.PVCSize, err)
	}
	f.Build.PVCSize = c.PVCSize

	// ImageDigest
	// Parsed off f.Image if provided.  Deploying adds the ability to specify a
	// digest on the associated image (not available on build as nonsensical).
	newDigest, err := imageDigest(f.Image)
	if err != nil {
		return f, err
	}
	if newDigest != "" {
		f.ImageDigest = newDigest
	}

	// Envs
	// Preprocesses any Envs provided (which may include removals) into a final
	// set
	f.Run.Envs, err = applyEnvs(f.Run.Envs, c.Env)
	if err != nil {
		return f, err
	}

	// .Revision
	// TODO: the system should support specifying revision (refSpec) as a URL
	// fragment (<url>[#<refspec>]) throughout, which, when implemented, removes
	// the need for the below split into separate members:
	if parts := strings.SplitN(c.GitURL, "#", 2); len(parts) == 2 {
		f.Build.Git.URL = parts[0]
		f.Build.Git.Revision = parts[1]
	}
	return f, nil
}

// Apply Env additions/removals to a set of extant envs, returning the final
// merged list.
func applyEnvs(current []fn.Env, args []string) (final []fn.Env, err error) {
	// TODO: validate env test cases completely validate this functionality

	// Parse and Merge
	inserts, removals, err := util.OrderedMapAndRemovalListFromArray(args, "=")
	if err != nil {
		return
	}
	final, _, err = mergeEnvs(current, inserts, removals)
	return
}

// Prompt the user with value of config members, allowing for interactive changes.
// Skipped if not in an interactive terminal (non-TTY), or if --yes (agree to
// all prompts) was explicitly set.
func (c deployConfig) Prompt() (deployConfig, error) {
	var err error
	if c.buildConfig, err = c.buildConfig.Prompt(); err != nil {
		return c, err
	}

	if !interactiveTerminal() || !c.Confirm {
		return c, nil
	}

	var qs = []*survey.Question{
		{
			Name: "namespace",
			Prompt: &survey.Input{
				Message: "Destination namespace:",
				Default: c.Namespace,
			},
		},
		{
			Name: "remote",
			Prompt: &survey.Confirm{
				Message: "Trigger a remote (on-cluster) build?",
				Default: c.Remote,
			},
		},
	}
	if err = survey.Ask(qs, &c); err != nil {
		return c, err
	}

	if c.Remote {
		qs = []*survey.Question{
			{
				Name: "GitURL",
				Prompt: &survey.Input{
					Message: "URL to Git Repository for the remote to use (default is to send local source code)",
					Default: c.GitURL,
				},
			},
		}
		if err = survey.Ask(qs, &c); err != nil {
			return c, err
		}
	}

	// TODO: prompt for optional additional git settings here:
	// if c.GitURL != "" {
	// }

	return c, err
}

// Validate the config passes an initial consistency check
func (c deployConfig) Validate(cmd *cobra.Command) (err error) {
	// Bubble validation
	if err = c.buildConfig.Validate(); err != nil {
		return
	}

	// Check Image Digest was included
	// (will be set on the function during .Configure)
	var digest string
	if digest, err = imageDigest(c.Image); err != nil {
		return
	}

	// --build can be "auto"|true|false
	if c.Build != "auto" {
		if _, err := strconv.ParseBool(c.Build); err != nil {
			return fmt.Errorf("unrecognized value for --build '%v'.  accepts 'auto', 'true' or 'false' (or similarly truthy value)", c.Build)
		}
	}

	// Can not enable build when specifying an --image with digest (already built)
	truthy := func(s string) bool {
		v, _ := strconv.ParseBool(s)
		return v
	}
	if digest != "" && truthy(c.Build) {
		return errors.New("building can not be enabled when using an image with digest")
	}

	// Can not push when specifying an --image with digest
	if digest != "" && c.Push {
		return errors.New("pushing is not valid when specifying an image with digest")
	}

	// Git references can only be supplied explicitly when coupled with --remote
	// See `printDeployMessages` which issues informative messages to the user
	// regarding this potentially confusing nuance.
	if !c.Remote && (cmd.Flags().Changed("git-url") || cmd.Flags().Changed("git-dir") || cmd.Flags().Changed("git-branch")) {
		return errors.New("git settings (--git-url --git-dir and --git-branch) are only applicable when triggering remote deployments (--remote)")
	}

	// Git URL can contain at maximum one '#'
	urlParts := strings.Split(c.GitURL, "#")
	if len(urlParts) > 2 {
		return fmt.Errorf("invalid --git-url '%v'", c.GitURL)
	}

	// NOTE: There is no explicit check for --registry or --image here, because
	// this logic is baked into core, which will validate the cases and return
	// an fn.ErrNameRequired, fn.ErrImageRequired etc. as needed.

	return
}

// imageDigest returns the image digest from a full image string if it exists,
// and includes basic validation that a provided digest is correctly formatted.
func imageDigest(v string) (digest string, err error) {
	vv := strings.Split(v, "@")
	if len(vv) < 2 {
		return // has no digest
	} else if len(vv) > 2 {
		err = fmt.Errorf("image '%v' contains an invalid digest (extra '@')", v)
		return
	}
	digest = vv[1]

	if !strings.HasPrefix(digest, "sha256:") {
		err = fmt.Errorf("image digest '%s' requires 'sha256:' prefix", digest)
		return
	}

	if len(digest[7:]) != 64 {
		err = fmt.Errorf("image digest '%v' has an invalid sha256 hash length of %v when it should be 64", digest, len(digest[7:]))
	}

	return
}

// printDeployMessages to the output.  Non-error deployment messages.
func printDeployMessages(out io.Writer, cfg deployConfig) {
	// Digest
	// ------
	// If providing an image digest, print this, and note that the values
	// of push and build are ignored.
	// TODO: perhaps just error if either --push or --build were actually
	// provided (using the cobra .Changed accessor)
	digest, err := imageDigest(cfg.Image)
	if err != nil && digest != "" {
		fmt.Fprintf(out, "Deploying image '%v' with digest '%s'. Build and push are disabled.\n", cfg.Image, digest)
	}

	// Namespace
	// ---------
	f, _ := fn.NewFunction(cfg.Path)
	currentNamespace := f.Deploy.Namespace // will be "" if no initialed f at path.
	targetNamespace := cfg.Namespace

	// If potentially creating a duplicate deployed function in a different
	// namespace.  TODO: perhaps add a --delete or --force flag which will
	// automagically delete the deployment in the "old" namespace.
	if targetNamespace != currentNamespace && currentNamespace != "" {
		fmt.Fprintf(out, "Warning: function is in namespace '%s', but requested namespace is '%s'. Continuing with deployment to '%v'.\n", currentNamespace, targetNamespace, targetNamespace)
	}

	// Namespace Changing
	// -------------------
	// If the target namespace is provided but differs from active, warn because
	// the function won't be visible to other commands such as kubectl unless
	// context namespace is switched.
	activeNamespace, err := k8s.GetNamespace("")
	if err == nil && targetNamespace != "" && targetNamespace != activeNamespace {
		fmt.Fprintf(out, "Warning: namespace chosen is '%s', but currently active namespace is '%s'. Continuing with deployment to '%s'.\n", cfg.Namespace, activeNamespace, cfg.Namespace)
	}

	// Git Args
	// -----------------
	// Print a warning if the function already contains Git attributes, but the
	// current invocation is not remote.  (providing Git attributes directly
	// via flags without --remote will error elsewhere).
	//
	// When invoking a remote build with --remote, the --git-X arguments
	// are persisted to the local function's source code such that the reference
	// is retained.  Subsequent runs of deploy then need not have these arguments
	// present.
	//
	// However, when building _locally_ thereafter, the deploy command should
	// prefer the local source code, ignoring the values for --git-url etc.
	// Since this might be confusing, a warning is issued below that the local
	// function source does include a reference to a git repository, but that it
	// will be ignored in favor of the local source code since --remote was not
	// specified.
	if !cfg.Remote && (cfg.GitURL != "" || cfg.GitBranch != "" || cfg.GitDir != "") {
		fmt.Fprintf(out, "Warning: git settings are only applicable when running with --remote.  Local source code will be used.")
	}

}
