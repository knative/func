package cmd

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"os"

	"github.com/ory/viper"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v2"

	"github.com/boson-project/faas"
	"github.com/boson-project/faas/knative"
)

func init() {
	root.AddCommand(listCmd)
	listCmd.Flags().StringP("namespace", "n", "", "Override namespace in which to search for Functions.  Default is to use currently active underlying platform setting - $FAAS_NAMESPACE")
	listCmd.Flags().StringP("format", "f", "human", "optionally specify output format (human|plain|json|xml|yaml) $FAAS_FORMAT")

	err := listCmd.RegisterFlagCompletionFunc("format", CompleteOutputFormatList)
	if err != nil {
		fmt.Println("Error while calling RegisterFlagCompletionFunc: ", err)
	}
}

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "Lists deployed Functions",
	Long: `Lists deployed Functions

Lists all deployed functions. The namespace defaults to the value in faas.yaml
or the namespace currently active in the user's Kubernetes configuration. The
namespace may be specified on the command line using the --namespace or -n flag.
If specified this will overwrite the value in faas.yaml.
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

	nn, err := client.List()
	if err != nil {
		return
	}

	write(os.Stdout, names(nn), config.Format)
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

type names []string

func (nn names) Human(w io.Writer) error {
	return nn.Plain(w)
}

func (nn names) Plain(w io.Writer) error {
	for _, name := range nn {
		fmt.Fprintln(w, name)
	}
	return nil
}

func (nn names) JSON(w io.Writer) error {
	return json.NewEncoder(w).Encode(nn)
}

func (nn names) XML(w io.Writer) error {
	return xml.NewEncoder(w).Encode(nn)
}

func (nn names) YAML(w io.Writer) error {
	// the yaml.v2 package refuses to directly serialize a []string unless
	// exposed as a public struct member; so an inline anonymous is used.
	ff := struct{ Names []string }{nn}
	return yaml.NewEncoder(w).Encode(ff.Names)
}
