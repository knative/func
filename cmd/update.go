package cmd

import (
	"errors"

	"github.com/boson-project/faas/buildpacks"

	"github.com/ory/viper"
	"github.com/spf13/cobra"

	"github.com/boson-project/faas"
	"github.com/boson-project/faas/docker"
	"github.com/boson-project/faas/kn"
)

func init() {
	root.AddCommand(updateCmd)
	updateCmd.Flags().StringP("registry", "r", "quay.io", "image registry (ex: quay.io). $FAAS_REGISTRY")
	updateCmd.Flags().StringP("namespace", "s", "", "namespace at image registry (usually username or org name). $FAAS_NAMESPACE")
	updateCmd.RegisterFlagCompletionFunc("registry", CompleteRegistryList)
}

var updateCmd = &cobra.Command{
	Use:        "update",
	Short:      "Update or create a deployed Function",
	Long:       `Update deployed function to match the current local state.`,
	SuggestFor: []string{"push", "deploy"},
	RunE:       update,
	PreRun: func(cmd *cobra.Command, args []string) {
		viper.BindPFlag("registry", cmd.Flags().Lookup("registry"))
		viper.BindPFlag("namespace", cmd.Flags().Lookup("namespace"))
	},
}

func update(cmd *cobra.Command, args []string) (err error) {
	var (
		path      = ""                           // defaults to current working directory
		verbose   = viper.GetBool("verbose")     // Verbose logging
		registry  = viper.GetString("registry")  // Registry (ex: docker.io)
		namespace = viper.GetString("namespace") // namespace at registry (user or org name)
	)

	if len(args) == 1 {
		path = args[0]
	}

	// Namespace can not be defaulted.
	if namespace == "" {
		return errors.New("image registry namespace (--namespace or FAAS_NAMESPACE is required)")
	}

	// Builder creates images from function source.
	builder := buildpacks.NewBuilder(registry, namespace)
	builder.Verbose = verbose

	// Pusher of images
	pusher := docker.NewPusher()
	pusher.Verbose = verbose

	// Deployer of built images.
	updater := kn.NewUpdater()
	updater.Verbose = verbose

	client, err := faas.New(
		faas.WithVerbose(verbose),
		faas.WithBuilder(builder),
		faas.WithPusher(pusher),
		faas.WithUpdater(updater),
	)
	if err != nil {
		return
	}

	return client.Update(path)
}
