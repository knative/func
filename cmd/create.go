package cmd

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/AlecAivazis/survey/v2"
	"github.com/ory/viper"
	"github.com/spf13/cobra"

	fn "knative.dev/kn-plugin-func"
	"knative.dev/kn-plugin-func/utils"
)

func init() {
	// Add to the root a new "Create" command which obtains an appropriate
	// instance of fn.Client from the given client creator function.
	root.AddCommand(NewCreateCmd(newCreateClient))
}

// createClientFn is a factory function which returns a Client suitable for
// use with the Create command.
type createClientFn func(createConfig) (*fn.Client, error)

// newCreateClient returns an instance of fn.Client for the "Create" command.
// The createClientFn is a client factory which creates a new Client for use by
// the create command during normal execution (see tests for alternative client
// factories which return clients with various mocks).
func newCreateClient(cfg createConfig) (*fn.Client, error) {
	client := fn.New(
		fn.WithRepositories(cfg.Repositories), // path to repositories in disk
		fn.WithRepository(cfg.Repository),     // URI of repository override
		fn.WithVerbose(cfg.Verbose))           // verbose logging
	return client, nil
}

// NewCreateCmd creates a create command using the given client creator.
func NewCreateCmd(clientFn createClientFn) *cobra.Command {
	cmd := &cobra.Command{
		Short: "Create a Function Project",
		Use:   "create",
		Long: `
NAME
	func-create - Create a Function project.

SYNOPSIS
	func create [-l|--language] [-t|--template] [-r|--repository]
	             [-c|--confirm]  [-v|--verbose] [path]

DESCRIPTION
	Creates a new Function project.

	  $ func create -l go -t http

	Creates a Function in the current directory '.' which is written in the
	language/runtime 'Go' and handles HTTP events.

	If [path] is provided, the Function is initialized at that path, creating
	if necessary.

	To complete this command interactivly, use --confirm (-c):
	  $ func create -c

	Language Runtime and Templates Currently Installed
	Language Runtime  |  Template
	// Coming Soon

	To install more language runtimes and their templates see 'func-repository'.

EXAMPLES
	o Create a Node.js Function (the default language runtime) in the current
	  directory (the default path) which handles http events (the default
	  template).
	  $ func create

	o Create a Node.js Function in the directory 'myfunc'.
	  $ func create myfunc

	o Create a Go Function which handles Cloud Events in ./myfunc.
	  $ func create -l go -t events myfunc
`,
		SuggestFor: []string{"vreate", "creaet", "craete", "new"},
		PreRunE:    bindEnv("language", "template", "repository", "confirm"),
	}

	// Flags
	cmd.Flags().StringP("language", "l", fn.DefaultLanguage, "Language Runtime (see help text for list) (Env: $FUNC_RUNTIME)")
	cmd.Flags().StringP("template", "t", fn.DefaultTemplate, "Function template. (see help text for list) (Env: $FUNC_TEMPLATE)")
	cmd.Flags().StringP("repository", "r", "", "URI to a Git repository containing the specified template (Env: $FUNC_REPOSITORY)")
	cmd.Flags().BoolP("confirm", "c", false, "Prompt to confirm all options interactively (Env: $FUNC_CONFIRM)")

	// Tab Complition
	if err := cmd.RegisterFlagCompletionFunc("language", CompleteLanguageRuntimeList); err != nil {
		fmt.Fprintf(os.Stderr, "unable to provide runtime suggestions: %v", err)
	}
	if err := cmd.RegisterFlagCompletionFunc("template", CompleteTemplateList); err != nil {
		fmt.Fprintf(os.Stderr, "unable to provide template suggestions: %v", err)
	}

	// Command Action
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		return runCreate(cmd, args, clientFn)
	}

	return cmd
}

// Run
func runCreate(cmd *cobra.Command, args []string, clientFn createClientFn) (err error) {
	// Config
	// Create a config based on args.  Also uses the clientFn to create a
	// temporary client for completing options such as available runtimes.
	cfg, err := newCreateConfig(args, clientFn)
	if err != nil {
		return
	}

	// Config and Client
	// From environment variables, flags, arguments, and user prompts if --confirm
	// (in increasing levels of precidence)
	client, err := clientFn(cfg)
	if err != nil {
		return
	}

	// Validate
	if err = cfg.Validate(); err != nil {
		return
	}

	// Create
	return client.Create(fn.Function{
		Root:     cfg.Path,
		Runtime:  cfg.Language,
		Template: cfg.Template,
	})
}

type createConfig struct {
	Path       string // Absolute path to Function sourcsource
	Language   string // Language Runtime
	Repository string // Repository URI (overrides builtin and installed)
	Verbose    bool   // Verbose output
	Confirm    bool   // Confirm values via an interactive prompt

	// Repositories is an optional path that, if it exists, will be used as a source
	// for additional template repositories not included in the binary.  provided via
	// env (FUNC_REPOSITORIES), the default location is $XDG_CONFIG_HOME/repositories
	// ($HOME/.config/func/repositories)
	Repositories string

	// Template is the code written into the new Function project, including
	// an implementation adhering to one of the supported function signatures.
	// May also include additional configuration settings or examples.
	// For example, embedded are 'http' for a Function whose function signature
	// is invoked via straight HTTP requests, or 'events' for a Function which
	// will be invoked with CloudEvents.  These embedded templates contain a
	// minimum implementation of the signature itself and example tests.
	Template string

	// Name of the Function
	// Not prominently highlighted in help text or confirmation statements as it
	// is likely this will be refactored away to be part of the Deploy step, in
	// a git-like flow where it is not required to create, but is part of a named
	// remote for a one-to-many replationship.
	Name string
}

