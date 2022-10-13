package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/ory/viper"
	"github.com/spf13/cobra"

	fn "knative.dev/func"
)

func NewLanguagesCmd(newClient ClientFactory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "languages",
		Short: "List available function language runtimes",
		Long: `
NAME
	{{.Name}} languages - list available language runtimes.

SYNOPSIS
	{{.Name}} languages [--json] [-r|--repository]

DESCRIPTION
	List the language runtimes that are currently available.
	This includes embedded (included) language runtimes as well as any installed
	via the 'repositories add' command.

	To specify a URI of a single, specific repository for which languages
	should be displayed, use the --repository flag.

	Installed repositories are by default located at ~/.func/repositories
	($XDG_CONFIG_HOME/.func/repositories).  This can be overridden with
	$FUNC_REPOSITORIES_PATH.

	To see templates available for a given language, see the 'templates' command.


EXAMPLES

	o Show a list of all available language runtimes
	  $ {{.Name}} languages

	o Return a list of all language runtimes in JSON
	  $ {{.Name}} languages --json

	o Return language runtimes in a specific repository
		$ {{.Name}} languages --repository=https://github.com/boson-project/templates
`,
		SuggestFor: []string{"language", "runtime", "runtimes", "lnaguages", "languagse",
			"panguages", "manguages", "kanguages", "lsnguages", "lznguages"},
		PreRunE: bindEnv("json", "repository"),
	}

	cmd.Flags().BoolP("json", "", false, "Set output to JSON format. (Env: $FUNC_JSON)")
	cmd.Flags().StringP("repository", "r", "", "URI to a specific repository to consider (Env: $FUNC_REPOSITORY)")

	cmd.SetHelpFunc(defaultTemplatedHelp)

	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		return runLanguages(cmd, args, newClient)
	}

	return cmd
}

func runLanguages(cmd *cobra.Command, args []string, newClient ClientFactory) (err error) {
	cfg, err := newLanguagesConfig(newClient)
	if err != nil {
		return
	}

	client, done := newClient(ClientConfig{Verbose: cfg.Verbose},
		fn.WithRepository(cfg.Repository),             // Use exactly this repo OR
		fn.WithRepositoriesPath(cfg.RepositoriesPath)) // Path on disk to installed repos
	defer done()

	runtimes, err := client.Runtimes()
	if err != nil {
		return
	}

	if cfg.JSON {
		var s []byte
		s, err = json.MarshalIndent(runtimes, "", "  ")
		if err != nil {
			return
		}
		fmt.Fprintln(cmd.OutOrStdout(), string(s))
	} else {
		for _, runtime := range runtimes {
			fmt.Fprintln(cmd.OutOrStdout(), runtime)
		}
	}
	return
}

type languagesConfig struct {
	Verbose          bool
	Repository       string // Consider only a specific repository (URI)
	RepositoriesPath string // Override location on disk of "installed" repos
	JSON             bool   // output as JSON
}

func newLanguagesConfig(newClient ClientFactory) (cfg languagesConfig, err error) {
	// Repositories Path
	// Not exposed as a flag due to potential confusion with the more likely
	// "repository" flag, but still available as an environment variable
	repositoriesPath := os.Getenv("FUNC_REPOSITORIES_PATH")
	if repositoriesPath == "" { // if no env var provided
		repositoriesPath = fn.New().RepositoriesPath() // default to ~/.config/func/repositories
	}

	cfg = languagesConfig{
		Verbose:          viper.GetBool("verbose"),
		Repository:       viper.GetString("repository"),
		RepositoriesPath: repositoriesPath,
		JSON:             viper.GetBool("json"),
	}

	return
}
