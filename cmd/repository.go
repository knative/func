package cmd

import (
	"errors"
	"fmt"
	"os"

	"github.com/AlecAivazis/survey/v2"
	"github.com/ory/viper"
	"github.com/spf13/cobra"

	fn "knative.dev/kn-plugin-func"
)

// command constructors
// --------------------

func NewRepositoryCmd(newClient ClientFactory) *cobra.Command {
	cmd := &cobra.Command{
		Short:   "Manage installed template repositories",
		Use:     "repository",
		Aliases: []string{"repo", "repositories"},
		Long: `
NAME
	{{.Name}} - Manage set of installed repositories.

SYNOPSIS
	{{.Name}} repo [-c|--confirm] [-v|--verbose]
	{{.Name}} repo list [-r|--repositories] [-c|--confirm] [-v|--verbose]
	{{.Name}} repo add <name> <url>[-r|--repositories] [-c|--confirm] [-v|--verbose]
	{{.Name}} repo rename <old> <new> [-r|--repositories] [-c|--confirm] [-v|--verbose]
	{{.Name}} repo remove <name> [-r|--repositories] [-c|--confirm] [-v|--verbose]

DESCRIPTION
	Manage template repositories installed on disk at either the default location
	(~/.config/func/repositories) or the location specified by the --repository
	flag.  Once added, a template from the repository can be used when creating
	a new function.

	Interactive Prompts:
	To complete these commands interactively, pass the --confirm (-c) flag to
	the 'repository' command, or any of the inidivual subcommands.

	The Default Repository:
	The default repository is not stored on disk, but embedded in the binary and
	can be used without explicitly specifying the name.  The default repository
	is always listed first, and is assumed when creating a new function without
	specifying a repository name prefix.
	For example, to create a new Go function using the 'http' template from the
	default repository.
		$ {{.Name}} create -l go -t http

	The Repository Flag:
	Installing repositories locally is optional.  To use a template from a remote
	repository directly, it is possible to use the --repository flag on create.
	This leaves the local disk untouched.  For example, To create a function using
	the Boson Project Hello-World template without installing the template
	repository locally, use the --repository (-r) flag on create:
		$ {{.Name}} create -l go \
			--template hello-world \
			--repository https://github.com/boson-project/templates

	Alternative Repositories Location:
	Repositories are stored on disk in ~/.config/func/repositories by default.
	This location can be altered by setting the FUNC_REPOSITORIES_PATH
	environment variable.


COMMANDS

	With no arguments, this help text is shown.  To manage repositories with
	an interactive prompt, use the use the --confirm (-c) flag.
	  $ {{.Name}} repository -c

	add
	  Add a new repository to the installed set.
	    $ {{.Name}} repository add <name> <URL>

	  For Example, to add the Boson Project repository:
	    $ {{.Name}} repository add boson https://github.com/boson-project/templates

	  Once added, a function can be created with templates from the new repository
	  by prefixing the template name with the repository.  For example, to create
	  a new function using the Go Hello World template:
	    $ {{.Name}} create -l go -t boson/hello-world

	list
	  List all available repositories, including the installed default
	  repository.  Repositories available are listed by name.  To see the URL
	  which was used to install remotes, use --verbose (-v).

	rename
	  Rename a previously installed repository from <old> to <new>. Only installed
	  repositories can be renamed.
	    $ {{.Name}} repository rename <name> <new name>

	remove
	  Remove a repository by name.  Removes the repository from local storage
	  entirely.  When in confirm mode (--confirm) it will confirm before
	  deletion, but in regular mode this is done immediately, so please use
	  caution, especially when using an altered repositories location
	  (via the FUNC_REPOSITORIES_PATH environment variable).
	    $ {{.Name}} repository remove <name>

EXAMPLES
	o Run in confirmation mode (interactive prompts) using the --confirm flag
	  $ {{.Name}} repository -c

	o Add a repository and create a new function using a template from it:
	  $ {{.Name}} repository add boson https://github.com/boson-project/templates
	  $ {{.Name}} repository list
	  default
	  boson
	  $ {{.Name}} create -l go -t boson/hello-world
	  ...

	o List all repositories including the URL from which remotes were installed
	  $ {{.Name}} repository list -v
	  default
	  boson	https://github.com/boson-project/templates

	o Rename an installed repository
	  $ {{.Name}} repository list
	  default
	  boson
	  $ {{.Name}} repository rename boson boson-examples
	  $ {{.Name}} repository list
	  default
	  boson-examples

	o Remove an installed repository
	  $ {{.Name}} repository list
	  default
	  boson-examples
	  $ {{.Name}} repository remove boson-examples
	  $ {{.Name}} repository list
	  default
`,
		SuggestFor: []string{"repositories", "repos", "template", "templates", "pack", "packs"},
		PreRunE:    bindEnv("confirm"),
	}

	cmd.Flags().BoolP("confirm", "c", false, "Prompt to confirm all options interactively (Env: $FUNC_CONFIRM)")

	cmd.SetHelpFunc(defaultTemplatedHelp)

	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		return runRepository(cmd, args, newClient)
	}

	cmd.AddCommand(NewRepositoryListCmd(newClient))
	cmd.AddCommand(NewRepositoryAddCmd(newClient))
	cmd.AddCommand(NewRepositoryRenameCmd(newClient))
	cmd.AddCommand(NewRepositoryRemoveCmd(newClient))

	return cmd
}

