package cmd

import (
	"encoding/json"
	"encoding/xml"
	"fmt"

	"github.com/ory/viper"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v2"

	"github.com/boson-project/faas"
	"github.com/boson-project/faas/knative"
)

func init() {
	root.AddCommand(describeCmd)
	describeCmd.Flags().StringP("namespace", "n", "", "Override namespace in which to search for the Function.  Default is to use currently active underlying platform setting - $FAAS_NAMESPACE")
	describeCmd.Flags().StringP("output", "o", "yaml", "optionally specify output format (yaml,xml,json). - $FAAS_OUTPUT")
	describeCmd.Flags().StringP("path", "p", cwd(), "Path to the project which should be described - $FAAS_PATH")

	err := describeCmd.RegisterFlagCompletionFunc("output", CompleteOutputFormatList)
	if err != nil {
		fmt.Println("Error while calling RegisterFlagCompletionFunc: ", err)
	}
}

var describeCmd = &cobra.Command{
	Use:               "describe <name> [options]",
	Short:             "Describe Function",
	Long:              `Describes the Function by name, by explicit path, or by default the current directory.`,
	SuggestFor:        []string{"desc", "get"},
	ValidArgsFunction: CompleteFunctionList,
	PreRunE:           bindEnv("namespace", "output", "path"),
	RunE:              runDescribe,
}

func runDescribe(cmd *cobra.Command, args []string) (err error) {
	config := newDescribeConfig(args)

	describer, err := knative.NewDescriber(config.Namespace)
	if err != nil {
		return
	}
	describer.Verbose = config.Verbose

	client := faas.New(
		faas.WithVerbose(verbose),
		faas.WithDescriber(describer))

	description, err := client.Describe(config.Name, config.Path)
	if err != nil {
		return
	}

	formatted, err := formatDescription(description, config.Output)
	if err != nil {
		return
	}

	fmt.Println(formatted)
	return
}

// TODO: Placeholder.  Create a fit-for-purpose Description plaintext formatter.
func fmtDescriptionPlain(i interface{}) ([]byte, error) {
	return []byte(fmt.Sprintf("%v", i)), nil
}

// format the description as json|yaml|xml
func formatDescription(desc faas.FunctionDescription, format string) (string, error) {
	formatters := map[string]func(interface{}) ([]byte, error){
		"plain": fmtDescriptionPlain,
		"json":  json.Marshal,
		"yaml":  yaml.Marshal,
		"xml":   xml.Marshal,
	}
	formatFn, ok := formatters[format]
	if !ok {
		return "", fmt.Errorf("unknown format '%s'", format)
	}
	bytes, err := formatFn(desc)
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}

type describeConfig struct {
	Name      string
	Namespace string
	Output    string
	Path      string
	Verbose   bool
}

func newDescribeConfig(args []string) describeConfig {
	var name string
	if len(args) > 0 {
		name = args[0]
	}
	return describeConfig{
		Name:      deriveName(name, viper.GetString("path")),
		Namespace: viper.GetString("namespace"),
		Output:    viper.GetString("output"),
		Path:      viper.GetString("path"),
		Verbose:   viper.GetBool("verbose"),
	}
}
