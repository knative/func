package cmd

import (
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"

	"gopkg.in/yaml.v2"

	"github.com/boson-project/faas"
	"github.com/boson-project/faas/knative"
	"github.com/ory/viper"
	"github.com/spf13/cobra"
)

func init() {
	root.AddCommand(describeCmd)

	describeCmd.Flags().StringP("output", "o", "yaml", "optionally specify output format (yaml,xml,json).")

	describeCmd.Flags().StringP("name", "n", "", "optionally specify an explicit name for the serive, overriding path-derivation. $FAAS_NAME")

	describeCmd.RegisterFlagCompletionFunc("name", CompleteFunctionList)

	describeCmd.RegisterFlagCompletionFunc("output", CompleteOutputFormatList)
}

var describeCmd = &cobra.Command{
	Use:               "describe",
	Short:             "Describe Function",
	Long:              `Describe Function`,
	SuggestFor:        []string{"desc"},
	ValidArgsFunction: CompleteFunctionList,
	Args:              cobra.ExactArgs(1),
	RunE:              describe,
	PreRun: func(cmd *cobra.Command, args []string) {
		viper.BindPFlag("output", cmd.Flags().Lookup("output"))
		viper.BindPFlag("name", cmd.Flags().Lookup("name"))
	},
}

func describe(cmd *cobra.Command, args []string) (err error) {
	var (
		verbose = viper.GetBool("verbose")
		format  = viper.GetString("output")
		name    = viper.GetString("name")
		path    = "" // default to current working directory
	)
	// If provided use the path as the first argument
	if len(args) == 1 {
		name = args[0]
	}

	describer, err := knative.NewDescriber(faas.DefaultNamespace)
	if err != nil {
		return
	}
	describer.Verbose = verbose

	client, err := faas.New(
		faas.WithVerbose(verbose),
		faas.WithDescriber(describer),
	)
	if err != nil {
		return
	}

	// describe the given name, or path if not provided.
	description, err := client.Describe(name, path)
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
