package cmd

import (
	"fmt"
	"strings"

	"github.com/ory/viper"
	"github.com/spf13/cobra"
	fn "knative.dev/func/pkg/functions"
)

func NewSubscribeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "subscribe",
		Short: "Subscribe a function to events",
		Long: `Subscribe a function to events

Subscribe the function to a set of events, matching a set of filters for Cloud Event metadata
and a Knative Broker from where the events are consumed.
`,
		Example: `
# Subscribe the function to the 'default' broker where  events have 'type' of 'com.example'
and an 'extension' attribute for the value 'my-extension-value'.
{{rootCmdUse}} subscribe --filter type=com.example --filter extension=my-extension-value

# Subscribe the function to the 'my-broker' broker where  events have 'type' of 'com.example'
and an 'extension' attribute for the value 'my-extension-value'.
{{rootCmdUse}} subscribe --filter type=com.example --filter extension=my-extension-value --source my-broker
`,
		SuggestFor: []string{"subcsribe"}, //nolint:misspell
		PreRunE:    bindEnv("filter", "source"),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSubscribe(cmd, args)
		},
	}

	cmd.Flags().StringArrayP("filter", "f", []string{}, "Filter for the Cloud Event metadata")

	cmd.Flags().StringP("source", "s", "default", "The source, like a Knative Broker")

	addPathFlag(cmd)

	return cmd
}

func runSubscribe(cmd *cobra.Command, args []string) (err error) {
	var (
		cfg subscibeConfig
		f   fn.Function
	)
	cfg = newSubscribeConfig(cmd)

	if f, err = fn.NewFunction(effectivePath()); err != nil {
		return
	}
	if !f.Initialized() {
		return fn.NewErrNotInitialized(f.Root)
	}
	if !f.Initialized() {
		return fn.NewErrNotInitialized(f.Root)
	}

	// add subscription	to function
	f.Deploy.Subscriptions = updateOrAddSubscription(f.Deploy.Subscriptions, cfg)

	// pump it
	return f.Write()
}

func extractFilterMap(filters []string) map[string]string {
	subscriptionFilters := make(map[string]string)
	for _, filter := range filters {
		kv := strings.Split(filter, "=")
		if len(kv) != 2 {
			fmt.Println("Invalid pair:", filter)
			continue
		}
		key := kv[0]
		value := kv[1]
		subscriptionFilters[key] = value
	}
	return subscriptionFilters
}

type subscibeConfig struct {
	Filter []string
	Source string
}

func updateOrAddSubscription(subscriptions []fn.KnativeSubscription, cfg subscibeConfig) []fn.KnativeSubscription {
	found := false
	newFilters := extractFilterMap(cfg.Filter)

	// Iterate over subscriptions to find if one with the same source already exists
	for i, subscription := range subscriptions {
		if subscription.Source == cfg.Source {
			found = true
			// Update filters. Override if the key already exists.
			for newKey, newValue := range newFilters {
				subscription.Filters[newKey] = newValue
			}
			subscriptions[i] = subscription // Reassign the updated subscription
			break
		}
	}

	// If a subscription with the source was not found, add a new one
	if !found {
		subscriptions = append(subscriptions, fn.KnativeSubscription{
			Source:  cfg.Source,
			Filters: newFilters,
		})
	}
	return subscriptions
}

func newSubscribeConfig(cmd *cobra.Command) (c subscibeConfig) {
	c = subscibeConfig{
		Filter: viper.GetStringSlice("filter"),
		Source: viper.GetString("source"),
	}
	// NOTE: .Filter should be viper.GetStringSlice, but this returns unparsed
	// results and appears to be an open issue since 2017:
	// https://github.com/spf13/viper/issues/380
	var err error
	if c.Filter, err = cmd.Flags().GetStringArray("filter"); err != nil {
		fmt.Fprintf(cmd.OutOrStdout(), "error reading filter arguments: %v", err)
	}

	return
}
