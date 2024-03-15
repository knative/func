package cmd

import (
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"os"
	"text/tabwriter"

	"github.com/ory/viper"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v2"

	"knative.dev/func/pkg/config"
	fn "knative.dev/func/pkg/functions"
)

func NewListCmd(newClient ClientFactory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List deployed functions",
		Long: `List deployed functions

Lists deployed functions.
`,
		Example: `
# List all functions in the current namespace with human readable output
{{rootCmdUse}} list

# List all functions in the 'test' namespace with yaml output
{{rootCmdUse}} list --namespace test --output yaml

# List all functions in all namespaces with JSON output
{{rootCmdUse}} list --all-namespaces --output json
`,
		SuggestFor: []string{"lsit"},
		Aliases:    []string{"ls"},
		PreRunE:    bindEnv("all-namespaces", "output", "namespace", "verbose"),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runList(cmd, args, newClient)
		},
	}

	cfg, err := config.NewDefault()
	if err != nil {
		fmt.Fprintf(cmd.OutOrStdout(), "error loading config at '%v'. %v\n", config.File(), err)
	}

	// Namespace Config
	// Differing from other commands, the default namespace for the list
	// command is set to the currently active namespace as returned by
	// calling k8s.DefaultNamespace().  This way a call to `func list` will
	// show functions in the currently active namespace.  If the value can
	// not be determined due to error, a warning is printed to log and
	// no namespace is passed to the lister, which should result in the
	// lister showing functions for all namespaces.
	//
	// This also extends to the treatment of the global setting for
	// namespace.  This is likewise intended for command which require a
	// namespace no matter what.  Therefore the global namespace setting is
	// not applicable to this command because "default" really means "all".
	//
	// This is slightly different than other commands wherein their
	// default is often to presume namespace "default" if none was either
	// supplied nor available.

	// Flags
	cmd.Flags().BoolP("all-namespaces", "A", false, "List functions in all namespaces. If set, the --namespace flag is ignored.")
	cmd.Flags().StringP("namespace", "n", defaultNamespace(fn.Function{}, false), "The namespace for which to list functions. ($FUNC_NAMESPACE)")
	cmd.Flags().StringP("output", "o", "human", "Output format (human|plain|json|xml|yaml) ($FUNC_OUTPUT)")
	addVerboseFlag(cmd, cfg.Verbose)

	if err := cmd.RegisterFlagCompletionFunc("output", CompleteOutputFormatList); err != nil {
		fmt.Println("internal: error while calling RegisterFlagCompletionFunc: ", err)
	}

	return cmd
}

func runList(cmd *cobra.Command, _ []string, newClient ClientFactory) (err error) {
	cfg, err := newListConfig(cmd)
	if err != nil {
		return err
	}

	client, done := newClient(ClientConfig{Verbose: cfg.Verbose})
	defer done()

	items, err := client.List(cmd.Context(), cfg.Namespace)
	if err != nil {
		return
	}

	if len(items) == 0 {
		if cfg.Namespace != "" {
			fmt.Printf("no functions found in namespace '%v'\n", cfg.Namespace)
		} else {
			fmt.Println("no functions found")
		}
		return
	}

	write(os.Stdout, listItems(items), cfg.Output)

	return
}

// CLI Configuration (parameters)
// ------------------------------

type listConfig struct {
	Namespace string
	Output    string
	Verbose   bool
}

func newListConfig(cmd *cobra.Command) (cfg listConfig, err error) {
	cfg = listConfig{
		Namespace: viper.GetString("namespace"),
		Output:    viper.GetString("output"),
		Verbose:   viper.GetBool("verbose"),
	}
	// If --all-namespaces, zero out any value for namespace (such as)
	// "all" to the lister.
	if viper.GetBool("all-namespaces") {
		cfg.Namespace = ""
	}

	// specifying both -A and --namespace is logically inconsistent
	if cmd.Flags().Changed("namespace") && viper.GetBool("all-namespaces") {
		err = errors.New("Both --namespace and --all-namespaces specified.")
	}

	return
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

func (items listItems) URL(w io.Writer) error {
	for _, item := range items {
		fmt.Fprintf(w, "%s\n", item.URL)
	}
	return nil
}
