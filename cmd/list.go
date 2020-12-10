package cmd

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"text/tabwriter"

	"github.com/ory/viper"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v2"

	"github.com/boson-project/faas"
	"github.com/boson-project/faas/knative"
)

func init() {
	root.AddCommand(listCmd)
	listCmd.Flags().BoolP("all-namespaces", "A", false, "List functions in all namespaces. If set, the --namespace flag is ignored.")
	listCmd.Flags().StringP("namespace", "n", "", "Namespace of the function to undeploy. By default, the functions of the actual active namespace are listed. (Env: $FUNC_NAMESPACE)")
	listCmd.Flags().StringP("format", "f", "human", "Output format (human|plain|json|xml|yaml) (Env: $FUNC_FORMAT)")
	err := listCmd.RegisterFlagCompletionFunc("format", CompleteOutputFormatList)
	if err != nil {
		fmt.Println("internal: error while calling RegisterFlagCompletionFunc: ", err)
	}
}

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List functions",
	Long: `List functions

Lists all deployed functions in a given namespace.
`,
	Example: `
# List all functions in the current namespace in human readable format
kn func list

# List all functions in the 'test' namespace in yaml format
kn func list --namespace test --format yaml

# List all functions in all namespaces in JSON format
kn func list --all-namespaces --format json
`,
	SuggestFor: []string{"ls", "lsit"},
	PreRunE:    bindEnv("namespace", "format"),
	RunE:       runList,
}

func runList(cmd *cobra.Command, args []string) (err error) {
	config := newListConfig()

	lister, err := knative.NewLister(config.Namespace)
	if err != nil {
		return
	}
	lister.Verbose = config.Verbose

	a, err := cmd.Flags().GetBool("all-namespaces")
	if err != nil {
		return
	}
	if a {
		lister.Namespace = ""
	}

	client := faas.New(
		faas.WithVerbose(config.Verbose),
		faas.WithLister(lister))

	items, err := client.List()
	if err != nil {
		return
	}

	write(os.Stdout, listItems(items), config.Format)

	return
}

// CLI Configuration (parameters)
// ------------------------------

type listConfig struct {
	Namespace string
	Format    string
	Verbose   bool
}

func newListConfig() listConfig {
	return listConfig{
		Namespace: viper.GetString("namespace"),
		Format:    viper.GetString("format"),
		Verbose:   viper.GetBool("verbose"),
	}
}

// Output Formatting (serializers)
// -------------------------------

type listItems []faas.ListItem

func (items listItems) Human(w io.Writer) error {
	return items.Plain(w)
}

func (items listItems) Plain(w io.Writer) error {

	// minwidth, tabwidth, padding, padchar, flags
	tabWriter := tabwriter.NewWriter(w, 0, 8, 2, ' ', 0)
	defer tabWriter.Flush()

	fmt.Fprintf(tabWriter, "%s\t%s\t%s\t%s\t%s\t%s\n", "NAME", "NAMESPACE", "RUNTIME", "URL", "KSERVICE", "READY")
	for _, item := range items {
		fmt.Fprintf(tabWriter, "%s\t%s\t%s\t%s\t%s\t%s\n", item.Name, item.Namespace, item.Runtime, item.URL, item.KService, item.Ready)
	}
	return nil
}

func (items listItems) JSON(w io.Writer) error {
	return json.NewEncoder(w).Encode(items)
}

func (items listItems) XML(w io.Writer) error {
	return xml.NewEncoder(w).Encode(items)
}

func (items listItems) YAML(w io.Writer) error {
	return yaml.NewEncoder(w).Encode(items)
}
