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

	"knative.dev/func/pkg/config"
	fn "knative.dev/func/pkg/functions"
)

func NewDescribeCmd(newClient ClientFactory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "describe <name>",
		Short: "Describe a function",
		Long: `Describe a function

Prints the name, route and event subscriptions for a deployed function in
the current directory or from the directory specified with --path.
`,
		Example: `
# Show the details of a function as declared in the local func.yaml
{{rootCmdUse}} describe

# Show the details of the function in the directory with yaml output
{{rootCmdUse}} describe --output yaml --path myotherfunc
`,
		SuggestFor: []string{"ifno", "fino", "get"},

		ValidArgsFunction: CompleteFunctionList,
		Aliases:           []string{"info", "desc"},
		PreRunE:           bindEnv("output", "path", "namespace", "verbose"),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDescribe(cmd, args, newClient)
		},
	}

	// Config
	cfg, err := config.NewDefault()
	if err != nil {
		fmt.Fprintf(cmd.OutOrStdout(), "error loading config at '%v'. %v\n", config.File(), err)
	}

	// Flags
	cmd.Flags().StringP("output", "o", "human", "Output format (human|plain|json|xml|yaml|url) ($FUNC_OUTPUT)")
	cmd.Flags().StringP("namespace", "n", defaultNamespace(fn.Function{}, false), "The namespace in which to look for the named function. ($FUNC_NAMESPACE)")
	addPathFlag(cmd)
	addVerboseFlag(cmd, cfg.Verbose)

	if err := cmd.RegisterFlagCompletionFunc("output", CompleteOutputFormatList); err != nil {
		fmt.Println("internal: error while calling RegisterFlagCompletionFunc: ", err)
	}

	return cmd
}

func runDescribe(cmd *cobra.Command, args []string, newClient ClientFactory) (err error) {
	cfg, err := newDescribeConfig(cmd, args)
	if err != nil {
		return
	}
	// TODO cfg.Prompt()

	client, done := newClient(ClientConfig{Verbose: cfg.Verbose})
	defer done()

	var details fn.Instance
	if cfg.Name != "" { // Describe by name if provided
		details, err = client.Describe(cmd.Context(), cfg.Name, cfg.Namespace, fn.Function{})
		if err != nil {
			return err
		}
	} else {
		f, err := fn.NewFunction(cfg.Path)
		if err != nil {
			return err
		}
		details, err = client.Describe(cmd.Context(), "", "", f)
		if err != nil {
			return err
		}
	}

	write(os.Stdout, info(details), cfg.Output)
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

func newDescribeConfig(cmd *cobra.Command, args []string) (cfg describeConfig, err error) {
	var name string
	if len(args) > 0 {
		name = args[0]
	}
	cfg = describeConfig{
		Name:      name,
		Namespace: viper.GetString("namespace"),
		Output:    viper.GetString("output"),
		Path:      viper.GetString("path"),
		Verbose:   viper.GetBool("verbose"),
	}
	if cfg.Name == "" && cmd.Flags().Changed("namespace") {
		// logicially inconsistent to supply only a namespace.
		// Either use the function's local state in its entirety, or specify
		// both a name and a namespace to ignore any local function source.
		err = fmt.Errorf("must also specify a name when specifying namespace.")
	}
	if cfg.Name != "" && cmd.Flags().Changed("path") {
		// logically inconsistent to provide both a name and a path to source.
		// Either use the function's local state on disk (--path), or specify
		// a name and a namespace to ignore any local function source.
		err = fmt.Errorf("only one of --path and [NAME] should be provided")
	}
	return
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
