package cmd

import (
	"errors"
	"fmt"

	"github.com/ory/viper"
	"github.com/spf13/cobra"

	"github.com/boson-project/faas"
	"github.com/boson-project/faas/buildpacks"
	"github.com/boson-project/faas/knative"
)

func init() {
	root.AddCommand(updateCmd)
	updateCmd.Flags().StringP("registry", "r", "quay.io", "image registry (ex: quay.io). $FAAS_REGISTRY")
	updateCmd.Flags().StringP("namespace", "s", "", "namespace at image registry (usually username or org name). $FAAS_NAMESPACE")
	err := updateCmd.RegisterFlagCompletionFunc("registry", CompleteRegistryList)
	if err != nil {
		fmt.Println("Error while calling RegisterFlagCompletionFunc: ", err)
	}
}

var updateCmd = &cobra.Command{
	Use:        "update",
	Short:      "Update or create a deployed Function",
	Long:       `Update deployed function to match the current local state.`,
	SuggestFor: []string{"push", "deploy"},
	RunE:       update,
	PreRun: func(cmd *cobra.Command, args []string) {
		err := viper.BindPFlag("registry", cmd.Flags().Lookup("registry"))
		if err != nil {
			panic(err)
		}
		err = viper.BindPFlag("namespace", cmd.Flags().Lookup("namespace"))
		if err != nil {
			panic(err)
		}
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
	// TODO: FIX ME - param should be an image tag
	builder := buildpacks.NewBuilder(registry)
	builder.Verbose = verbose

	// Pusher of images
	// pusher := docker.NewPusher()
	// pusher.Verbose = verbose

	// Deployer of built images.
	updater, err := knative.NewUpdater(faas.DefaultNamespace)
	if err != nil {
		return fmt.Errorf("couldn't create updater: %v", err)
	}
	updater.Verbose = verbose

	client, err := faas.New(
		faas.WithVerbose(verbose),
		faas.WithBuilder(builder),
		// TODO: FIX ME
		// faas.WithPusher(pusher),
		faas.WithUpdater(updater),
	)
	if err != nil {
		return
	}

	return client.Update(path)
}
