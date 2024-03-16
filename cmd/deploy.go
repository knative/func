package cmd

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/AlecAivazis/survey/v2"
	"github.com/ory/viper"
	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/api/resource"
	"knative.dev/client-pkg/pkg/util"
	"knative.dev/func/pkg/builders"
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
	             [-e|--env] [-g|--git-url] [-t|--git-branch] [-d|--git-dir]
	             [-b|--build] [--builder] [--builder-image] [-p|--push]
	             [--domain] [--platform] [--build-timestamp] [--pvc-size]
	             [--service-account] [-c|--confirm] [-v|--verbose]
	             [--registry-insecure]

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
	This mode is useful for the first deployment in particular, since subsequent
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

	Domain
	  When deploying, a function's route is automatically generated using the
	  default domain with which the target platform has been configured.  The
	  optional flag --domain can be used to choose this domain explicitly for
	  clusters which have been configured with support for function domain
	  selectors. Note that the domain specified must be one of those configured
	  or the flag will be ignored.

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
		PreRunE:    bindEnv("build", "build-timestamp", "builder", "builder-image", "confirm", "domain", "env", "git-branch", "git-dir", "git-url", "image", "namespace", "path", "platform", "push", "pvc-size", "service-account", "registry", "registry-insecure", "remote", "verbose"),
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
	cmd.Flags().Bool("registry-insecure", cfg.RegistryInsecure, "Disable HTTPS when communicating to the registry ($FUNC_REGISTRY_INSECURE)")
	cmd.Flags().StringP("namespace", "n", cfg.Namespace,
		"Deploy into a specific namespace. Will use function's current namespace by default if already deployed, and the currently active namespace if it can be determined. ($FUNC_NAMESPACE)")

	// Function-Context Flags:
	// Options whose value is available on the function with context only
	// (persisted but not globally configurable)
	builderImage := f.Build.BuilderImages[f.Build.Builder]
	cmd.Flags().String("builder-image", builderImage,
		"Specify a custom builder image for use by the builder other than its default. ($FUNC_BUILDER_IMAGE)")
	cmd.Flags().StringP("image", "i", f.Image,
		"Full image name in the form [registry]/[namespace]/[name]:[tag]@[digest]. This option takes precedence over --registry. Specifying digest is optional, but if it is given, 'build' and 'push' phases are disabled. ($FUNC_IMAGE)")

	cmd.Flags().StringArrayP("env", "e", []string{},
		"Environment variable to set in the form NAME=VALUE. "+
			"You may provide this flag multiple times for setting multiple environment variables. "+
			"To unset, specify the environment variable name followed by a \"-\" (e.g., NAME-).")
	cmd.Flags().String("domain", f.Domain,
		"Domain to use for the function's route.  Cluster must be configured with domain matching for the given domain (ignored if unrecognized) ($FUNC_DOMAIN)")
	cmd.Flags().StringP("git-url", "g", f.Build.Git.URL,
		"Repository url containing the function to build ($FUNC_GIT_URL)")
	cmd.Flags().StringP("git-branch", "t", f.Build.Git.Revision,
		"Git revision (branch) to be used when deploying via the Git repository ($FUNC_GIT_BRANCH)")
	cmd.Flags().StringP("git-dir", "d", f.Build.Git.ContextDir,
		"Directory in the Git repository containing the function (default is the root) ($FUNC_GIT_DIR)")
	cmd.Flags().BoolP("remote", "R", f.Local.Remote,
		"Trigger a remote deployment. Default is to deploy and build from the local system ($FUNC_REMOTE)")
	cmd.Flags().String("pvc-size", f.Build.PVCSize,
		"When triggering a remote deployment, set a custom volume size to allocate for the build operation ($FUNC_PVC_SIZE)")
	cmd.Flags().String("service-account", f.Deploy.ServiceAccountName,
		"Service account to be used in the deployed function ($FUNC_SERVICE_ACCOUNT)")
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
	if !f.Initialized() {
		return fn.NewErrNotInitialized(f.Root)
	}
	if f, err = cfg.Configure(f); err != nil { // Updates f with deploy cfg
		return
	}

	// If using Openshift registry AND redeploying Function, update image registry
	if f.Namespace != "" && f.Namespace != f.Deploy.Namespace && f.Deploy.Namespace != "" {
		// when running openshift, namespace is tied to registry, override on --namespace change
		// The most default part of registry (in buildConfig) checks 'k8s.IsOpenShift()' and if true,
		// sets default registry by current namespace

		// If Function is being moved to different namespace in Openshift -- update registry
		if k8s.IsOpenShift() {
			// this name is based of k8s package
			f.Registry = "image-registry.openshift-image-registry.svc:5000/" + f.Namespace
			if cfg.Verbose {
				fmt.Fprintf(cmd.OutOrStdout(), "Info: Overriding openshift registry to %s\n", f.Registry)
			}
		}
	}

	// Informative non-error messages regarding the final deployment request
	printDeployMessages(cmd.OutOrStdout(), f)

	clientOptions, err := cfg.clientOptions()
	if err != nil {
		return
	}
	client, done := newClient(ClientConfig{Namespace: f.Namespace, Verbose: cfg.Verbose}, clientOptions...)
	defer done()

	// Deploy
	if cfg.Remote {
		// Invoke a remote build/push/deploy pipeline
		// Returned is the function with fields like Registry, f.Deploy.Image &
		// f.Deploy.Namespace populated.
		if f, err = client.RunPipeline(cmd.Context(), f); err != nil {
			return
		}
	} else {
		var buildOptions []fn.BuildOption
		if buildOptions, err = cfg.buildOptions(); err != nil {
			return
		}

		// check if --image was provided with a digest. 'digested' bool indicates if
		// image contains a digest or not (image is "digested").
		var digested bool
		digested, err = isDigested(cfg.Image)
		if err != nil {
			return
		}

		// If user provided --image with digest, they are requesting that specific
		// image to be used which means building phase should be skipped and image
		// should be deployed as is
		if digested {
			f.Deploy.Image = cfg.Image
		} else {
			// if NOT digested, build and push the Function first
			if f, err = build(cmd, cfg.Build, f, client, buildOptions); err != nil {
				return
			}
			if cfg.Push {
				if f, err = client.Push(cmd.Context(), f); err != nil {
					return
				}
			}
			// f.Build.Image is set in Push for now, just set it as a deployed image
			f.Deploy.Image = f.Build.Image
		}

		if f, err = client.Deploy(cmd.Context(), f, fn.WithDeploySkipBuildCheck(cfg.Build == "false")); err != nil {
			return
		}
	}

	// Write
	if err = f.Write(); err != nil {
		return
	}
	// Stamp is a performance optimization: treat the function as being built
	// (cached) unless the fs changes.
	// Updates the build stamp because building must have been accomplished
	// during this process, and a future call to deploy without any appreciable
	// changes to the filesystem should not rebuild again unless `--build`
	return f.Stamp()
}

