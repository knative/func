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
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runSubscribe(cmd)
		},
	}

	cmd.Flags().StringArrayP("filter", "f", []string{}, "Filter for the Cloud Event metadata")

	cmd.Flags().StringP("source", "s", "default", "The source, like a Knative Broker")

	addPathFlag(cmd)

	return cmd
}

func runSubscribe(cmd *cobra.Command) (err error) {
	var (
		cfg subscribeConfig
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
	f.Deploy.Subscriptions = addNewSubscriptions(f.Deploy.Subscriptions, cfg)

	// pump it
	return f.Write()
}

func extractNewSubscriptions(filters []string, source string) (newSubscriptions []*subscriptionConfig) {
	for _, filter := range filters {
		kv := strings.Split(filter, "=")
		if len(kv) != 2 {
			fmt.Println("Invalid pair:", filter)
			continue
		}
		filterKey := kv[0]
		filterValue := kv[1]
		newSubscriptions = append(newSubscriptions, &subscriptionConfig{
			Source:      source,
			FilterKey:   filterKey,
			FilterValue: filterValue,
		})
	}
	return newSubscriptions
}

type subscriptionConfig struct {
	Source      string
	FilterKey   string
	FilterValue string
}

type subscribeConfig struct {
	Filter []string
	Source string
}

func addNewSubscriptions(subscriptions []fn.KnativeSubscription, cfg subscribeConfig) []fn.KnativeSubscription {
	newSubscriptions := extractNewSubscriptions(cfg.Filter, cfg.Source)
	for _, newSubscription := range newSubscriptions {
		isNew := true
		for _, subscription := range subscriptions {
			if subscription.Source == newSubscription.Source {
				for k, v := range subscription.Filters {
					if k == newSubscription.FilterKey && v == newSubscription.FilterValue {
						isNew = false
						break
					}
				}
			}
		}
		if isNew {
			newFilter := make(map[string]string)
			newFilter[newSubscription.FilterKey] = newSubscription.FilterValue
			subscriptions = append(subscriptions, fn.KnativeSubscription{
				Source:  cfg.Source,
				Filters: newFilter,
			})
		}
	}
	return subscriptions
}

func newSubscribeConfig(cmd *cobra.Command) (c subscribeConfig) {
	c = subscribeConfig{
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
