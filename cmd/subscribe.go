package cmd

import (
	"github.com/ory/viper"
	"github.com/spf13/cobra"
	"knative.dev/func/pkg/config"
	fn "knative.dev/func/pkg/functions"
)

func NewSubscribeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:        "subscribe",
		Short:      "Subscribe to a function",
		Long:       `Subscribe to a function`,
		SuggestFor: []string{"subscribe", "subscribe"},
		PreRunE:    bindEnv("filter", "source"),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSubscribe(cmd, args)
		},
	}

	cmd.Flags().StringP("filter", "f", "", "The event metadata to filter for")
	cmd.Flags().StringP("source", "s", "default", "The source, like a Knative Broker")

	return cmd
}

// /
func runSubscribe(cmd *cobra.Command, args []string) (err error) {
	var (
		cfg subscibeConfig
		f   fn.Function
	)
	cfg = newSubscribeConfig()

	if f, err = fn.NewFunction(""); err != nil {
		return
	}
	if !f.Initialized() {
		return fn.NewErrNotInitialized(f.Root)
	}
	if !f.Initialized() {
		return fn.NewErrNotInitialized(f.Root)
	}

	// add it
	f.Subscription = append(f.Subscription, fn.SubscriptionSpec{
		Source: cfg.Source,
		Filters: map[string]string{
			"type": cfg.Filter,
		},
	})

	// pump it
	return f.Write()

}

type subscibeConfig struct {
	Filter string
	Source string
}

func newSubscribeConfig() subscibeConfig {
	c := subscibeConfig{
		Filter: viper.GetString("filter"),
		Source: viper.GetString("source"),
	}

	return c
}