// build when flag == 'auto' and the function is out-of-date, or when the
// flag value is explicitly truthy such as 'true' or '1'.  Error if flag
// is neither 'auto' nor parseable as a boolean.  Return CLI-specific error
// message verbeage suitable for both Deploy and Run commands which feature an
// optional build step.
func build(cmd *cobra.Command, flag string, f fn.Function, client *fn.Client, buildOptions []fn.BuildOption) (fn.Function, error) {
	var err error
	if flag == "auto" {
		if f.Built() {
			fmt.Fprintln(cmd.OutOrStdout(), "function up-to-date. Force rebuild with --build")
		} else {
			if f, err = client.Build(cmd.Context(), f, buildOptions...); err != nil {
				return f, err
			}
		}
	} else if build, _ := strconv.ParseBool(flag); build {
		if f, err = client.Build(cmd.Context(), f, buildOptions...); err != nil {
			return f, err
		}
	} else if _, err = strconv.ParseBool(flag); err != nil {
		return f, fmt.Errorf("--build ($FUNC_BUILD) %q not recognized.  Should be 'auto' or a truthy value such as 'true', 'false', '0', or '1'.", flag)

	}
	return f, nil
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

	// Also a good place to stick feature-flags; to wit:
	enable_host, _ := strconv.ParseBool(os.Getenv("FUNC_ENABLE_HOST_BUILDER"))
	if !enable_host {
		bb := []string{}
		for _, b := range builders.All() {
			if b != builders.Host {
				bb = append(bb, b)
			}
		}
		return bb
	}

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

	// Domain to use for the function's route.  Default is to let the cluster
	// apply its default.  If configured to use domain matching, the given domain
	// will be used.  This configuration, in short, is to configure the
	// cluster's config-domain map to match on the `func.domain` label and use
	// its value as the domain... presuming it is one of those explicitly
	// enumerated.  This allows a function to be deployed explicitly choosing
	// a route from one of the domains in a cluster with multiple configured
	// domains.  Example:
	//   func create -l go hello && func deploy --domain domain2.org
	//   -> Func created as hello.[namespace].domain2.org
	// This can also be useful to configure a cluster to deploy functions at
	// the domain root when requested, but only be cluster-local (unexposed) by
	// default.  This is accomplished by configuring the cluster-domain map to
	// have the domain "cluster.local" as the default (empty selector), and
	// the domain map template to omit the namespace interstitial.
	// Example:
	//   func create -l go myclusterservice && func deploy
	//   ->  func creates myclusterservice.cluster.local which is not exposed
	//       publicly outside the cluster
	//   func create -l go www && func deploy --domain example.com
	//   -> func deploys www.example.com as a publicly exposed service.
	// TODO: allow for a simplified syntax of simply using the function's name
	// as its route, and automatically parse off the domain suffix and validate
	// the prefix is a dns label (ideally even validating the domain suffix is
	// currently available and configured on the cluster).
	// Example:
	// func create -l go www.example.com
	// -> func creates service www, with label func.domain as example.com, which
	//    is one which the cluster has configured to server, so it is deployed with
	//    a publicly accessible route
	// -> func create -l go myclusterservice.cluster.local
	//    is equivalent to `func create -l go myclusterservice`
	//  All func commands which operate on function name now instead can use
	// the FWDN.  Example `func delete www.example.com`
	Domain string

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

	//Service account to be used in deployed function
	ServiceAccountName string

	// Remote indicates the deployment (and possibly build) process are to
	// be triggered in a remote environment rather than run locally.
	Remote bool

	// PVCSize configures the PVC size used by the pipeline if --remote flag is set.
	PVCSize string

	// Timestamp the built contaienr with the current date and time.
	// This is currently only supported by the Pack builder.
	Timestamp bool
}

