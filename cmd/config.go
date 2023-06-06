package cmd

import (
	"fmt"

	"github.com/AlecAivazis/survey/v2"
	"github.com/ory/viper"
	"github.com/spf13/cobra"

	"knative.dev/func/pkg/config"
	fn "knative.dev/func/pkg/functions"
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
		return fn.Function{}, fn.NewErrNotInitialized(f.Root)
	}
	return f, nil
}

func (s standardLoaderSaver) Save(f fn.Function) error {
	return f.Write()
}

var defaultLoaderSaver standardLoaderSaver

func NewConfigCmd(loadSaver functionLoaderSaver, newClient ClientFactory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Configure a function",
		Long: `Configure a function

Interactive prompt that allows configuration of Git configuration, Volume mounts, Environment
variables, and Labels for a function project present in the current directory
or from the directory specified with --path.
`,
		SuggestFor: []string{"cfg", "cofnig"},
		PreRunE:    bindEnv("path", "verbose"),
		RunE:       runConfigCmd,
	}
	cfg, err := config.NewDefault()
	if err != nil {
		fmt.Fprintf(cmd.OutOrStdout(), "error loading config at '%v'. %v\n", config.File(), err)
	}

	addPathFlag(cmd)
	addVerboseFlag(cmd, cfg.Verbose)

	cmd.AddCommand(NewConfigGitCmd(newClient))
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
				Options: []string{"Git", "Environment variables", "Volumes", "Labels"},
				Default: "Git",
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
		} else if answers.SelectedConfig == "Git" {
			err = runConfigGitSetCmd(cmd, NewClient)
		}
	case "Remove":
		if answers.SelectedConfig == "Volumes" {
			err = runRemoveVolumesPrompt(function)
		} else if answers.SelectedConfig == "Environment variables" {
			err = runRemoveEnvsPrompt(function)
		} else if answers.SelectedConfig == "Labels" {
			err = runRemoveLabelsPrompt(function, defaultLoaderSaver)
		} else if answers.SelectedConfig == "Git" {
			err = runConfigGitRemoveCmd(cmd, NewClient)
		}
	case "List":
		if answers.SelectedConfig == "Volumes" {
			listVolumes(function)
		} else if answers.SelectedConfig == "Environment variables" {
			err = listEnvs(function, cmd.OutOrStdout(), Human)
		} else if answers.SelectedConfig == "Labels" {
			listLabels(function)
		} else if answers.SelectedConfig == "Git" {
			err = runConfigGitCmd(cmd, NewClient)
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
		Path:    viper.GetString("path"),
		Verbose: viper.GetBool("verbose"),
	}
}

func initConfigCommand(loader functionLoader) (fn.Function, error) {
	config := newConfigCmdConfig()

	function, err := loader.Load(config.Path)
	if err != nil {
		return fn.Function{}, err
	}

	return function, nil
}
