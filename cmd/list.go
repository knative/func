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
	root.AddCommand(listCmd)
	listCmd.Flags().StringP("namespace", "n", "", "Override namespace in which to search for Functions.  Default is to use currently active underlying platform setting - $FAAS_NAMESPACE")
	listCmd.Flags().StringP("output", "o", "plain", "optionally specify output format (plain,json,yaml)")

	err := listCmd.RegisterFlagCompletionFunc("output", CompleteOutputFormatList)
	if err != nil {
		fmt.Println("Error while calling RegisterFlagCompletionFunc: ", err)
	}
}

var listCmd = &cobra.Command{
	Use:        "list [options]",
	Short:      "Lists deployed Functions",
	Long:       `Lists deployed Functions`,
	SuggestFor: []string{"ls", "lsit"},
	PreRunE:    bindEnv("namespace", "output"),
	RunE:       runList,
}

func runList(cmd *cobra.Command, args []string) (err error) {
	config := newListConfig()

	lister, err := knative.NewLister(config.Namespace)
	if err != nil {
		return
	}
	lister.Verbose = config.Verbose

	client := faas.New(
		faas.WithVerbose(verbose),
		faas.WithLister(lister))

	names, err := client.List()
	if err != nil {
		return
	}

	formatted, err := formatNames(names, config.Output)
	if err != nil {
		return
	}

	fmt.Println(formatted)
	return
}

// TODO: placeholder. Create a fit-for-purpose Names plaintext formatter
func fmtNamesPlain(i interface{}) ([]byte, error) {
	return []byte(fmt.Sprintf("%v", i)), nil
}

func formatNames(names []string, format string) (string, error) {
	formatters := map[string]func(interface{}) ([]byte, error){
		"plain": fmtNamesPlain,
		"json":  json.Marshal,
		"yaml":  yaml.Marshal,
		"xml":   xml.Marshal,
	}
	formatFn, ok := formatters[format]
	if !ok {
		return "", fmt.Errorf("Unknown format '%v'", format)
	}
	bytes, err := formatFn(names)
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}

type listConfig struct {
	Namespace string
	Output    string
	Verbose   bool
}

func newListConfig() listConfig {
	return listConfig{
		Namespace: viper.GetString("namespace"),
		Output:    viper.GetString("output"),
		Verbose:   viper.GetBool("verbose"),
	}
}

// DEPRECATED BELOW (?):
// TODO: regenerate completions, which may necessitate the below change:
/*

var validFormats []string

func completeFormats(cmd *cobra.Command, args []string, toComplete string) (formats []string, directive cobra.ShellCompDirective) {
	formats = validFormats
	directive = cobra.ShellCompDirectiveDefault
	return
}

type fmtFn func(writer io.Writer, names []string) error

func fmtPlain(writer io.Writer, names []string) error {
	for _, name := range names {
		_, err := fmt.Fprintf(writer, "%s\n", name)
		if err != nil {
			return err
		}
	}
	return nil
}

func fmtJSON(writer io.Writer, names []string) error {
	encoder := json.NewEncoder(writer)
	return encoder.Encode(names)
}

func fmtYAML(writer io.Writer, names []string) error {
	encoder := yaml.NewEncoder(writer)
	return encoder.Encode(names)
}
*/
