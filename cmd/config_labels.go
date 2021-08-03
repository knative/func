package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/AlecAivazis/survey/v2"
	"github.com/AlecAivazis/survey/v2/terminal"
	"github.com/spf13/cobra"

	fn "knative.dev/kn-plugin-func"
	"knative.dev/kn-plugin-func/utils"
)

func init() {
	configCmd.AddCommand(configLabelsCmd)
	configLabelsCmd.Flags().StringP("path", "p", cwd(), "Path to the project directory (Env: $FUNC_PATH)")
	configLabelsCmd.AddCommand(configLabelsAddCmd)
	configLabelsAddCmd.Flags().StringP("path", "p", cwd(), "Path to the project directory (Env: $FUNC_PATH)")
	configLabelsCmd.AddCommand(configLabelsRemoveCmd)
	configLabelsRemoveCmd.Flags().StringP("path", "p", cwd(), "Path to the project directory (Env: $FUNC_PATH)")
}

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
		function, err := initConfigCommand(args)
		if err != nil {
			return
		}

		listLabels(function)

		return
	},
}

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
		function, err := initConfigCommand(args)
		if err != nil {
			return
		}

		return runAddLabelsPrompt(cmd.Context(), function)
	},
}

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
		function, err := initConfigCommand(args)
		if err != nil {
			return
		}

		return runRemoveLabelsPrompt(function)
	},
}

func listLabels(f fn.Function) {
	if len(f.Labels) == 0 {
		fmt.Println("There aren't any configured labels")
		return
	}

	fmt.Println("Configured labels:")
	for _, e := range f.Labels {
		fmt.Println(" - ", e.String())
	}
}

func runAddLabelsPrompt(ctx context.Context, f fn.Function) (err error) {

	insertToIndex := 0

	// SECTION - if there are some labels already set, choose the position of the new entry
	if len(f.Labels) > 0 {
		options := []string{}
		for _, e := range f.Labels {
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

	newPair := fn.Pair{}

	switch selectedOption {
	// SECTION - add new label with the specified value
	case optionLabelValue:
		qs := []*survey.Question{
			{
				Name:   "name",
				Prompt: &survey.Input{Message: "Please specify the label name:"},
				Validate: func(val interface{}) error {
					return utils.ValidateLabelName(val.(string))
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
			Name  string
			Value string
		}{}

		err = survey.Ask(qs, &answers)
		if err != nil {
			if err == terminal.InterruptErr {
				return nil
			}
			return
		}

		newPair.Name = &answers.Name
		newPair.Value = &answers.Value

	// SECTION - add new label with value from a local environment variable
	case optionLabelLocal:
		qs := []*survey.Question{
			{
				Name:   "name",
				Prompt: &survey.Input{Message: "Please specify the label name:"},
				Validate: func(val interface{}) error {
					return utils.ValidateLabelName(val.(string))
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
			Name  string
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
		newPair.Name = &answers.Name
		newPair.Value = &value
	}

	// we have all necessary information -> let's insert the label to the selected position in the list
	if insertToIndex == len(f.Labels) {
		f.Labels = append(f.Labels, newPair)
	} else {
		f.Labels = append(f.Labels[:insertToIndex+1], f.Labels[insertToIndex:]...)
		f.Labels[insertToIndex] = newPair
	}

	err = f.WriteConfig()
	if err == nil {
		fmt.Println("Label entry was added to the function configuration")
	}

	return
}

func runRemoveLabelsPrompt(f fn.Function) (err error) {
	if len(f.Labels) == 0 {
		fmt.Println("There aren't any configured labels")
		return
	}

	options := []string{}
	for _, e := range f.Labels {
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

	var newLabels fn.Pairs
	removed := false
	for i, e := range f.Labels {
		if e.String() == selectedLabel {
			newLabels = append(f.Labels[:i], f.Labels[i+1:]...)
			removed = true
			break
		}
	}

	if removed {
		f.Labels = newLabels
		err = f.WriteConfig()
		if err == nil {
			fmt.Println("Label was removed from the function configuration")
		}
	}

	return
}
