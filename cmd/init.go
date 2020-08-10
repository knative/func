package cmd

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/boson-project/faas"
	"github.com/boson-project/faas/embedded"
	"github.com/boson-project/faas/prompt"
	"github.com/mitchellh/go-homedir"
	"github.com/ory/viper"
	"github.com/spf13/cobra"
)

func init() {
	cwd, err := os.Getwd()
	if err != nil {
		panic(err)
	}

	root.AddCommand(initCmd)
	initCmd.Flags().StringP("name", "n", "", "A name for the project, overriding the default path based name")
	initCmd.Flags().StringP("path", "p", cwd, "Path to the new project directory")
	initCmd.Flags().StringP("tag", "t", "", "Specify an image tag, for example quay.io/myrepo/project.name:latest")
	initCmd.Flags().StringP("trigger", "g", embedded.DefaultTemplate, "Function trigger (ex: 'http','events')")
	initCmd.Flags().StringP("templates", "", filepath.Join(configPath(), "faas", "templates"), "Extensible templates path")
	err = initCmd.MarkFlagRequired("tag")
	if err != nil {
		fmt.Println("Error marking 'tag' flag required")
	}
}

// The init command creates a new function project with a noop implementation.
var initCmd = &cobra.Command{
	Use:        "init <runtime> --tag=\"image tag\"",
	Short:      "Create a new function project",
	SuggestFor: []string{"inti", "new"},
	// TODO: Add completions for init
	// ValidArgsFunction: CompleteRuntimeList,
	RunE: initializeProject,
	PreRunE: func(cmd *cobra.Command, args []string) (err error) {
		flags := []string{"name", "path", "tag", "trigger", "templates"}
		for _, f := range flags {
			err := viper.BindPFlag(f, cmd.Flags().Lookup(f))
			if err != nil {
				return err
			}
		}
		return
	},
}

// The init command expects a runtime language/framework, and optionally
// a couple of flags.
type initConfig struct {
	// Verbose mode instructs the system to output detailed logs as the command
	// progresses.
	Verbose bool

	// Name of the service in DNS-compatible format (ex myfunc.example.com)
	Name string

	// Trigger is the form of the resultant function, i.e. the function signature
	// and contextually avaialable resources.  For example 'http' for a funciton
	// expected to be invoked via straight HTTP requests, or 'events' for a
	// function which will be invoked with CloudEvents.
	Trigger string

	// Templates is an optional path that, if it exists, will be used as a source
	// for additional templates not included in the binary.  If not provided
	// explicitly as a flag (--templates) or env (FAAS_TEMPLATES), the default
	// location is $XDG_CONFIG_HOME/templates ($HOME/.config/faas/templates)
	Templates string

	// Runtime is the first argument, and specifies the resultant Function
	// implementation runtime.
	Runtime string

	// Path of the Function implementation on local disk. Defaults to current
	// working directory of the process.
	Path string

	// Image tag for the resulting Function
	Tag string
}

func initializeProject(cmd *cobra.Command, args []string) (err error) {
	if len(args) == 0 {
		return errors.New("'faas init' requires a runtime argument")
	}

	var config = initConfig{
		Runtime:   args[0],
		Verbose:   viper.GetBool("verbose"),
		Name:      viper.GetString("name"),
		Path:      viper.GetString("path"),
		Trigger:   viper.GetString("trigger"),
		Templates: viper.GetString("templates"),
		Tag:       viper.GetString("tag"),
	}

	// If we are running as an interactive terminal, allow the user
	// to mutate default config prior to execution.
	if interactiveTerminal() {
		config, err = promptWithDefaults(config)
		if err != nil {
			return err
		}
	}

	// Initializer creates a deployable noop function implementation in the
	// configured path.
	initializer := embedded.NewInitializer(config.Templates)
	initializer.Verbose = config.Verbose

	client, err := faas.New(
		faas.WithVerbose(config.Verbose),
		faas.WithInitializer(initializer),
	)
	if err != nil {
		return err
	}

	// Invoke the creation of the new Function locally.
	// Returns the final address.
	// Name can be empty string (path-dervation will be attempted)
	// Path can be empty, defaulting to current working directory.
	_, err = client.Initialize(config.Runtime, config.Trigger, config.Name, config.Tag, config.Path)

	// If no error this returns nil
	return err
}

func promptWithDefaults(config initConfig) (c initConfig, err error) {
	config.Path = prompt.ForString("Path to project directory", config.Path)
	config.Name, err = promptForName("Function project name", config)
	if err != nil {
		return config, err
	}
	config.Runtime = prompt.ForString("Runtime of source", config.Runtime)
	config.Trigger = prompt.ForString("Function Template", config.Trigger)
	config.Tag = prompt.ForString("Image tag", config.Tag)
	return config, nil
}

// Prompting for Name with Default
// Early calclation of service function name is required to provide a sensible
// default.  If the user did not provide a --name parameter or FAAS_NAME,
// this funciton sets the default to the value that the client would have done
// on its own if non-interactive: by creating a new function rooted at config.Path
// and then calculate from that path.
func promptForName(label string, config initConfig) (string, error) {
	// Pre-calculate the function name derived from path
	if config.Name == "" {
		f, err := faas.NewFunction(config.Path)
		if err != nil {
			return "", err
		}
		maxRecursion := 5 // TODO synchronize with that used in actual initialize step.
		return prompt.ForString("Name of service function", f.DerivedName(maxRecursion), prompt.WithRequired(true)), nil
	}

	// The user provided a --name or FAAS_NAME; just confirm it.
	return prompt.ForString("Name of service function", config.Name, prompt.WithRequired(true)), nil
}

func configPath() (path string) {
	if path = os.Getenv("XDG_CONFIG_HOME"); path != "" {
		return
	}

	path, err := homedir.Expand("~/.config")
	if err != nil {
		fmt.Fprintf(os.Stderr, "could not derive home directory for use as default templates path: %v", err)
	}
	return
}
