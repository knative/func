package cmd

import (
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"

	"gopkg.in/yaml.v2"

	"github.com/boson-project/faas/client"
	"github.com/boson-project/faas/knative"
	"github.com/ory/viper"
	"github.com/spf13/cobra"
)

func init() {
	root.AddCommand(describeCmd)

	describeCmd.Flags().StringP("output", "o", "yaml", "optionally specify output format (yaml,xml,json).")
	viper.BindPFlag("output", describeCmd.Flags().Lookup("output"))
}

var describeCmd = &cobra.Command{
	Use:        "describe",
	Short:      "Describe Service Function",
	Long:       `Describe Service Function`,
	SuggestFor: []string{"desc"},
	Args:       cobra.ExactArgs(1),
	RunE:       describe,
}

func describe(cmd *cobra.Command, args []string) (err error) {
	var (
		verbose = viper.GetBool("verbose")
		format  = viper.GetString("output")
	)
	name := args[0]

	describer, err := knative.NewDescriber(client.DefaultNamespace)
	if err != nil {
		return
	}
	describer.Verbose = verbose

	client, err := client.New(".",
		client.WithVerbose(verbose),
		client.WithDescriber(describer),
	)
	if err != nil {
		return
	}

	description, err := client.Describe(name)
	if err != nil {
		return
	}

	formatFunctions := map[string]func(interface{}) ([]byte, error){
		"json": json.Marshal,
		"yaml": yaml.Marshal,
		"xml":  xml.Marshal,
	}

	formatFun, found := formatFunctions[format]
	if !found {
		return errors.New("unknown output format")
	}
	data, err := formatFun(description)
	if err != nil {
		return
	}
	fmt.Println(string(data))

	return
}
