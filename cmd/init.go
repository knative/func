package cmd

import (
	"fmt"
	"path/filepath"

	"github.com/ory/viper"
	"github.com/spf13/cobra"

	"github.com/boson-project/faas"
	"github.com/boson-project/faas/prompt"
)

func init() {
	root.AddCommand(initCmd)
	initCmd.Flags().BoolP("confirm", "c", false, "Prompt to confirm all configuration options - $FAAS_CONFIRM")
	initCmd.Flags().StringP("runtime", "l", faas.DefaultRuntime, "Function runtime language/framework. Default runtime is 'go'. Available runtimes: 'node', 'quarkus' and 'go'. - $FAAS_RUNTIME")
	initCmd.Flags().StringP("templates", "", filepath.Join(configPath(), "templates"), "Extensible templates path. - $FAAS_TEMPLATES")
	initCmd.Flags().StringP("trigger", "t", faas.DefaultTrigger, "Function trigger. Default trigger is 'http'. Available triggers: 'http' and 'events' - $FAAS_TRIGGER")

	if err := initCmd.RegisterFlagCompletionFunc("runtime", CompleteRuntimeList); err != nil {
		fmt.Println("Error while calling RegisterFlagCompletionFunc: ", err)
	}
}

var initCmd = &cobra.Command{
	Use:   "init <path>",
	Short: "Initialize a new Function project",
	Long: `Initializes a new Function project

Creates a new Function project at <path>. If <path> does not exist, it is
created. The Function name is the name of the leaf directory at <path>.

A project for a Go Function will be created by default. Specify an alternate
runtime with the --language or -l flag. Available alternates are "node" and
"quarkus".

Use the --trigger or -t flag to specify the function invocation context.
By default, the trigger is "http". To create a Function for CloudEvents, use
"events".
`,
	SuggestFor: []string{"inti", "new"},
	PreRunE:    bindEnv("runtime", "templates", "trigger", "confirm"),
	RunE:       runInit,
	// TODO: autocomplate Functions for runtime and trigger.
}

func runInit(cmd *cobra.Command, args []string) error {
	config := newInitConfig(args).Prompt()

	function := faas.Function{
		Name:    config.Name,
		Root:    config.Path,
		Runtime: config.Runtime,
		Trigger: config.Trigger,
	}

	client := faas.New(
		faas.WithTemplates(config.Templates),
		faas.WithVerbose(config.Verbose))

	return client.Initialize(function)
}

type initConfig struct {
	// Name of the Function.
	Name string

	// Absolute path to Function on disk.
	Path string

	// Runtime language/framework.
	Runtime string

	// Templates is an optional path that, if it exists, will be used as a source
	// for additional templates not included in the binary.  If not provided
	// explicitly as a flag (--templates) or env (FAAS_TEMPLATES), the default
	// location is $XDG_CONFIG_HOME/templates ($HOME/.config/faas/templates)
	Templates string

	// Trigger is the form of the resultant Function, i.e. the Function signature
	// and contextually avaialable resources.  For example 'http' for a Function
	// expected to be invoked via straight HTTP requests, or 'events' for a
	// Function which will be invoked with CloudEvents.
	Trigger string

	// Verbose output
	Verbose bool

	// Confirm: confirm values arrived upon from environment plus flags plus defaults,
	// with interactive prompting (only applicable when attached to a TTY).
	Confirm bool
}

// newInitConfig returns a config populated from the current execution context
// (args, flags and environment variables)
func newInitConfig(args []string) initConfig {
	var path string
	if len(args) > 0 {
		path = args[0] // If explicitly provided, use.
	}

	derivedName, derivedPath := deriveNameAndAbsolutePathFromPath(path)
	return initConfig{
		Name:      derivedName,
		Path:      derivedPath,
		Runtime:   viper.GetString("runtime"),
		Templates: viper.GetString("templates"),
		Trigger:   viper.GetString("trigger"),
		Confirm:   viper.GetBool("confirm"),
		Verbose:   viper.GetBool("verbose"),
	}
}

// Prompt the user with value of config members, allowing for interaractive changes.
// Skipped if not in an interactive terminal (non-TTY), or if --confirm false (agree to
// all prompts) was set (default).
func (c initConfig) Prompt() initConfig {
	if !interactiveTerminal() || !c.Confirm {
		// Just print the basics if not confirming
		fmt.Printf("Project path: %v\n", c.Path)
		fmt.Printf("Function name: %v\n", c.Name)
		fmt.Printf("Runtime: %v\n", c.Runtime)
		fmt.Printf("Trigger: %v\n", c.Trigger)
		return c
	}

	derivedName, derivedPath := deriveNameAndAbsolutePathFromPath(prompt.ForString("Project path", c.Path, prompt.WithRequired(true)))
	return initConfig{
		Name:    derivedName,
		Path:    derivedPath,
		Runtime: prompt.ForString("Runtime", c.Runtime),
		Trigger: prompt.ForString("Trigger", c.Trigger),
		// Templates intentiopnally omitted from prompt for being an edge case.
	}
}
