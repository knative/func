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
	deployCmd.Flags().BoolP("confirm", "c", false, "Prompt to confirm all configuration options - $FAAS_CONFIRM")
	deployCmd.Flags().StringArrayP("env", "e", []string{}, "Sets environment variables for the Function.")
	deployCmd.Flags().StringP("image", "i", "", "Optional full image name, in form [registry]/[namespace]/[name]:[tag] for example quay.io/myrepo/project.name:latest (overrides --registry) - $FAAS_IMAGE")
	deployCmd.Flags().StringP("namespace", "n", "", "Override namespace into which the Function is deployed (on supported platforms).  Default is to use currently active underlying platform setting - $FAAS_NAMESPACE")
	deployCmd.Flags().StringP("path", "p", cwd(), "Path to the function project directory - $FAAS_PATH")
	deployCmd.Flags().StringP("registry", "r", "", "Image registry for built images, ex 'docker.io/myuser' or just 'myuser'.  - $FAAS_REGISTRY")
}

var deployCmd = &cobra.Command{
	Use:   "deploy",
	Short: "Deploy an existing Function project to a cluster",
	Long: `Deploy an existing Function project to a cluster

Builds and Deploys the Function project in the current directory. 
A path to the project directory may be provided using the --path or -p flag.
Reads the faas.yaml configuration file to determine the image name. 
An image and registry may be specified on the command line using 
the --image or -i and --registry or -r flag.

If the Function is already deployed, it is updated with a new container image
that is pushed to an image registry, and the Knative Service is updated.

The namespace into which the project is deployed defaults to the value in the
faas.yaml configuration file. If NAMESPACE is not set in the configuration,
the namespace currently active in the Kubernetes configuration file will be
used. The namespace may be specified on the command line using the --namespace
or -n flag, and if so this will overwrite the value in the faas.yaml file.


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
		return fmt.Errorf("the given path '%v' does not contain an initialized Function. Please create one at this path before deploying.", config.Path)
	}

	// If the Function does not yet have an image name and one was not provided on the command line
	if function.Image == "" {
		//  AND a --registry was not provided, then we need to
		// prompt for a registry from which we can derive an image name.
		if config.Registry == "" {
			fmt.Print("A registry for Function images is required. For example, 'docker.io/tigerteam'.\n\n")
			config.Registry = prompt.ForString("Registry for Function images", "")
			if config.Registry == "" {
				return fmt.Errorf("Unable to determine Function image name")
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

	deployer, err := knative.NewDeployer(config.Namespace)
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