func NewRepositoryListCmd(newClient ClientFactory) *cobra.Command {
	cmd := &cobra.Command{
		Short: "List repositories",
		Use:   "list",
	}

	cmd.RunE = func(_ *cobra.Command, args []string) error {
		return runRepositoryList(cmd, args, newClient)
	}

	return cmd
}

func NewRepositoryAddCmd(newClient ClientFactory) *cobra.Command {
	cmd := &cobra.Command{
		Short:      "Add a repository",
		Use:        "add <name> <url>",
		SuggestFor: []string{"ad", "install"},
		PreRunE:    bindEnv("confirm"),
	}

	cmd.Flags().BoolP("confirm", "c", false, "Prompt to confirm all options interactively (Env: $FUNC_CONFIRM)")

	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		return runRepositoryAdd(cmd, args, newClient)
	}

	return cmd
}

func NewRepositoryRenameCmd(newClient ClientFactory) *cobra.Command {
	cmd := &cobra.Command{
		Short:   "Rename a repository",
		Use:     "rename <old> <new>",
		PreRunE: bindEnv("confirm"),
	}

	cmd.Flags().BoolP("confirm", "c", false, "Prompt to confirm all options interactively (Env: $FUNC_CONFIRM)")

	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		return runRepositoryRename(cmd, args, newClient)
	}

	return cmd
}

func NewRepositoryRemoveCmd(newClient ClientFactory) *cobra.Command {
	cmd := &cobra.Command{
		Short:      "Remove a repository",
		Use:        "remove <name>",
		Aliases:    []string{"rm"},
		SuggestFor: []string{"delete", "del"},
		PreRunE:    bindEnv("confirm"),
	}

	cmd.Flags().BoolP("confirm", "c", false, "Prompt to confirm all options interactively (Env: $FUNC_CONFIRM)")

	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		return runRepositoryRemove(cmd, args, newClient)
	}

	return cmd
}

// command implementations
// -----------------------

// Run
// (list by default or interactive with -c|--confirm)
func runRepository(cmd *cobra.Command, args []string, newClient ClientFactory) (err error) {
	cfg, err := newRepositoryConfig(args)
	if err != nil {
		return
	}

	// If in noninteractive, normal mode the help text is shown
	if !cfg.Confirm {
		return cmd.Help()
	}

	// If in interactive mode, the user chan choose which subcommand to invoke
	// Prompt for action to perform
	question := &survey.Question{
		Name: "Action",
		Prompt: &survey.Select{
			Message: "Operation to perform:",
			Options: []string{"list", "add", "rename", "remove"},
			Default: "list",
		}}
	answer := struct{ Action string }{}
	if err = survey.Ask([]*survey.Question{question}, &answer); err != nil {
		return
	}

	// Run the command indicated
	switch answer.Action {
	case "list":
		return runRepositoryList(cmd, args, newClient)
	case "add":
		return runRepositoryAdd(cmd, args, newClient)
	case "rename":
		return runRepositoryRename(cmd, args, newClient)
	case "remove":
		return runRepositoryRemove(cmd, args, newClient)
	}
	return fmt.Errorf("invalid action '%v'", answer.Action) // Unreachable
}

