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
	listCmd.Flags().StringP("namespace", "n", "", "Override namespace in which to search for Functions.  Default is to use currently active underlying platform setting - $FUNCTION_NAMESPACE")
	listCmd.Flags().StringP("format", "f", "human", "optionally specify output format (human|plain|json|xml|yaml) $FUNCTION_FORMAT")

	err := listCmd.RegisterFlagCompletionFunc("format", CompleteOutputFormatList)
	if err != nil {
		fmt.Println("Error while calling RegisterFlagCompletionFunc: ", err)
	}
}

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "Lists deployed Functions",
	Long: `Lists deployed Functions

Lists all deployed functions. The namespace defaults to the value in func.yaml
or the namespace currently active in the user's Kubernetes configuration. The
namespace may be specified on the command line using the --namespace or -n flag.
If specified this will overwrite the value in func.yaml.
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

	fmt.Fprintf(tabWriter, "%s\t%s\t%s\t%s\t%s\n", "NAME", "RUNTIME", "URL", "KSERVICE", "READY")
	for _, item := range items {
		fmt.Fprintf(tabWriter, "%s\t%s\t%s\t%s\t%s\n", item.Name, item.Runtime, item.URL, item.KService, item.Ready)
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
