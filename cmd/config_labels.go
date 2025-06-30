package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/AlecAivazis/survey/v2"
	"github.com/ory/viper"
	"github.com/spf13/cobra"

	"knative.dev/func/pkg/config"
	fn "knative.dev/func/pkg/functions"
	"knative.dev/func/pkg/utils"
)

func NewConfigLabelsCmd(loaderSaver functionLoaderSaver) *cobra.Command {
	var configLabelsCmd = &cobra.Command{
		Use:   "labels",
		Short: "List and manage configured labels for a function",
		Long: `List and manage configured labels for a function

Prints configured labels for a function project present in
the current directory or from the directory specified with --path.
`,
		Aliases:    []string{"label"},
		SuggestFor: []string{"albels", "abels"},
		PreRunE:    bindEnv("path", "output", "verbose"),
		RunE: func(cmd *cobra.Command, args []string) (err error) {
			function, err := initConfigCommand(loaderSaver)
			if err != nil {
				return
			}

			return listLabels(function, cmd.OutOrStdout(), Format(viper.GetString("output")))
		},
	}

	var configLabelsAddCmd = &cobra.Command{
		Use:   "add",
		Short: "Add labels to the function configuration",
		Long: `Add labels to the function configuration

If label is not set explicitly by flag, interactive prompt is used.

The label can be set directly from a value or from an environment variable on
the local machine.
`,
		Example: `# set label directly
{{rootCmdUse}} config labels add --name=Foo --value=Bar

# set label from local env $FOO
{{rootCmdUse}} config labels add --name=Foo --value='{{"{{"}} env:FOO {{"}}"}}'`,
		SuggestFor: []string{"ad", "create", "insert", "append"},
		PreRunE:    bindEnv("path", "name", "value", "verbose"),
		RunE: func(cmd *cobra.Command, args []string) (err error) {
			function, err := initConfigCommand(loaderSaver)
			if err != nil {
				return
			}

			var np *string
			var vp *string

			if cmd.Flags().Changed("name") {
				s, e := cmd.Flags().GetString("name")
				if e != nil {
					return e
				}
				np = &s
			}
			if cmd.Flags().Changed("value") {
				s, e := cmd.Flags().GetString("value")
				if e != nil {
					return e
				}
				vp = &s
			}

			if np != nil && vp != nil {
				if err := utils.ValidateLabelKey(*np); err != nil {
					return err
				}
				if err := utils.ValidateLabelValue(*vp); err != nil {
					return err
				}

				function.Deploy.Labels = append(function.Deploy.Labels, fn.Label{Key: np, Value: vp})
				return loaderSaver.Save(function)
			}

			return runAddLabelsPrompt(cmd.Context(), function, loaderSaver)
		},
	}

	var configLabelsRemoveCmd = &cobra.Command{
		Use:   "remove",
		Short: "Remove labels from the function configuration",
		Long: `Remove labels from the function configuration

Interactive prompt to remove labels from the function project in the current
directory or from the directory specified with --path.
`,
		Aliases:    []string{"rm"},
		SuggestFor: []string{"del", "delete", "rmeove"},
		PreRunE:    bindEnv("path", "name", "verbose"),
		RunE: func(cmd *cobra.Command, args []string) (err error) {
			function, err := initConfigCommand(loaderSaver)
			if err != nil {
				return
			}

			var name string
			if cmd.Flags().Changed("name") {
				s, e := cmd.Flags().GetString("name")
				if e != nil {
					return e
				}
				name = s
			}

			if name != "" {
				labels := []fn.Label{}
				for _, v := range function.Deploy.Labels {
					if v.Key == nil || *v.Key != name {
						labels = append(labels, v)
					}
				}
				function.Deploy.Labels = labels
				return loaderSaver.Save(function)
			}

			return runRemoveLabelsPrompt(function, loaderSaver)
		},
	}

	cfg, err := config.NewDefault()
	if err != nil {
		fmt.Fprintf(configLabelsCmd.OutOrStdout(), "error loading config at '%v'. %v\n", config.File(), err)
	}

	// Add flags
	configLabelsCmd.Flags().StringP("output", "o", "human", "Output format (human|json)")
	configLabelsAddCmd.Flags().StringP("name", "", "", "Name of the label.")
	configLabelsAddCmd.Flags().StringP("value", "", "", "Value of the label.")
	configLabelsRemoveCmd.Flags().StringP("name", "", "", "Name of the label.")

	addPathFlag(configLabelsCmd)
	addPathFlag(configLabelsAddCmd)
	addPathFlag(configLabelsRemoveCmd)
	addVerboseFlag(configLabelsCmd, cfg.Verbose)
	addVerboseFlag(configLabelsAddCmd, cfg.Verbose)
	addVerboseFlag(configLabelsRemoveCmd, cfg.Verbose)

	configLabelsCmd.AddCommand(configLabelsAddCmd)
	configLabelsCmd.AddCommand(configLabelsRemoveCmd)

	return configLabelsCmd
}