// newCreateConfig returns a config populated from the current execution context
// (args, flags and environment variables)
// The client constructor function is used to create a transient client for
// accessing things like the current valid templates list, and uses the
// current value of the config at time of prompting.
func newCreateConfig(args []string, clientFn createClientFn) (cfg createConfig, err error) {
	var (
		path         string
		dirName      string
		absolutePath string
		repositories string
	)

	if len(args) == 1 {
		path = args[0]
	} else if len(args) > 1 {
		return cfg, errors.New("too many arguments.  usage: func create [flags] [path]")
	}

	// Convert the path to an absolute path, and extract the ending directory name
	// as the function name. TODO: refactor to be git-like with no name up-front
	// and set instead as a named one-to-many deploy target.
	dirName, absolutePath = deriveNameAndAbsolutePathFromPath(path)

	// Repositories Path
	// Not exposed as a flag due to potential confusion with the more likely
	// "repository override" flag, and due to its unlikliness of being needed, but
	// it is still available as an environment variable.
	repositories = os.Getenv("FUNC_REPOSITORIES")
	if repositories == "" { // if no env var provided
		repositories = repositoriesPath() // use ~/.config/func/repositories
	}

	// Config is the final default values based off the execution context.
	// When prompting, these become the defaults presented.
	cfg = createConfig{
		Name:         dirName, // TODO: refactor to be git-like
		Path:         absolutePath,
		Repositories: repositories,
		Repository:   viper.GetString("repository"),
		Language:     viper.GetString("language"),
		Template:     viper.GetString("template"),
		Confirm:      viper.GetBool("confirm"),
		Verbose:      viper.GetBool("verbose"),
	}

	// If not in confirm/prompting mode, this cfg structure is complete.
	if !cfg.Confirm {
		return
	}

	// Create a tempoarary client for use by the following prompts to complete
	// runtime/template suggestions etc
	client, err := clientFn(cfg)
	if err != nil {
		return
	}

	// IN confirm mode.  If also in an interactive terminal, run prompts.
	if interactiveTerminal() {
		return cfg.prompt(client)
	}

	// Confirming, but noninteractive
	// Print out the final values as a confirmation.  Only show Repository or
	// Repositories, not both (repository takes precidence) in order to avoid
	// likely confusion if both are displayed and one is empty.
	// be removed and both displayed.
	fmt.Printf("Path:         %v\n", cfg.Path)
	fmt.Printf("Language:     %v\n", cfg.Language)
	if cfg.Repository != "" { // if an override was provided
		fmt.Printf("Repository:   %v\n", cfg.Repository) // show only the override
	} else {
		fmt.Printf("Repositories: %v\n", cfg.Repositories) // or path to installed
	}
	fmt.Printf("Template:     %v\n", cfg.Template)
	return
}

// Validate the current state of the config, returning any errors.
func (c createConfig) Validate() (err error) {
	// TODO: refactor to be git-like with no name at time of creation, but rather
	// with named deployment targets in a one-to-many configuration.
	dirName, _ := deriveNameAndAbsolutePathFromPath(c.Path)
	return utils.ValidateFunctionName(dirName)
	// NOTE: perhaps additional validation would be of use here in the CLI, but
	// the client libray itself is ultimately responsible for validating all input
	// prior to exeuting any requests.
}

// prompt the user with value of config members, allowing for interactively
// mutating the values. The provided clientFn is used to construct a transient
// client for use during prompt autocompletion/suggestions (such as suggesting
// valid templates)
func (c createConfig) prompt(client *fn.Client) (createConfig, error) {
	var qs []*survey.Question

	runtimes, err := client.Runtimes()
	if err != nil {
		return createConfig{}, err
	}

	// First ask for path...
	qs = []*survey.Question{
		{
			Name: "path",
			Prompt: &survey.Input{
				Message: "Function Path:",
				Default: c.Path,
			},
			Validate: func(val interface{}) error {
				derivedName, _ := deriveNameAndAbsolutePathFromPath(val.(string))
				return utils.ValidateFunctionName(derivedName)
			},
			Transform: func(ans interface{}) interface{} {
				_, absolutePath := deriveNameAndAbsolutePathFromPath(ans.(string))
				return absolutePath
			},
		}, {
			Name: "language",
			Prompt: &survey.Select{
				Message: "Language Runtime:",
				Options: runtimes,
				Default: c.Language,
			},
		}}
	if err := survey.Ask(qs, &c); err != nil {
		return c, err
	}

	// Second loop: choose template with autocompletion filtered by chosen runtime
	qs = []*survey.Question{
		{
			Name: "template",
			Prompt: &survey.Input{
				Message: "Template:",
				Default: c.Template,
				Suggest: func(prefix string) []string {
					suggestions, err := templatesWithPrefix(prefix, c.Language, client)
					if err != nil {
						fmt.Fprintf(os.Stderr, "unable to suggest: %v", err)
					}
					return suggestions
				},
			},
		},
	}
	if err := survey.Ask(qs, &c); err != nil {
		return c, err
	}

	return c, nil
}

// return templates for language runtime whose full name (including repository)
// have the given prefix.
func templatesWithPrefix(prefix, language string, client *fn.Client) ([]string, error) {
	var (
		suggestions    = []string{}
		templates, err = client.Templates().List(language)
	)
	if err != nil {
		return suggestions, err
	}
	for _, template := range templates {
		if strings.HasPrefix(template, prefix) {
			suggestions = append(suggestions, template)
		}
	}
	return suggestions, nil
}
