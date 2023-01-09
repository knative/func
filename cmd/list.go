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

	fn "knative.dev/func"
	"knative.dev/func/config"
)

func NewListCmd(newClient ClientFactory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List functions",
		Long: `List functions

Lists all deployed functions in a given namespace.
`,
		Example: `
# List all functions in the current namespace with human readable output
{{rootCmdUse}} list

# List all functions in the 'test' namespace with yaml output
{{rootCmdUse}} list --namespace test --output yaml

# List all functions in all namespaces with JSON output
{{rootCmdUse}} list --all-namespaces --output json
`,
		SuggestFor: []string{"ls", "lsit"},
		PreRunE:    bindEnv("all-namespaces", "output", "namespace"),
	}

	// Config
	cfg, err := config.NewDefault()
	if err != nil {
		fmt.Fprintf(cmd.OutOrStdout(), "error loading config at '%v'. %v\n", config.File(), err)
	}

	// Flags
	cmd.Flags().BoolP("all-namespaces", "A", false, "List functions in all namespaces. If set, the --namespace flag is ignored.")
	cmd.Flags().StringP("namespace", "n", cfg.Namespace, "The namespace for which to list functions. (Env: $FUNC_NAMESPACE)")
	cmd.Flags().StringP("output", "o", "human", "Output format (human|plain|json|xml|yaml) (Env: $FUNC_OUTPUT)")

	if err := cmd.RegisterFlagCompletionFunc("output", CompleteOutputFormatList); err != nil {
		fmt.Println("internal: error while calling RegisterFlagCompletionFunc: ", err)
	}

	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		return runList(cmd, args, newClient)
	}

	return cmd
}

func runList(cmd *cobra.Command, _ []string, newClient ClientFactory) (err error) {
	cfg := newListConfig()

	if err := cfg.Validate(cmd); err != nil {
		return err
	}

	client, done := newClient(ClientConfig{Namespace: cfg.Namespace, Verbose: cfg.Verbose})
	defer done()

	items, err := client.List(cmd.Context())
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

func newListConfig() listConfig {
	c := listConfig{
		Namespace: viper.GetString("namespace"),
		Output:    viper.GetString("output"),
		Verbose:   viper.GetBool("verbose"),
	}
	// Lister instantiated by newClient explicitly expects "" namespace to
	// inidicate it should list from all namespaces, so remove default "default"
	// when -A.
	if viper.GetBool("all-namespaces") {
		c.Namespace = ""
	}
	return c
}

func (c listConfig) Validate(cmd *cobra.Command) error {
	if cmd.Flags().Changed("namespace") && viper.GetBool("all-namespaces") {
		return errors.New("Both --namespace and --all-namespaces specified.")
	}
	return nil
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