func listLabels(f fn.Function, w io.Writer, outputFormat Format) error {
	switch outputFormat {
	case Human:
		if len(f.Deploy.Labels) == 0 {
			_, err := fmt.Fprintln(w, "No labels defined")
			return err
		}

		fmt.Fprintln(w, "Labels:")
		for _, e := range f.Deploy.Labels {
			_, err := fmt.Fprintln(w, " - ", e.String())
			if err != nil {
				return err
			}
		}
		return nil
	case JSON:
		enc := json.NewEncoder(w)
		return enc.Encode(f.Deploy.Labels)
	default:
		return fmt.Errorf("invalid format: %v", outputFormat)
	}
}

func runAddLabelsPrompt(_ context.Context, f fn.Function, saver functionSaver) (err error) {

	insertToIndex := 0

	// SECTION - if there are some labels already set, choose the position of the new entry
	if len(f.Deploy.Labels) > 0 {
		options := []string{}
		for _, e := range f.Deploy.Labels {
			options = append(options, fmt.Sprintf("Insert before:  %s", e.String()))
		}
		options = append(options, "Insert here.")

		selectedLabel := ""
		prompt := &survey.Select{
			Message: "Where do you want to add the label?",
			Options: options,
			Default: options[len(options)-1],
		}
		err = survey.AskOne(prompt, &selectedLabel)
		if err != nil {
			return
		}

		for i, option := range options {
			if option == selectedLabel {
				insertToIndex = i
				break
			}
		}
	}

	// SECTION - select the type of label to be added
	selectedOption := ""
	const (
		optionLabelValue = "Label with a specified value"
		optionLabelLocal = "Value from a local environment variable"
	)
	options := []string{optionLabelValue, optionLabelLocal}

	err = survey.AskOne(&survey.Select{
		Message: "What type of label do you want to add?",
		Options: options,
	}, &selectedOption)
	if err != nil {
		return
	}

	newPair := fn.Label{}

	switch selectedOption {
	// SECTION - add new label with the specified value
	case optionLabelValue:
		qs := []*survey.Question{
			{
				Name:   "key",
				Prompt: &survey.Input{Message: "Please specify the label key:"},
				Validate: func(val interface{}) error {
					return utils.ValidateLabelKey(val.(string))
				},
			},
			{
				Name:   "value",
				Prompt: &survey.Input{Message: "Please specify the label value:"},
				Validate: func(val interface{}) error {
					return utils.ValidateLabelValue(val.(string))
				}},
		}
		answers := struct {
			Key   string
			Value string
		}{}

		err = survey.Ask(qs, &answers)
		if err != nil {
			return
		}

		newPair.Key = &answers.Key
		newPair.Value = &answers.Value

	// SECTION - add new label with value from a local environment variable
	case optionLabelLocal:
		qs := []*survey.Question{
			{
				Name:   "key",
				Prompt: &survey.Input{Message: "Please specify the label key:"},
				Validate: func(val interface{}) error {
					return utils.ValidateLabelKey(val.(string))
				},
			},
			{
				Name:   "value",
				Prompt: &survey.Input{Message: "Please specify the local environment variable:"},
				Validate: func(val interface{}) error {
					return utils.ValidateLabelValue(val.(string))
				},
			},
		}
		answers := struct {
			Key   string
			Value string
		}{}

		err = survey.Ask(qs, &answers)
		if err != nil {
			return
		}

		if _, ok := os.LookupEnv(answers.Value); !ok {
			fmt.Printf("Warning: specified local environment variable %q is not set\n", answers.Value)
		}

		value := fmt.Sprintf("{{ env:%s }}", answers.Value)
		newPair.Key = &answers.Key
		newPair.Value = &value
	}

	// we have all necessary information -> let's insert the label to the selected position in the list
	if insertToIndex == len(f.Deploy.Labels) {
		f.Deploy.Labels = append(f.Deploy.Labels, newPair)
	} else {
		f.Deploy.Labels = append(f.Deploy.Labels[:insertToIndex+1], f.Deploy.Labels[insertToIndex:]...)
		f.Deploy.Labels[insertToIndex] = newPair
	}

	err = saver.Save(f)
	if err == nil {
		fmt.Println("Label entry was added to the function configuration")
	}

	return
}

func runRemoveLabelsPrompt(f fn.Function, saver functionSaver) (err error) {
	if len(f.Deploy.Labels) == 0 {
		fmt.Println("There aren't any configured labels")
		return
	}

	options := []string{}
	for _, e := range f.Deploy.Labels {
		options = append(options, e.String())
	}

	selectedLabel := ""
	prompt := &survey.Select{
		Message: "Which labels do you want to remove?",
		Options: options,
	}
	err = survey.AskOne(prompt, &selectedLabel)
	if err != nil {
		return
	}

	var newLabels []fn.Label
	removed := false
	for i, e := range f.Deploy.Labels {
		if e.String() == selectedLabel {
			newLabels = append(f.Deploy.Labels[:i], f.Deploy.Labels[i+1:]...)
			removed = true
			break
		}
	}

	if removed {
		f.Deploy.Labels = newLabels
		err = saver.Save(f)
		if err == nil {
			fmt.Println("Label was removed from the function configuration")
		}
	}

	return
}