// List
func runRepositoryList(_ *cobra.Command, args []string, newClient ClientFactory) (err error) {
	cfg, err := newRepositoryConfig(args)
	if err != nil {
		return
	}

	client, done := newClient(ClientConfig{Verbose: cfg.Verbose},
		fn.WithRepositoriesPath(cfg.RepositoriesPath))
	defer done()

	// List all repositories given a client instantiated about config.
	rr, err := client.Repositories().All()
	if err != nil {
		return
	}

	// Print repository names, or name plus url if verbose
	// This follows the format of `git remote`, as it is likely familiar.
	for _, r := range rr {
		if cfg.Verbose {
			fmt.Fprintln(os.Stdout, r.Name+"\t"+r.URL())
		} else {
			fmt.Fprintln(os.Stdout, r.Name)
		}
	}
	return
}

// Add
func runRepositoryAdd(_ *cobra.Command, args []string, newClient ClientFactory) (err error) {
	// Supports both composable, discrete CLI commands or prompt-based "config"
	// by setting the argument values (name and ulr) to value of positional args,
	// but only requires them if not prompting.  If prompting, those values
	// become the prompt defaults.

	cfg, err := newRepositoryConfig(args)
	if err != nil {
		return
	}

	client, done := newClient(ClientConfig{Verbose: cfg.Verbose},
		fn.WithRepositoriesPath(cfg.RepositoriesPath))
	defer done()

	// Preconditions
	// If not confirming/prompting, assert the args were both provided.
	if len(args) != 2 && !cfg.Confirm {
		return fmt.Errorf("usage: func repository add <name> <url>")
	}

	// Extract Params
	// Populate a struct with the arguments (if provided)
	params := struct {
		Name string
		URL  string
	}{}
	if len(args) > 0 {
		params.Name = args[0]
	}
	if len(args) > 1 {
		params.URL = args[1]
	}

	// Prompt/Confirm
	// If confirming/prompting, interactively populate the params from the user
	// (using the current values as defaults)
	//
	// If terminal not interactive, effective values are echoed.
	//
	// Note that empty values can be passed to the final client's Add method if:
	//   Argument(s) not provided
	//   Confirming (-c|--confirm)
	//   Is a noninteractive terminal
	// This is an expected case.  The empty value will be echoed to stdout, the
	// API will be invoked, and a helpful error message will indicate that the
	// request is missing required parameters.
	if cfg.Confirm && interactiveTerminal() {
		questions := []*survey.Question{
			{
				Name:     "Name",
				Validate: survey.Required,
				Prompt: &survey.Input{
					Message: "Name for the new repository:",
					Default: params.Name,
				},
			}, {
				Name:     "URL",
				Validate: survey.Required,
				Prompt: &survey.Input{
					Message: "URL of the new repository:",
					Default: params.URL,
				},
			},
		}
		if err = survey.Ask(questions, &params); err != nil {
			return
			// not checking for terminal.InterruptError because failure to complete,
			// for whatever reason, should exit the program non-zero.
		}
	} else if cfg.Confirm {
		fmt.Fprintf(os.Stdout, "Name: %v\n", params.Name)
		fmt.Fprintf(os.Stdout, "URL:  %v\n", params.URL)
	}

	// Add repository
	var n string
	if n, err = client.Repositories().Add(params.Name, params.URL); err != nil {
		return
	}
	if cfg.Verbose {
		fmt.Fprintf(os.Stdout, "Repository added: %s\n", n)
	}
	return
}

