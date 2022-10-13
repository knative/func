package cmd

import (
	"errors"
	"fmt"

	"github.com/AlecAivazis/survey/v2"
	"github.com/AlecAivazis/survey/v2/terminal"
	"github.com/ory/viper"
	"github.com/spf13/cobra"

	fn "knative.dev/func"
)

type functionLoader interface {
	Load(path string) (fn.Function, error)
}

type functionSaver interface {
	Save(f fn.Function) error
}

type functionLoaderSaver interface {
	functionLoader
	functionSaver
}

type standardLoaderSaver struct{}

func (s standardLoaderSaver) Load(path string) (fn.Function, error) {
	f, err := fn.NewFunction(path)
	if err != nil {
		return fn.Function{}, fmt.Errorf("failed to create new function (path: %q): %w", path, err)
	}
	if !f.Initialized() {
		return fn.Function{}, fmt.Errorf("the given path '%v' does not contain an initialized function", path)
	}
	return f, nil
}

func (s standardLoaderSaver) Save(f fn.Function) error {
	return f.Write()
}

var defaultLoaderSaver standardLoaderSaver

func NewConfigCmd(loadSaver functionLoaderSaver) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Configure a function",
		Long: `Configure a function

Interactive prompt that allows configuration of Volume mounts, Environment
variables, and Labels for a function project present in the current directory
or from the directory specified with --path.
`,
		SuggestFor: []string{"cfg", "cofnig"},
		PreRunE:    bindEnv("path"),
		RunE:       runConfigCmd,
	}
	cmd.SetHelpFunc(defaultTemplatedHelp)

	setPathFlag(cmd)

	cmd.AddCommand(NewConfigLabelsCmd(loadSaver))
	cmd.AddCommand(NewConfigEnvsCmd(loadSaver))
	cmd.AddCommand(NewConfigVolumesCmd())

	return cmd
}

func runConfigCmd(cmd *cobra.Command, args []string) (err error) {

	function, err := initConfigCommand(defaultLoaderSaver)
	if err != nil {
		return
	}

	var qs = []*survey.Question{
		{
			Name: "selectedConfig",
			Prompt: &survey.Select{
				Message: "What do you want to configure?",
				Options: []string{"Environment variables", "Volumes", "Labels"},
				Default: "Environment variables",
			},
		},
		{
			Name: "selectedOperation",
			Prompt: &survey.Select{
				Message: "What operation do you want to perform?",
				Options: []string{"Add", "Remove", "List"},
				Default: "List",
			},
		},
	}

	answers := struct {
		SelectedConfig    string
		SelectedOperation string
	}{}

	err = survey.Ask(qs, &answers)
	if err != nil {
		if errors.Is(err, terminal.InterruptErr) {
			return nil
		}
		return
	}

	switch answers.SelectedOperation {
	case "Add":
		if answers.SelectedConfig == "Volumes" {
			err = runAddVolumesPrompt(cmd.Context(), function)
		} else if answers.SelectedConfig == "Environment variables" {
			err = runAddEnvsPrompt(cmd.Context(), function)
		} else if answers.SelectedConfig == "Labels" {
			err = runAddLabelsPrompt(cmd.Context(), function, defaultLoaderSaver)
		}
	case "Remove":
		if answers.SelectedConfig == "Volumes" {
			err = runRemoveVolumesPrompt(function)
		} else if answers.SelectedConfig == "Environment variables" {
			err = runRemoveEnvsPrompt(function)
		} else if answers.SelectedConfig == "Labels" {
			err = runRemoveLabelsPrompt(function, defaultLoaderSaver)
		}
	case "List":
		if answers.SelectedConfig == "Volumes" {
			listVolumes(function)
		} else if answers.SelectedConfig == "Environment variables" {
			err = listEnvs(function, cmd.OutOrStdout(), Human)
		} else if answers.SelectedConfig == "Labels" {
			listLabels(function)
		}
	}

	return
}

// CLI Configuration (parameters)
// ------------------------------

type configCmdConfig struct {
	Path    string
	Verbose bool
}

func newConfigCmdConfig() configCmdConfig {
	return configCmdConfig{
		Path:    getPathFlag(),
		Verbose: viper.GetBool("verbose"),
	}
}

func initConfigCommand(loader functionLoader) (fn.Function, error) {
	config := newConfigCmdConfig()

	function, err := loader.Load(config.Path)
	if err != nil {
		return fn.Function{}, fmt.Errorf("failed to load the function (path: %q): %w", config.Path, err)
	}

	return function, nil
}
