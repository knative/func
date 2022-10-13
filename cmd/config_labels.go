package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/AlecAivazis/survey/v2"
	"github.com/AlecAivazis/survey/v2/terminal"
	"github.com/spf13/cobra"

	fn "knative.dev/func"
	"knative.dev/func/utils"
)

func NewConfigLabelsCmd(loaderSaver functionLoaderSaver) *cobra.Command {
	var configLabelsCmd = &cobra.Command{
		Use:   "labels",
		Short: "List and manage configured labels for a function",
		Long: `List and manage configured labels for a function

Prints configured labels for a function project present in
the current directory or from the directory specified with --path.
`,
		SuggestFor: []string{"albels", "abels", "label"},
		PreRunE:    bindEnv("path"),
		RunE: func(cmd *cobra.Command, args []string) (err error) {
			function, err := initConfigCommand(loaderSaver)
			if err != nil {
				return
			}

			listLabels(function)

			return
		},
	}
	configLabelsCmd.SetHelpFunc(defaultTemplatedHelp)

	var configLabelsAddCmd = &cobra.Command{
		Use:   "add",
		Short: "Add labels to the function configuration",
		Long: `Add labels to the function configuration

Interactive prompt to add labels to the function project in the current
directory or from the directory specified with --path.

The label can be set directly from a value or from an environment variable on
the local machine.
`,
		SuggestFor: []string{"ad", "create", "insert", "append"},
		PreRunE:    bindEnv("path"),
		RunE: func(cmd *cobra.Command, args []string) (err error) {
			function, err := initConfigCommand(loaderSaver)
			if err != nil {
				return
			}

			return runAddLabelsPrompt(cmd.Context(), function, loaderSaver)
		},
	}
	configLabelsAddCmd.SetHelpFunc(defaultTemplatedHelp)

	var configLabelsRemoveCmd = &cobra.Command{
		Use:   "remove",
		Short: "Remove labels from the function configuration",
		Long: `Remove labels from the function configuration

Interactive prompt to remove labels from the function project in the current
directory or from the directory specified with --path.
`,
		SuggestFor: []string{"del", "delete", "rmeove"},
		PreRunE:    bindEnv("path"),
		RunE: func(cmd *cobra.Command, args []string) (err error) {
			function, err := initConfigCommand(loaderSaver)
			if err != nil {
				return
			}

			return runRemoveLabelsPrompt(function, loaderSaver)
		},
	}
	configLabelsRemoveCmd.SetHelpFunc(defaultTemplatedHelp)

	setPathFlag(configLabelsCmd)
	setPathFlag(configLabelsAddCmd)
	setPathFlag(configLabelsRemoveCmd)
	configLabelsCmd.AddCommand(configLabelsAddCmd)
	configLabelsCmd.AddCommand(configLabelsRemoveCmd)

	return configLabelsCmd
}

func listLabels(f fn.Function) {
	if len(f.Deploy.Labels) == 0 {
		fmt.Println("There aren't any configured labels")
		return
	}

	fmt.Println("Configured labels:")
	for _, e := range f.Deploy.Labels {
		fmt.Println(" - ", e.String())
	}
}

func runAddLabelsPrompt(ctx context.Context, f fn.Function, saver functionSaver) (err error) {

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
			if err == terminal.InterruptErr {
				return nil
			}
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
		if err == terminal.InterruptErr {
			return nil
		}
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
			if err == terminal.InterruptErr {
				return nil
			}
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
			if err == terminal.InterruptErr {
				return nil
			}
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
		if err == terminal.InterruptErr {
			return nil
		}
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
