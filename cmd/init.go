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
	initCmd.Flags().StringP("path", "p", cwd(), "Path to the new project directory - $FAAS_PATH")
	initCmd.Flags().StringP("runtime", "l", faas.DefaultRuntime, "Function runtime language/framework. - $FAAS_RUNTIME")
	initCmd.Flags().StringP("templates", "", filepath.Join(configPath(), "faas", "templates"), "Extensible templates path. - $FAAS_TEMPLATES")
	initCmd.Flags().StringP("trigger", "t", faas.DefaultTrigger, "Function trigger (ex: 'http','events') - $FAAS_TRIGGER")
	initCmd.Flags().BoolP("yes", "y", false, "When in interactive mode (attached to a TTY), skip prompts. - $FAAS_YES")

	if err := initCmd.RegisterFlagCompletionFunc("runtime", CompleteRuntimeList); err != nil {
		fmt.Println("Error while calling RegisterFlagCompletionFunc: ", err)
	}
}

var initCmd = &cobra.Command{
	Use:        "init <name> [options]",
	Short:      "Initialize a new Function project",
	SuggestFor: []string{"inti", "new"},
	PreRunE:    bindEnv("path", "runtime", "templates", "trigger", "yes"),
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
		faas.WithVerbose(config.Verbose),
		faas.WithTemplates(config.Templates))

	return client.Initialize(function)
}

type initConfig struct {
	// Name of the service in DNS-compatible format (ex myfunc.example.com)
	Name string

	// Path to files on disk.  Defaults to current working directory.
	Path string

	// Runtime language/framework.
	Runtime string

	// Templates is an optional path that, if it exists, will be used as a source
	// for additional templates not included in the binary.  If not provided
	// explicitly as a flag (--templates) or env (FAAS_TEMPLATES), the default
	// location is $XDG_CONFIG_HOME/templates ($HOME/.config/faas/templates)
	Templates string

	// Trigger is the form of the resultant Function, i.e. the Function signature
	// and contextually avaialable resources.  For example 'http' for a funciton
	// expected to be invoked via straight HTTP requests, or 'events' for a
	// Function which will be invoked with CloudEvents.
	Trigger string

	// Verbose logging enabled.
	Verbose bool

	// Yes: agree to values arrived upon from environment plus flags plus defaults,
	// and skip the interactive prompting (only applicable when attached to a TTY).
	Yes bool
}

// newInitConfig returns a config populated from the current execution context
// (args, flags and environment variables)
func newInitConfig(args []string) initConfig {
	var name string
	if len(args) > 0 {
		name = args[0] // If explicitly provided, use.
	}
	return initConfig{
		Name:      deriveName(name, viper.GetString("path")), // args[0] or derived
		Path:      viper.GetString("path"),
		Runtime:   viper.GetString("runtime"),
		Templates: viper.GetString("templates"),
		Trigger:   viper.GetString("trigger"),
		Verbose:   viper.GetBool("verbose"), // defined on root
		Yes:       viper.GetBool("yes"),
	}
}

// Prompt the user with value of config members, allowing for interaractive changes.
// Skipped if not in an interactive terminal (non-TTY), or if --yes (agree to
// all prompts) was explicitly set.
func (c initConfig) Prompt() initConfig {
	if !interactiveTerminal() || c.Yes {
		return c
	}
	return initConfig{
		// TODO: Path should be prompted for and set prior to name attempting path derivation.  Test/fix this if necessary.
		Path:    prompt.ForString("Path to project directory", c.Path),
		Name:    prompt.ForString("Function project name", deriveName(c.Name, c.Path), prompt.WithRequired(true)),
		Verbose: prompt.ForBool("Verbose logging", c.Verbose),
		Runtime: prompt.ForString("Runtime of source", c.Runtime),
		Trigger: prompt.ForString("Function Trigger", c.Trigger),
		// Templates intentiopnally omitted from prompt for being an edge case.
	}
}
