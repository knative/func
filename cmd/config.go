package cmd

import (
	"fmt"
	"os"

	"github.com/AlecAivazis/survey/v2"
	"github.com/ory/viper"
	"github.com/spf13/cobra"

	"knative.dev/func/cmd/ci"
	"knative.dev/func/cmd/common"
	"knative.dev/func/pkg/config"
	fn "knative.dev/func/pkg/functions"
)

func NewConfigCmd(loaderSaver common.FunctionLoaderSaver, writer ci.WorkflowWriter, newClient ClientFactory) *cobra.Command {
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
	cmd.AddCommand(NewConfigLabelsCmd(loaderSaver))
	cmd.AddCommand(NewConfigEnvsCmd(loaderSaver))
	cmd.AddCommand(NewConfigVolumesCmd())

	if os.Getenv(ci.ConfigCIFeatureFlag) == "true" {
		cmd.AddCommand(NewConfigCICmd(loaderSaver, writer))
	}

	return cmd
}

func runConfigCmd(cmd *cobra.Command, args []string) (err error) {

	function, err := initConfigCommand(common.DefaultLoaderSaver)
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
		switch answers.SelectedConfig {
		case "Volumes":
			err = runAddVolumesPrompt(cmd.Context(), function)
		case "Environment variables":
			err = runAddEnvsPrompt(cmd.Context(), function)
		case "Labels":
			err = runAddLabelsPrompt(cmd.Context(), function, common.DefaultLoaderSaver)
		case "Git":
			err = runConfigGitSetCmd(cmd, NewClient)
		}
	case "Remove":
		switch answers.SelectedConfig {
		case "Volumes":
			err = runRemoveVolumesPrompt(function)
		case "Environment variables":
			err = runRemoveEnvsPrompt(function)
		case "Labels":
			err = runRemoveLabelsPrompt(function, common.DefaultLoaderSaver)
		case "Git":
			err = runConfigGitRemoveCmd(cmd, NewClient)
		}
	case "List":
		switch answers.SelectedConfig {
		case "Volumes":
			listVolumes(function)
		case "Environment variables":
			err = listEnvs(function, cmd.OutOrStdout(), Human)
		case "Labels":
			err = listLabels(function, cmd.OutOrStdout(), Human)
		case "Git":
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

func initConfigCommand(loader common.FunctionLoader) (fn.Function, error) {
	config := newConfigCmdConfig()

	function, err := loader.Load(config.Path)
	if err != nil {
		return fn.Function{}, err
	}

	return function, nil
}
