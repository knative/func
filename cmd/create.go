package cmd

import (
	"fmt"
	"path/filepath"

	"github.com/ory/viper"
	"github.com/spf13/cobra"

	fn "github.com/boson-project/func"
	"github.com/boson-project/func/prompt"
	"github.com/boson-project/func/utils"
)

func init() {
	root.AddCommand(createCmd)
	createCmd.Flags().BoolP("confirm", "c", false, "Prompt to confirm all configuration options (Env: $FUNC_CONFIRM)")
	createCmd.Flags().StringP("runtime", "l", fn.DefaultRuntime, "Function runtime language/framework. Available runtimes: "+utils.RuntimeList()+" (Env: $FUNC_RUNTIME)")
	createCmd.Flags().StringP("templates", "", filepath.Join(configPath(), "templates"), "Path to additional templates (Env: $FUNC_TEMPLATES)")
	createCmd.Flags().StringP("trigger", "t", fn.DefaultTrigger, "Function trigger. Available triggers: 'http' and 'events' (Env: $FUNC_TRIGGER)")

	if err := createCmd.RegisterFlagCompletionFunc("runtime", CompleteRuntimeList); err != nil {
		fmt.Println("internal: error while calling RegisterFlagCompletionFunc: ", err)
	}
}

var createCmd = &cobra.Command{
	Use:   "create [PATH]",
	Short: "Create a function project",
	Long: `Create a function project

Creates a new function project in PATH, or in the current directory if no PATH is given. 
The name of the project is determined by the directory name the project is created in.
`,
	Example: `
# Create a Node.js function project in the current directory, choosing the
# directory name as the project's name.
kn func create

# Create a Quarkus function project in the directory "sample-service". 
# The directory will be created in the local directory if non-existent and 
# the project is called "sample-service"
kn func create --runtime quarkus myfunc

# Create a function project that uses a CloudEvent based function signature
kn func create --trigger events myfunc
`,
	SuggestFor: []string{"inti", "new"},
	PreRunE:    bindEnv("runtime", "templates", "trigger", "confirm"),
	RunE:       runCreate,
	// TODO: autocomplate Functions for runtime and trigger.
}

func runCreate(cmd *cobra.Command, args []string) error {
	config := newCreateConfig(args)

	if err := utils.ValidateFunctionName(config.Name); err != nil {
		return err
	}

	config = config.Prompt()

	function := fn.Function{
		Name:    config.Name,
		Root:    config.Path,
		Runtime: config.Runtime,
		Trigger: config.Trigger,
	}

	client := fn.New(
		fn.WithTemplates(config.Templates),
		fn.WithVerbose(config.Verbose))

	return client.Create(function)
}

type createConfig struct {
	// Name of the Function.
	Name string

	// Absolute path to Function on disk.
	Path string

	// Runtime language/framework.
	Runtime string

	// Templates is an optional path that, if it exists, will be used as a source
	// for additional templates not included in the binary.  If not provided
	// explicitly as a flag (--templates) or env (FUNC_TEMPLATES), the default
	// location is $XDG_CONFIG_HOME/templates ($HOME/.config/func/templates)
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

// newCreateConfig returns a config populated from the current execution context
// (args, flags and environment variables)
func newCreateConfig(args []string) createConfig {
	var path string
	if len(args) > 0 {
		path = args[0] // If explicitly provided, use.
	}

	derivedName, derivedPath := deriveNameAndAbsolutePathFromPath(path)
	return createConfig{
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
func (c createConfig) Prompt() createConfig {
	if !interactiveTerminal() || !c.Confirm {
		// Just print the basics if not confirming
		fmt.Printf("Project path: %v\n", c.Path)
		fmt.Printf("Function name: %v\n", c.Name)
		fmt.Printf("Runtime: %v\n", c.Runtime)
		fmt.Printf("Trigger: %v\n", c.Trigger)
		return c
	}

	var derivedName, derivedPath string
	for {
		derivedName, derivedPath = deriveNameAndAbsolutePathFromPath(prompt.ForString("Project path", c.Path, prompt.WithRequired(true)))
		err := utils.ValidateFunctionName(derivedName)
		if err == nil {
			break
		}
		fmt.Println("Error:", err)
	}

	return createConfig{
		Name:    derivedName,
		Path:    derivedPath,
		Runtime: prompt.ForString("Runtime", c.Runtime),
		Trigger: prompt.ForString("Trigger", c.Trigger),
		// Templates intentionally omitted from prompt for being an edge case.
	}
}
