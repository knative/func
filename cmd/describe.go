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
	root.AddCommand(describeCmd)
	describeCmd.Flags().StringP("namespace", "n", "", "Override namespace in which to search for the Function.  Default is to use currently active underlying platform setting - $FAAS_NAMESPACE")
	describeCmd.Flags().StringP("format", "f", "human", "optionally specify output format (human|plain|json|xml|yaml) $FAAS_FORMAT")
	describeCmd.Flags().StringP("path", "p", cwd(), "Path to the project which should be described - $FAAS_PATH")

	err := describeCmd.RegisterFlagCompletionFunc("format", CompleteOutputFormatList)
	if err != nil {
		fmt.Println("Error while calling RegisterFlagCompletionFunc: ", err)
	}
}

var describeCmd = &cobra.Command{
	Use:               "describe <name> [options]",
	Short:             "Describe Function",
	Long:              `Describes the Function initialized in the current directory, or by passed name argument.`,
	SuggestFor:        []string{"desc", "get"},
	ValidArgsFunction: CompleteFunctionList,
	PreRunE:           bindEnv("namespace", "format", "path"),
	RunE:              runDescribe,
}

func runDescribe(cmd *cobra.Command, args []string) (err error) {
	config := newDescribeConfig(args)

	describer, err := knative.NewDescriber(config.Namespace)
	if err != nil {
		return
	}
	describer.Verbose = config.Verbose

	client := faas.New(
		faas.WithVerbose(config.Verbose),
		faas.WithDescriber(describer))

	d, err := client.Describe(config.Name, config.Path)
	if err != nil {
		return
	}

	write(os.Stdout, description(d), config.Format)
	return
}

// CLI Configuration (parameters)
// ------------------------------

type describeConfig struct {
	Name      string
	Namespace string
	Format    string
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
		Format:    viper.GetString("format"),
		Path:      viper.GetString("path"),
		Verbose:   viper.GetBool("verbose"),
	}
}

// Output Formatting (serializers)
// -------------------------------

type description faas.Description

func (d description) Human(w io.Writer) error {
	fmt.Fprintln(w, d.Name)
	fmt.Fprintln(w, "Routes:")
	for _, route := range d.Routes {
		fmt.Fprintf(w, "  %v\n", route)
	}
	fmt.Fprintln(w, "Subscriptions (Source, Type, Broker):")
	for _, s := range d.Subscriptions {
		fmt.Fprintf(w, "  %v %v %v\n", s.Source, s.Type, s.Broker)
	}
	return d.Plain(w)
}

func (d description) Plain(w io.Writer) error {
	fmt.Fprintf(w, "NAME %v\n", d.Name)
	for _, route := range d.Routes {
		fmt.Fprintf(w, "ROUTE %v\n", route)
	}
	for _, s := range d.Subscriptions {
		fmt.Fprintf(w, "SUBSCRIPTION %v %v %v\n", s.Source, s.Type, s.Broker)
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
