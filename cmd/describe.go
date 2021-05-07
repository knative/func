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

	fn "github.com/boson-project/func"
	"github.com/boson-project/func/knative"
)

func init() {
	root.AddCommand(describeCmd)
	describeCmd.Flags().StringP("namespace", "n", "", "Namespace of the function. By default, the namespace in func.yaml is used or the actual active namespace if not set in the configuration. (Env: $FUNC_NAMESPACE)")
	describeCmd.Flags().StringP("output", "o", "human", "Output format (human|plain|json|xml|yaml) (Env: $FUNC_OUTPUT)")
	describeCmd.Flags().StringP("path", "p", cwd(), "Path to the project directory (Env: $FUNC_PATH)")

	err := describeCmd.RegisterFlagCompletionFunc("output", CompleteOutputFormatList)
	if err != nil {
		fmt.Println("internal: error while calling RegisterFlagCompletionFunc: ", err)
	}
}

var describeCmd = &cobra.Command{
	Use:   "describe <name>",
	Short: "Show details of a function",
	Long: `Show details of a function

Prints the name, route and any event subscriptions for a deployed function in
the current directory or from the directory specified with --path.
`,
	Example: `
# Show the details of a function as declared in the local func.yaml
kn func describe

# Show the details of the function in the myotherfunc directory with yaml output
kn func describe --output yaml --path myotherfunc
`,
	SuggestFor:        []string{"desc", "get"},
	ValidArgsFunction: CompleteFunctionList,
	PreRunE:           bindEnv("namespace", "output", "path"),
	RunE:              runDescribe,
}

func runDescribe(cmd *cobra.Command, args []string) (err error) {
	config := newDescribeConfig(args)

	function, err := fn.NewFunction(config.Path)
	if err != nil {
		return
	}

	// Check if the Function has been initialized
	if !function.Initialized() {
		return fmt.Errorf("the given path '%v' does not contain an initialized function", config.Path)
	}

	describer, err := knative.NewDescriber(config.Namespace)
	if err != nil {
		return
	}
	describer.Verbose = config.Verbose

	client := fn.New(
		fn.WithVerbose(config.Verbose),
		fn.WithDescriber(describer))

	d, err := client.Describe(config.Name, config.Path)
	if err != nil {
		return
	}
	d.Image = function.Image

	write(os.Stdout, description(d), config.Output)
	return
}

// CLI Configuration (parameters)
// ------------------------------

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

// Output Formatting (serializers)
// -------------------------------

type description fn.Description

func (d description) Human(w io.Writer) error {
	fmt.Fprintln(w, "Function name:")
	fmt.Fprintf(w, "  %v\n", d.Name)
	fmt.Fprintln(w, "Function is built in image:")
	fmt.Fprintf(w, "  %v\n", d.Image)
	fmt.Fprintln(w, "Function is deployed in namespace:")
	fmt.Fprintf(w, "  %v\n", d.Namespace)
	fmt.Fprintln(w, "Routes:")

	for _, route := range d.Routes {
		fmt.Fprintf(w, "  %v\n", route)
	}

	if len(d.Subscriptions) > 0 {
		fmt.Fprintln(w, "Subscriptions (Source, Type, Broker):")
		for _, s := range d.Subscriptions {
			fmt.Fprintf(w, "  %v %v %v\n", s.Source, s.Type, s.Broker)
		}
	}
	return nil
}

func (d description) Plain(w io.Writer) error {
	fmt.Fprintf(w, "Name %v\n", d.Name)
	fmt.Fprintf(w, "Image %v\n", d.Image)
	fmt.Fprintf(w, "Namespace %v\n", d.Namespace)

	for _, route := range d.Routes {
		fmt.Fprintf(w, "Route %v\n", route)
	}

	if len(d.Subscriptions) > 0 {
		for _, s := range d.Subscriptions {
			fmt.Fprintf(w, "Subscription %v %v %v\n", s.Source, s.Type, s.Broker)
		}
	}
	return nil
}

func (d description) JSON(w io.Writer) error {
	return json.NewEncoder(w).Encode(d)
}

func (d description) XML(w io.Writer) error {
	return xml.NewEncoder(w).Encode(d)
}

func (d description) YAML(w io.Writer) error {
	return yaml.NewEncoder(w).Encode(d)
}
