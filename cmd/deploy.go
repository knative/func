package cmd

import (
	"fmt"
	"github.com/ory/viper"
	"github.com/spf13/cobra"

	"github.com/boson-project/faas"
	"github.com/boson-project/faas/buildpacks"
	"github.com/boson-project/faas/docker"
	"github.com/boson-project/faas/knative"
	"github.com/boson-project/faas/progress"
	"github.com/boson-project/faas/prompt"
)

func init() {
	root.AddCommand(deployCmd)
	deployCmd.Flags().BoolP("confirm", "c", false, "Prompt to confirm all configuration options (Env: $FUNC_CONFIRM)")
	deployCmd.Flags().StringArrayP("env", "e", []string{}, "Environment variable to set in the form NAME=VALUE. You may provide this flag multiple times for setting multiple environment variables.")
	deployCmd.Flags().StringP("image", "i", "", "Full image name in the orm [registry]/[namespace]/[name]:[tag] (optional). This option takes precedence over --registry (Env: $FUNC_IMAGE")
	deployCmd.Flags().StringP("namespace", "n", "", "Namespace of the function to undeploy. By default, the namespace in func.yaml is used or the actual active namespace if not set in the configuration. (Env: $FUNC_NAMESPACE)")
	deployCmd.Flags().StringP("path", "p", cwd(), "Path to the project directory (Env: $FUNC_PATH)")
	deployCmd.Flags().StringP("registry", "r", "", "Registry + namespace part of the image to build, ex 'quay.io/myuser'.  The full image name is automatically determined based on the local directory name. If not provided the registry will be taken from func.yaml (Env: $FUNC_REGISTRY)")
}

var deployCmd = &cobra.Command{
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
kn func deploy --registry quay.io/myuser

# Same as above but using a full image name, that will create a Knative service "myfunc" in 
# the namespace "myns"
kn func deploy --image quay.io/myuser/myfunc -n myns
`,
    SuggestFor: []string{"delpoy", "deplyo"},
	PreRunE:    bindEnv("image", "namespace", "path", "registry", "confirm"),
	RunE:       runDeploy,
}

func runDeploy(cmd *cobra.Command, _ []string) (err error) {

	config := newDeployConfig(cmd).Prompt()

	function, err := functionWithOverrides(config.Path, functionOverrides{Namespace: config.Namespace, Image: config.Image})
	if err != nil {
		return
	}

	function.EnvVars = mergeEnvVarsMaps(function.EnvVars, config.EnvVars)

	// Check if the Function has been initialized
	if !function.Initialized() {
		return fmt.Errorf("the given path '%v' does not contain an initialized function. Please create one at this path before deploying", config.Path)
	}

	// If the Function does not yet have an image name and one was not provided on the command line
	if function.Image == "" {
		//  AND a --registry was not provided, then we need to
		// prompt for a registry from which we can derive an image name.
		if config.Registry == "" {
			fmt.Print("A registry for Function images is required. For example, 'docker.io/tigerteam'.\n\n")
			config.Registry = prompt.ForString("Registry for Function images", "")
			if config.Registry == "" {
				return fmt.Errorf("unable to determine function image name")
			}
		}

		// We have the registry, so let's use it to derive the Function image name
		config.Image = deriveImage(config.Image, config.Registry, config.Path)
		function.Image = config.Image
	}

	// All set, let's write changes in the config to the disk
	err = function.WriteConfig()
	if err != nil {
		return
	}

	builder := buildpacks.NewBuilder()
	builder.Verbose = config.Verbose

	pusher := docker.NewPusher()
	pusher.Verbose = config.Verbose

	ns := config.Namespace
	if ns == "" {
		ns = function.Namespace
	}

	deployer, err := knative.NewDeployer(ns)
	if err != nil {
		return
	}

	listener := progress.New()

	deployer.Verbose = config.Verbose

	client := faas.New(
		faas.WithVerbose(config.Verbose),
		faas.WithRegistry(config.Registry), // for deriving image name when --image not provided explicitly.
		faas.WithBuilder(builder),
		faas.WithPusher(pusher),
		faas.WithDeployer(deployer),
		faas.WithProgressListener(listener))

	return client.Deploy(config.Path)

	// NOTE: Namespace is optional, default is that used by k8s client
	// (for example kubectl usually uses ~/.kube/config)
}

type deployConfig struct {
	buildConfig

	// Namespace override for the deployed function.  If provided, the
	// underlying platform will be instructed to deploy the function to the given
	// namespace (if such a setting is applicable; such as for Kubernetes
	// clusters).  If not provided, the currently configured namespace will be
	// used.  For instance, that which would be used by default by `kubectl`
	// (~/.kube/config) in the case of Kubernetes.
	Namespace string

	// Path of the Function implementation on local disk. Defaults to current
	// working directory of the process.
	Path string

	// Verbose logging.
	Verbose bool

	// Confirm: confirm values arrived upon from environment plus flags plus defaults,
	// with interactive prompting (only applicable when attached to a TTY).
	Confirm bool

	EnvVars map[string]string
}

// newDeployConfig creates a buildConfig populated from command flags and
// environment variables; in that precedence.
func newDeployConfig(cmd *cobra.Command) deployConfig {
	return deployConfig{
		buildConfig: newBuildConfig(),
		Namespace:   viper.GetString("namespace"),
		Path:        viper.GetString("path"),
		Verbose:     viper.GetBool("verbose"), // defined on root
		Confirm:     viper.GetBool("confirm"),
		EnvVars:     envVarsFromCmd(cmd),
	}
}

// Prompt the user with value of config members, allowing for interaractive changes.
// Skipped if not in an interactive terminal (non-TTY), or if --yes (agree to
// all prompts) was explicitly set.
func (c deployConfig) Prompt() deployConfig {
	if !interactiveTerminal() || !c.Confirm {
		return c
	}
	dc := deployConfig{
		buildConfig: buildConfig{
			Registry: prompt.ForString("Registry for Function images", c.buildConfig.Registry),
		},
		Namespace: prompt.ForString("Namespace", c.Namespace),
		Path:      prompt.ForString("Project path", c.Path),
		Verbose:   c.Verbose,
	}

	dc.Image = deriveImage(dc.Image, dc.Registry, dc.Path)

	return dc
}
