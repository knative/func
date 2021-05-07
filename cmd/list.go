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

	fn "github.com/boson-project/func"
	"github.com/boson-project/func/knative"
)

func init() {
	root.AddCommand(listCmd)
	listCmd.Flags().BoolP("all-namespaces", "A", false, "List functions in all namespaces. If set, the --namespace flag is ignored.")
	listCmd.Flags().StringP("namespace", "n", "", "Namespace to search for functions. By default, the functions of the actual active namespace are listed. (Env: $FUNC_NAMESPACE)")
	listCmd.Flags().StringP("output", "o", "human", "Output format (human|plain|json|xml|yaml) (Env: $FUNC_OUTPUT)")
	err := listCmd.RegisterFlagCompletionFunc("output", CompleteOutputFormatList)
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
# List all functions in the current namespace with human readable output
kn func list

# List all functions in the 'test' namespace with yaml output
kn func list --namespace test --output yaml

# List all functions in all namespaces with JSON output
kn func list --all-namespaces --output json
`,
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

	a, err := cmd.Flags().GetBool("all-namespaces")
	if err != nil {
		return
	}
	if a {
		lister.Namespace = ""
	}

	client := fn.New(
		fn.WithVerbose(config.Verbose),
		fn.WithLister(lister))

	items, err := client.List(cmd.Context())
	if err != nil {
		return
	}

	if len(items) < 1 {
		fmt.Printf("No functions found in %v namespace\n", lister.Namespace)
		return
	}

	write(os.Stdout, listItems(items), config.Output)

	return
}

// CLI Configuration (parameters)
// ------------------------------

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

// Output Formatting (serializers)
// -------------------------------

type listItems []fn.ListItem

func (items listItems) Human(w io.Writer) error {
	return items.Plain(w)
}

func (items listItems) Plain(w io.Writer) error {

	// minwidth, tabwidth, padding, padchar, flags
	tabWriter := tabwriter.NewWriter(w, 0, 8, 2, ' ', 0)
	defer tabWriter.Flush()

	fmt.Fprintf(tabWriter, "%s\t%s\t%s\t%s\t%s\n", "NAME", "NAMESPACE", "RUNTIME", "URL", "READY")
	for _, item := range items {
		fmt.Fprintf(tabWriter, "%s\t%s\t%s\t%s\t%s\n", item.Name, item.Namespace, item.Runtime, item.URL, item.Ready)
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