// Rename
func runRepositoryRename(_ *cobra.Command, args []string, newClient ClientFactory) (err error) {
	cfg, err := newRepositoryConfig(args)
	if err != nil {
		return
	}
	client, done := newClient(ClientConfig{Verbose: cfg.Verbose},
		fn.WithRepositoriesPath(cfg.RepositoriesPath))
	defer done()

	// Preconditions
	if len(args) != 2 && !cfg.Confirm {
		return fmt.Errorf("usage: func repository rename <old> <new>")
	}

	// Extract Params
	params := struct {
		Old string
		New string
	}{}
	if len(args) > 0 {
		params.Old = args[0]
	}
	if len(args) > 1 {
		params.New = args[1]
	}

	// Repositories installed according to the client
	// (does not include the builtin default)
	repositories, err := installedRepositories(client)
	if err != nil {
		return
	}

	// Confirm (interactive prompt mode)
	if cfg.Confirm && interactiveTerminal() {
		questions := []*survey.Question{
			{
				Name:     "Old",
				Validate: survey.Required,
				Prompt: &survey.Select{
					Message: "Repository to rename:",
					Options: repositories,
				},
			}, {
				Name:     "New",
				Validate: survey.Required,
				Prompt: &survey.Input{
					Message: "New name:",
					Default: params.New,
				},
			},
		}
		if err = survey.Ask(questions, &params); err != nil {
			return // for any reason, including interrupt, is an nonzero exit
		}
	} else if cfg.Confirm {
		fmt.Fprintf(os.Stdout, "Old: %v\n", params.Old)
		fmt.Fprintf(os.Stdout, "New: %v\n", params.New)
	}

	// Rename the repository
	if err = client.Repositories().Rename(params.Old, params.New); err != nil {
		return
	}
	if cfg.Verbose {
		fmt.Fprintln(os.Stdout, "Repository renamed")
	}
	return
}

// Remove
func runRepositoryRemove(_ *cobra.Command, args []string, newClient ClientFactory) (err error) {
	cfg, err := newRepositoryConfig(args)
	if err != nil {
		return
	}
	client, done := newClient(ClientConfig{Verbose: cfg.Verbose},
		fn.WithRepositoriesPath(cfg.RepositoriesPath))
	defer done()

	// Preconditions
	if len(args) != 1 && !cfg.Confirm {
		return fmt.Errorf("usage: func repository remove <name>")
	}

	// Extract param(s)
	params := struct {
		Name string
		Sure bool
	}{}
	if len(args) > 0 {
		params.Name = args[0]
	}
	// "Are you sure" confirmation flag
	// (not using name 'Confirm' to avoid confustion with cfg.Confirm)
	// defaults to Yes.  This is debatable, but I don't want to choose the repo
	// to remove and then have to see a prompt and then have to hit 'y'.  Just
	// prompting once to make sure, which requires another press of enter, seems
	// sufficient.
	params.Sure = true

	// Repositories installed according to the client
	// (does not include the builtin default)
	repositories, err := installedRepositories(client)
	if err != nil {
		return
	}

	if len(repositories) == 0 {
		return errors.New("No repositories installed. use 'add' to install")
	}

	// Confirm (interactive prompt mode)
	if cfg.Confirm && interactiveTerminal() {
		questions := []*survey.Question{
			{
				Name:     "Name",
				Validate: survey.Required,
				Prompt: &survey.Select{
					Message: "Repository to remove:",
					Options: repositories,
				},
			}, {
				Name: "Sure",
				Prompt: &survey.Confirm{
					Message: "This will remove the repository from local disk. Are you sure?",
					Default: params.Sure,
				},
			},
		}
		if err = survey.Ask(questions, &params); err != nil {
			return // for any reason, including interrupt, is a nonzero exit
		}
	} else if cfg.Confirm {
		fmt.Fprintf(os.Stdout, "Repository: %v\n", params.Name)
	}

	// Cancel if they got cold feet.
	if !params.Sure {
		// While an argument could be made to the contrary, I believe it is
		// important than an abort by the user, either by answering no to the
		// confirmation or by an os interrupt such as ^C be considered an error,
		// and thus a non-zero program exit.  This is because a user may have
		// chained the command, and an abort (for whatever reason) should cancel
		// the whole chain.  For example, given the command:
		//    func repo rm -cv && doSomethingOnSuccess
		// The trailing command 'doSomethignOnSuccess' should not be evaluated if
		// the first, `func repo rm`, does not exit 0.
		if cfg.Verbose {
			fmt.Fprintln(os.Stdout, "Repository remove canceled")
		}
		return fmt.Errorf("repository removal canceled")
	}

	// Remove the repository
	if err = client.Repositories().Remove(params.Name); err != nil {
		return
	}
	if cfg.Verbose {
		fmt.Fprintln(os.Stdout, "Repository removed")
	}
	return
}

