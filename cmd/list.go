package cmd

import (
	"encoding/json"
	"fmt"
	"gopkg.in/yaml.v2"
	"io"
	"os"

	"github.com/boson-project/faas"
	"github.com/boson-project/faas/knative"
	"github.com/spf13/cobra"
)

var formats = map[string]fmtFn{
	"plain": fmtPlain,
	"json":  fmtJSON,
	"yaml":  fmtYAML,
}

var validFormats []string

func completeFormats(cmd *cobra.Command, args []string, toComplete string) (formats []string, directive cobra.ShellCompDirective) {
	formats = validFormats
	directive = cobra.ShellCompDirectiveDefault
	return
}

func init() {
	root.AddCommand(listCmd)

	validFormats = make([]string, 0, len(formats))
	for name := range formats {
		validFormats = append(validFormats, name)
	}

	listCmd.Flags().StringP("namespace", "s", "", "cluster namespace to list functions from")
	listCmd.Flags().StringP("output", "o", "plain", "optionally specify output format (plain,json,yaml)")
	err := listCmd.RegisterFlagCompletionFunc("output", completeFormats)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
	}
}

var listCmd = &cobra.Command{
	Use:        "list",
	Short:      "Lists deployed Functions",
	Long:       `Lists deployed Functions`,
	SuggestFor: []string{"ls"},
	RunE:       list,
}

type fmtFn func(writer io.Writer, names []string) error

func fmtPlain(writer io.Writer, names []string) error {
	for _, name := range names {
		_, err := fmt.Fprintf(writer, "%s\n", name)
		if err != nil {
			return err
		}
	}
	return nil
}

func fmtJSON(writer io.Writer, names []string) error {
	encoder := json.NewEncoder(writer)
	return encoder.Encode(names)
}

func fmtYAML(writer io.Writer, names []string) error {
	encoder := yaml.NewEncoder(writer)
	return encoder.Encode(names)
}

func list(cmd *cobra.Command, args []string) (err error) {

	verbose, err := cmd.Flags().GetBool("verbose")
	if err != nil {
		return
	}

	format, err := cmd.Flags().GetString("output")
	if err != nil {
		return
	}

	namespace, err := cmd.Flags().GetString("namespace")
	if err != nil {
		return
	}

	lister, err := knative.NewLister(namespace)
	if err != nil {
		return
	}
	lister.Verbose = verbose

	client, err := faas.New(
		faas.WithVerbose(verbose),
		faas.WithLister(lister),
	)
	if err != nil {
		return
	}

	names, err := client.List()
	if err != nil {
		return
	}

	fmtFn, ok := formats[format]
	if !ok {
		return fmt.Errorf("invalid format name: %s", format)
	}

	return fmtFn(os.Stdout, names)
}
