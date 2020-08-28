package cmd

import (
	"github.com/ory/viper"
	"github.com/spf13/cobra"

	"github.com/boson-project/faas"
	"github.com/boson-project/faas/docker"
	"github.com/boson-project/faas/knative"
	"github.com/boson-project/faas/prompt"
)

func init() {
	root.AddCommand(deployCmd)
	deployCmd.Flags().StringP("namespace", "n", "", "Override namespace into which the Function is deployed (on supported platforms).  Default is to use currently active underlying platform setting - $FAAS_NAMESPACE")
	deployCmd.Flags().StringP("path", "p", cwd(), "Path to the function project directory - $FAAS_PATH")
	deployCmd.Flags().BoolP("yes", "y", false, "When in interactive mode (attached to a TTY) skip prompts. - $FAAS_YES")
}

var deployCmd = &cobra.Command{
	Use:        "deploy",
	Short:      "Deploy an existing Function project to a cluster",
	SuggestFor: []string{"delpoy", "deplyo"},
	PreRunE:    bindEnv("namespace", "path", "yes"),
	RunE:       runDeploy,
}

func runDeploy(cmd *cobra.Command, _ []string) (err error) {
	config := newDeployConfig().Prompt()

	pusher := docker.NewPusher()
	pusher.Verbose = config.Verbose

	deployer := knative.NewDeployer()
	deployer.Verbose = config.Verbose

	client := faas.New(
		faas.WithVerbose(config.Verbose),
		faas.WithPusher(pusher),
		faas.WithDeployer(deployer))

	// overrieNamespace into which the function is deployed, if --namespace provided.
	if err = overrideNamespace(config.Path, config.Namespace); err != nil {
		return
	}

	return client.Deploy(config.Path)

	// NOTE: Namespace is optional, default is that used by k8s client
	// (for example kubectl usually uses ~/.kube/config)
}

type deployConfig struct {
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

	// Yes: agree to values arrived upon from environment plus flags plus defaults,
	// and skip the interactive prompting (only applicable when attached to a TTY).
	Yes bool
}

// newDeployConfig creates a buildConfig populated from command flags and
// environment variables; in that precedence.
func newDeployConfig() deployConfig {
	return deployConfig{
		Namespace: viper.GetString("namespace"),
		Path:      viper.GetString("path"),
		Verbose:   viper.GetBool("verbose"), // defined on root
		Yes:       viper.GetBool("yes"),
	}
}

// Prompt the user with value of config members, allowing for interaractive changes.
// Skipped if not in an interactive terminal (non-TTY), or if --yes (agree to
// all prompts) was explicitly set.
func (c deployConfig) Prompt() deployConfig {
	if !interactiveTerminal() || c.Yes {
		return c
	}
	return deployConfig{
		Namespace: prompt.ForString("Override default namespace (optional)", c.Namespace),
		Path:      prompt.ForString("Path to project directory", c.Path),
		Verbose:   prompt.ForBool("Verbose logging", c.Verbose),
	}
}