// Installed repositories
// All repositories which have been installed (does not include builtin)
func installedRepositories(client *fn.Client) ([]string, error) {
	// Client API contract stipulates the list always lists the defeault builtin
	// repo, and always lists it at index 0
	repositories, err := client.Repositories().List()
	if err != nil {
		return []string{}, err
	}
	return repositories[1:], nil
}

// client config
// -------------

// repositoryConfig used for instantiating a fn.Client
type repositoryConfig struct {
	RepositoriesPath string // Path to repos to be managed
	Verbose          bool   // Enables verbose logging
	Confirm          bool   // Enables interactive confirmation/prompting mode
}

// newRepositoryConfig creates a configuration suitable for use instantiating the
// fn Client. Note that parameters for the individual commands (add, remove etc)
// are collected separately in their requisite run functions.
func newRepositoryConfig(args []string) (cfg repositoryConfig, err error) {
	// initial config is populated based on flags, which are themselves
	// first populated by static defaults, then environment variables,
	// finally command flags.
	cfg = repositoryConfig{
		Verbose: viper.GetBool("verbose"),
		Confirm: viper.GetBool("confirm"),
	}

	// Repositories Path
	// Use env var to alter the default of ~/.config/func/repositories
	cfg.RepositoriesPath = os.Getenv("FUNC_REPOSITORIES_PATH")
	if cfg.RepositoriesPath == "" {
		cfg.RepositoriesPath = fn.New().RepositoriesPath()
	}

	// If not in confirm (interactive prompting) mode,
	// this struct is complete.
	if !cfg.Confirm {
		return
	}

	// Prompt the terminal for interactive input using the current values
	// as defaults. (noninteractive terminals are a noop)
	if interactiveTerminal() {
		return cfg.prompt()
	}

	// Noninteractive terminals in confirm/prompt mode simply echo
	// effective values to stdout.
	fmt.Fprintf(os.Stdout, "Repositories path: %v\n", cfg.RepositoriesPath)
	fmt.Fprintf(os.Stdout, "Verbose logging:   %v\n", cfg.Verbose)
	return
}

// prompt returns a config with values populated from interactivly prompting
// the user.
func (c repositoryConfig) prompt() (repositoryConfig, error) {
	// These prompts are overly verbose, as the user calling --confirm likely
	// only cares about the individual command-specific values (for example
	// "name" and "url" when calling "add".  However, we want to provide the
	// ability to interactively choose _all_ options if the user really wants
	// to, therefore these prompts are only shown if the user is "confirming
	// verbosely", for example `func repository add -cv`.  (of course the
	// associated flags, environment variables etc are still respected.  Just
	// no prompts unless verbose)
	if !c.Verbose || !interactiveTerminal() {
		return c, nil
	}

	// Prompt the user for the "global" settings Repositories Path and Verbose.
	// Not prompted for unless already in verbose mode (ex: func repo add -cv)
	qs := []*survey.Question{
		{
			Name: "Repositories",
			Prompt: &survey.Input{
				Message: "Path to repositories:",
				Default: c.RepositoriesPath,
			},
			Validate: survey.Required,
		}, {
			Name: "Verbose",
			Prompt: &survey.Confirm{
				Message: "Enable verbose logging:",
				Default: c.Verbose,
			},
		},
	}
	err := survey.Ask(qs, &c)
	return c, err
}