// newDeployConfig creates a buildConfig populated from command flags and
// environment variables; in that precedence.
func newDeployConfig(cmd *cobra.Command) (c deployConfig) {
	c = deployConfig{
		buildConfig:        newBuildConfig(),
		Build:              viper.GetString("build"),
		Env:                viper.GetStringSlice("env"),
		Domain:             viper.GetString("domain"),
		GitBranch:          viper.GetString("git-branch"),
		GitDir:             viper.GetString("git-dir"),
		GitURL:             viper.GetString("git-url"),
		Namespace:          viper.GetString("namespace"),
		Remote:             viper.GetBool("remote"),
		PVCSize:            viper.GetString("pvc-size"),
		Timestamp:          viper.GetBool("build-timestamp"),
		ServiceAccountName: viper.GetString("service-account"),
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
	f.Domain = c.Domain
	f.Namespace = c.Namespace
	f.Build.Git.URL = c.GitURL
	f.Build.Git.ContextDir = c.GitDir
	f.Build.Git.Revision = c.GitBranch // TODO: should match; perhaps "refSpec"
	f.Deploy.ServiceAccountName = c.ServiceAccountName
	f.Local.Remote = c.Remote

	// PVCSize
	// If a specific value is requested, ensure it parses as a resource.Quantity
	if c.PVCSize != "" {
		if _, err = resource.ParseQuantity(c.PVCSize); err != nil {
			return f, fmt.Errorf("cannot parse PVC size %q. %w", c.PVCSize, err)
		}
		f.Build.PVCSize = c.PVCSize
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
	var digest bool
	if digest, err = isDigested(c.Image); err != nil {
		return
	}

	// --build can be "auto"|true|false
	if c.Build != "auto" {
		if _, err := strconv.ParseBool(c.Build); err != nil {
			return fmt.Errorf("unrecognized value for --build '%v'.  Accepts 'auto', 'true' or 'false' (or similarly truthy value)", c.Build)
		}
	}

	// Can not enable build when specifying an --image with digest (already built)
	truthy := func(s string) bool {
		v, _ := strconv.ParseBool(s)
		return v
	}

	// Can not build when specifying an --image with digest
	if digest && truthy(c.Build) {
		return errors.New("building can not be enabled when using an image with digest")
	}

	// Can not push when specifying an --image with digest
	if digest && c.Push {
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

// printDeployMessages to the output.  Non-error deployment messages.
func printDeployMessages(out io.Writer, f fn.Function) {
	digest, err := isDigested(f.Image)
	if err == nil && digest {
		fmt.Fprintf(out, "Deploying image '%v', which has a digest. Build and push are disabled.\n", f.Image)
	}

	// Namespace
	// ---------
	currentNamespace := f.Deploy.Namespace // will be "" if no initialed f at path.
	targetNamespace := f.Namespace
	if targetNamespace == "" {
		return
	}

	// If creating a duplicate deployed function in a different
	// namespace.
	if targetNamespace != currentNamespace && currentNamespace != "" {
		fmt.Fprintf(out, "Info: chosen namespace has changed from '%s' to '%s'. Undeploying function from '%s' and deploying new in '%s'.\n", currentNamespace, targetNamespace, currentNamespace, targetNamespace)
	}

	// Namespace Changing
	// -------------------
	// If the target namespace is provided but differs from active, warn because
	// the function won't be visible to other commands such as kubectl unless
	// context namespace is switched.
	activeNamespace, err := k8s.GetDefaultNamespace()
	if err == nil && targetNamespace != "" && targetNamespace != activeNamespace {
		fmt.Fprintf(out, "Warning: namespace chosen is '%s', but currently active namespace is '%s'. Continuing with deployment to '%s'.\n", targetNamespace, activeNamespace, targetNamespace)
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

	// TODO update names of these to Source--Revision--Dir
	if !f.Local.Remote && (f.Build.Git.URL != "" || f.Build.Git.Revision != "" || f.Build.Git.ContextDir != "") {
		fmt.Fprintf(out, "Warning: git settings are only applicable when running with --remote.  Local source code will be used.")
	}
}

// isDigested returns true if provided image string 'v' has digest and false if not.
// Includes basic validation that a provided digest is correctly formatted.
func isDigested(v string) (validDigest bool, err error) {
	var digest string
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

	validDigest = true
	return
}
