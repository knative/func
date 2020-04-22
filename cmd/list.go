package cmd

import (
	"fmt"
	"github.com/ory/viper"
	"github.com/spf13/cobra"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/clientcmd"
	servingv1client "knative.dev/serving/pkg/client/clientset/versioned/typed/serving/v1"
)

const (
	nsFlag = "namespace"
)

func init() {
	root.AddCommand(listCmd)
	listCmd.Flags().StringP(nsFlag, "n", "", "optionally specify a namespace")
	viper.BindPFlag(nsFlag, listCmd.Flags().Lookup(nsFlag))
}

var listCmd = &cobra.Command{
	Use:        "list",
	Short:      "Lists deployed Service Function",
	Long:       `Lists deployed Service Function`,
	SuggestFor: []string{"ls"},
	RunE:       list,
}

func list(cmd *cobra.Command, args []string) (err error) {
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	clientConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, &clientcmd.ConfigOverrides{})
	config, err := clientConfig.ClientConfig()
	if err != nil {
		return
	}
	client, err := servingv1client.NewForConfig(config)
	if err != nil {
		return
	}
	opts := metav1.ListOptions{LabelSelector: "bosonFunction"}
	ns := viper.GetString(nsFlag)
	lst, err := client.Services(ns).List(opts)
	if err != nil {
		return
	}
	for _, service := range lst.Items {
		fmt.Printf("%s/%s", service.Namespace, service.Name)
	}
	return nil
}
