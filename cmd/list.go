package cmd

import (
	"fmt"

	"github.com/boson-project/faas/client"
	"github.com/boson-project/faas/knative"
	"github.com/ory/viper"
	"github.com/spf13/cobra"
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
	Short:      "Lists deployed Service Functions",
	Long:       `Lists deployed Service Functions`,
	SuggestFor: []string{"ls"},
	RunE:       list,
}

func list(cmd *cobra.Command, args []string) (err error) {
	var (
		namespace = viper.GetString(nsFlag)
		verbose   = viper.GetBool("verbose")
	)

	lister, err := knative.NewLister(namespace)
	if err != nil {
		return
	}
	lister.Verbose = verbose

	client, err := client.New(".",
		client.WithVerbose(verbose),
		client.WithLister(lister),
	)
	if err != nil {
		return
	}

	names, err := client.List()
	if err != nil {
		return
	}
	for _, name := range names {
		fmt.Printf("%s\n", name)
	}
	return
}
