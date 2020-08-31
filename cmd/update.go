package cmd

import (
	"github.com/ory/viper"
	"github.com/spf13/cobra"

	"github.com/boson-project/faas"
	"github.com/boson-project/faas/buildpacks"
	"github.com/boson-project/faas/docker"
	"github.com/boson-project/faas/knative"
)

func init() {
	root.AddCommand(updateCmd)
	updateCmd.Flags().StringP("namespace", "n", "", "Override namespace for the Function (on supported platforms).  Default is to use currently active underlying platform setting - $FAAS_NAMESPACE")
	updateCmd.Flags().StringP("path", "p", cwd(), "Path to the Function project directory - $FAAS_PATH")
	updateCmd.Flags().StringP("repository", "r", "", "Repository for built images, ex 'docker.io/myuser' or just 'myuser'.  - $FAAS_REPOSITORY")
}

var updateCmd = &cobra.Command{
	Use:        "update [options]",
	Short:      "Update or create a deployed Function",
	Long:       `Update deployed Function to match the current local state.`,
	SuggestFor: []string{"push", "deploy"},
	PreRunE:    bindEnv("namespace", "path", "repository"),
	RunE:       runUpdate,
}

func runUpdate(cmd *cobra.Command, args []string) (err error) {
	config := newUpdateConfig()

	builder := buildpacks.NewBuilder()
	builder.Verbose = config.Verbose

	pusher := docker.NewPusher()
	pusher.Verbose = config.Verbose

	updater, err := knative.NewUpdater(config.Namespace)
	if err != nil {
		return
	}
	updater.Verbose = config.Verbose

	client := faas.New(
		faas.WithVerbose(config.Verbose),
		faas.WithBuilder(builder),
		faas.WithPusher(pusher),
		faas.WithUpdater(updater))

	// overrieNamespace to which the Function is pinned (deployed/updated etc)
	if err = overrideNamespace(config.Path, config.Namespace); err != nil {
		return
	}

	return client.Update(config.Path)
}

type updateConfig struct {
	// Namespace override for the deployed Function.  If provided, the
	// underlying platform will be instructed to deploy the Function to the given
	// namespace (if such a setting is applicable; such as for Kubernetes
	// clusters).  If not provided, the currently configured namespace will be
	// used.  For instance, that which would be used by default by `kubectl`
	// (~/.kube/config) in the case of Kubernetes.
	Namespace string

	// Path of the Function implementation on local disk. Defaults to current
	// working directory of the process.
	Path string

	// Repository at which interstitial build artifacts should be kept.
	// Registry is optional and is defaulted to faas.DefaultRegistry.
	// ex: "quay.io/myrepo" or "myrepo"
	// This setting is ignored if Image is specified, which includes the full
	Repository string

	// Verbose logging.
	Verbose bool
}

func newUpdateConfig() updateConfig {
	return updateConfig{
		Namespace:  viper.GetString("namespace"),
		Path:       viper.GetString("path"),
		Repository: viper.GetString("repository"),
		Verbose:    viper.GetBool("verbose"), // defined on root
	}
}
