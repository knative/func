package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"text/tabwriter"

	"github.com/ory/viper"
	"github.com/spf13/cobra"

	fn "knative.dev/func"
)

// ErrTemplateRepoDoesNotExist is a sentinel error if a template repository responds with 404 status code
var ErrTemplateRepoDoesNotExist = errors.New("template repo does not exist")

func NewTemplatesCmd(newClient ClientFactory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "templates",
		Short: "Templates",
		Long: `
NAME
	{{rootCmdUse}} templates - list available templates

SYNOPSIS
	{{rootCmdUse}} templates [language] [--json] [-r|--repository]

DESCRIPTION
	List all templates available, optionally for a specific language runtime.

	To specify a URI of a single, specific repository for which templates
	should be displayed, use the --repository flag.

	Installed repositories are by default located at ~/.func/repositories
	($XDG_CONFIG_HOME/.func/repositories).  This can be overridden with
	$FUNC_REPOSITORIES_PATH.

	To see all available language runtimes, see the 'languages' command.


EXAMPLES

	o Show a list of all available templates grouped by language runtime
	  $ {{rootCmdUse}} templates

	o Show a list of all templates for the Go runtime
	  $ {{rootCmdUse}} templates go

	o Return a list of all template runtimes in JSON output format
	  $ {{rootCmdUse}} templates --json

	o Return Go templates in a specific repository
		$ {{rootCmdUse}} templates go --repository=https://github.com/boson-project/templates
`,
		SuggestFor: []string{"template", "templtaes", "templatse", "remplates",
			"gemplates", "yemplates", "tenplates", "tekplates", "tejplates",
			"temolates", "temllates", "temppates", "tempmates", "tempkates",
			"templstes", "templztes", "templqtes", "templares", "templages", //nolint:misspell
			"templayes", "templatee", "templatea", "templated", "templatew"},
		PreRunE: bindEnv("json", "repository"),
	}

	cmd.Flags().Bool("json", false, "Set output to JSON format. (Env: $FUNC_JSON)")
	cmd.Flags().StringP("repository", "r", "", "URI to a specific repository to consider (Env: $FUNC_REPOSITORY)")

	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		return runTemplates(cmd, args, newClient)
	}

	return cmd
}

func runTemplates(cmd *cobra.Command, args []string, newClient ClientFactory) (err error) {
	// Gather config
	cfg, err := newTemplatesConfig(newClient)
	if err != nil {
		return
	}

	// Simple ping to the repo to avoid subsequent errors from http package if it does not exist
	if cfg.Repository != "" {
		res, err := http.Get(cfg.Repository)
		if err != nil {
			return err
		}
		defer res.Body.Close()
		if res.StatusCode == http.StatusNotFound {
			return ErrTemplateRepoDoesNotExist
		}
	}

	// Client which will provide data
	client, done := newClient(
		ClientConfig{Verbose: cfg.Verbose},
		fn.WithRepository(cfg.Repository))
	defer done()

	// For a single language runtime
	// -------------------
	if len(args) == 1 {
		templates, err := client.Templates().List(args[0])
		if err != nil {
			return err
		}
		if cfg.JSON {
			s, err := json.MarshalIndent(templates, "", "  ")
			if err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), string(s))
		} else {
			for _, template := range templates {
				fmt.Fprintln(cmd.OutOrStdout(), template)
			}
		}
		return nil
	} else if len(args) > 1 {
		return errors.New("unexpected extra arguments")
	}

	// All language runtimes
	// ------------
	runtimes, err := client.Runtimes()
	if err != nil {
		return
	}
	if cfg.JSON {
		// Gather into a single data structure for printing as json
		templateMap := make(map[string][]string)
		for _, runtime := range runtimes {
			templates, err := client.Templates().List(runtime)
			if err != nil {
				return err
			}
			templateMap[runtime] = templates
		}
		s, err := json.MarshalIndent(templateMap, "", "  ")
		if err != nil {
			return err
		}
		fmt.Fprintln(cmd.OutOrStdout(), string(s))
	} else {
		// print using a formatted writer (sorted)
		builder := strings.Builder{}
		writer := tabwriter.NewWriter(&builder, 0, 0, 3, ' ', 0)
		fmt.Fprint(writer, "LANGUAGE\tTEMPLATE\n")
		for _, runtime := range runtimes {
			templates, err := client.Templates().List(runtime)
			if err != nil {
				return err
			}
			for _, template := range templates {
				fmt.Fprintf(writer, "%v\t%v\n", runtime, template)
			}
		}
		writer.Flush()
		fmt.Fprint(cmd.OutOrStdout(), builder.String())
	}
	return
}

type templatesConfig struct {
	Verbose    bool
	Repository string // Consider only a specific repository (URI)
	JSON       bool   // output as JSON
}

func newTemplatesConfig(newClient ClientFactory) (cfg templatesConfig, err error) {
	cfg = templatesConfig{
		Verbose:    viper.GetBool("verbose"),
		Repository: viper.GetString("repository"),
		JSON:       viper.GetBool("json"),
	}

	return
}
