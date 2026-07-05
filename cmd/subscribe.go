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
		PreRunE:    bindEnv("filter", "source", "verbose"),
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runSubscribe(cmd)
		},
	}

	cmd.Flags().StringArrayP("filter", "f", []string{}, "Filter for the Cloud Event metadata")

	cmd.Flags().StringP("source", "s", "default", "The source, like a Knative Broker")

	addPathFlag(cmd)
	addVerboseFlag(cmd, false)

	return cmd
}

func runSubscribe(cmd *cobra.Command) (err error) {
	var (
		cfg subscibeConfig
		f   fn.Function
	)
	cfg = newSubscribeConfig(cmd)

	if f, err = fn.NewFunction(effectivePath()); err != nil {
		return
	}
	if !f.Initialized() {
		return fmt.Errorf("no function found in current directory.\nYou need to be inside a function directory to subscribe to events.\n\nTry this:\n  func create --language go myfunction    Create a new function\n  cd myfunction                          Go into the function directory\n  func subscribe --filter type=example   Subscribe to events\n\nOr if you have an existing function:\n  cd path/to/your/function              Go to your function directory\n  func subscribe --filter type=example  Subscribe to events")
	}

	if f.Deploy.Subscriptions, err = updateOrAddSubscription(f.Deploy.Subscriptions, cfg); err != nil {
		return err
	}

	return f.Write()
}

func extractFilterMap(filters []string) (map[string]string, error) {
	subscriptionFilters := make(map[string]string)
	for _, filter := range filters {
		key, value, found := strings.Cut(filter, "=")
		if !found || strings.TrimSpace(key) == "" {
			return nil, fmt.Errorf("invalid --filter %q: must be in key=value format", filter)
		}
		subscriptionFilters[key] = value
	}
	return subscriptionFilters, nil
}

type subscibeConfig struct {
	Filter []string
	Source string
}

func updateOrAddSubscription(subscriptions []fn.KnativeSubscription, cfg subscibeConfig) ([]fn.KnativeSubscription, error) {
	found := false
	newFilters, err := extractFilterMap(cfg.Filter)
	if err != nil {
		return nil, err
	}
	// Iterate over subscriptions to find if one with the same source already exists
	for i, subscription := range subscriptions {
		if subscription.Source == cfg.Source {
			found = true

			if subscription.Filters == nil {
				subscription.Filters = make(map[string]string)
			}

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
	return subscriptions, nil
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
