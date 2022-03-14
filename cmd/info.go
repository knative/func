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

	fn "knative.dev/kn-plugin-func"
)

func NewInfoCmd(newClient ClientFactory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "info <name>",
		Short: "Show details of a function",
		Long: `Show details of a function

Prints the name, route and any event subscriptions for a deployed function in
the current directory or from the directory specified with --path.
`,
		Example: `
# Show the details of a function as declared in the local func.yaml
{{.Name}} info

# Show the details of the function in the myotherfunc directory with yaml output
{{.Name}} info --output yaml --path myotherfunc
`,
		SuggestFor:        []string{"ifno", "describe", "fino", "get"},
		ValidArgsFunction: CompleteFunctionList,
		PreRunE:           bindEnv("output", "path"),
	}

	cmd.Flags().StringP("output", "o", "human", "Output format (human|plain|json|xml|yaml|url) (Env: $FUNC_OUTPUT)")
	setPathFlag(cmd)

	if err := cmd.RegisterFlagCompletionFunc("output", CompleteOutputFormatList); err != nil {
		fmt.Println("internal: error while calling RegisterFlagCompletionFunc: ", err)
	}

	cmd.SetHelpFunc(defaultTemplatedHelp)

	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		return runInfo(cmd, args, newClient)
	}

	return cmd
}

func runInfo(cmd *cobra.Command, args []string, newClient ClientFactory) (err error) {
	config := newInfoConfig(args)

	function, err := fn.NewFunction(config.Path)
	if err != nil {
		return
	}

	// Check if the Function has been initialized
	if !function.Initialized() {
		return fmt.Errorf("the given path '%v' does not contain an initialized function", config.Path)
	}

	// Create a client
	client, done := newClient(ClientConfig{Namespace: config.Namespace, Verbose: config.Verbose})
	defer done()

	// Get the description
	d, err := client.Info(cmd.Context(), config.Name, config.Path)
	if err != nil {
		return
	}
	d.Image = function.Image

	write(os.Stdout, info(d), config.Output)
	return
}

// CLI Configuration (parameters)
// ------------------------------

type infoConfig struct {
	Name      string
	Namespace string
	Output    string
	Path      string
	Verbose   bool
}

func newInfoConfig(args []string) infoConfig {
	var name string
	if len(args) > 0 {
		name = args[0]
	}
	return infoConfig{
		Name:      deriveName(name, viper.GetString("path")),
		Namespace: viper.GetString("namespace"),
		Output:    viper.GetString("output"),
		Path:      viper.GetString("path"),
		Verbose:   viper.GetBool("verbose"),
	}
}

// Output Formatting (serializers)
// -------------------------------

type info fn.Instance

func (i info) Human(w io.Writer) error {
	fmt.Fprintln(w, "Function name:")
	fmt.Fprintf(w, "  %v\n", i.Name)
	fmt.Fprintln(w, "Function is built in image:")
	fmt.Fprintf(w, "  %v\n", i.Image)
	fmt.Fprintln(w, "Function is deployed in namespace:")
	fmt.Fprintf(w, "  %v\n", i.Namespace)
	fmt.Fprintln(w, "Routes:")

	for _, route := range i.Routes {
		fmt.Fprintf(w, "  %v\n", route)
	}

	if len(i.Subscriptions) > 0 {
		fmt.Fprintln(w, "Subscriptions (Source, Type, Broker):")
		for _, s := range i.Subscriptions {
			fmt.Fprintf(w, "  %v %v %v\n", s.Source, s.Type, s.Broker)
		}
	}
	return nil
}

func (i info) Plain(w io.Writer) error {
	fmt.Fprintf(w, "Name %v\n", i.Name)
	fmt.Fprintf(w, "Image %v\n", i.Image)
	fmt.Fprintf(w, "Namespace %v\n", i.Namespace)

	for _, route := range i.Routes {
		fmt.Fprintf(w, "Route %v\n", route)
	}

	if len(i.Subscriptions) > 0 {
		for _, s := range i.Subscriptions {
			fmt.Fprintf(w, "Subscription %v %v %v\n", s.Source, s.Type, s.Broker)
		}
	}
	return nil
}

func (i info) JSON(w io.Writer) error {
	return json.NewEncoder(w).Encode(i)
}

func (i info) XML(w io.Writer) error {
	return xml.NewEncoder(w).Encode(i)
}

func (i info) YAML(w io.Writer) error {
	return yaml.NewEncoder(w).Encode(i)
}

func (i info) URL(w io.Writer) error {
	if len(i.Routes) > 0 {
		fmt.Fprintf(w, "%s\n", i.Routes[0])
	}
	return nil
}
