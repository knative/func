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
	cmd.Flags().StringP("namespace", "n", cfg.Namespace, "The namespace in which to look for the named function. ($FUNC_NAMESPACE)")
	addPathFlag(cmd)
	addVerboseFlag(cmd, cfg.Verbose)

	if err := cmd.RegisterFlagCompletionFunc("output", CompleteOutputFormatList); err != nil {
		fmt.Println("internal: error while calling RegisterFlagCompletionFunc: ", err)
	}

	return cmd
}

func runDescribe(cmd *cobra.Command, args []string, newClient ClientFactory) (err error) {
	cfg := newDescribeConfig(args)

	if err = cfg.Validate(cmd); err != nil {
		return
	}

	var f fn.Function

	if cfg.Name == "" {
		if f, err = fn.NewFunction(cfg.Path); err != nil {
			return
		}
		if !f.Initialized() {
			return fmt.Errorf("the given path '%v' does not contain an initialized function.", cfg.Path)
		}
		// Use Function's Namespace with precedence
		//
		// Unless the namespace flag was explicitly provided (not the default),
		// use the function's current namespace.
		//
		// TODO(lkingland): this stanza can be removed when Global Config: Function
		// Context is merged.
		if !cmd.Flags().Changed("namespace") {
			cfg.Namespace = f.Deploy.Namespace
		}
	}

	client, done := newClient(ClientConfig{Namespace: cfg.Namespace, Verbose: cfg.Verbose})
	defer done()

	// TODO(lkingland): update API to use the above function instance rather than path
	d, err := client.Describe(cmd.Context(), cfg.Name, f)
	if err != nil {
		return
	}

	write(os.Stdout, info(d), cfg.Output)
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
	c := describeConfig{
		Namespace: viper.GetString("namespace"),
		Output:    viper.GetString("output"),
		Path:      viper.GetString("path"),
		Verbose:   viper.GetBool("verbose"),
	}
	if len(args) > 0 {
		c.Name = args[0]
	}
	return c
}

func (c describeConfig) Validate(cmd *cobra.Command) (err error) {
	if c.Name != "" && c.Path != "" && cmd.Flags().Changed("path") {
		return fmt.Errorf("Only one of --path or [NAME] should be provided")
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
