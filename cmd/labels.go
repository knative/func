package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/AlecAivazis/survey/v2"
	"github.com/spf13/cobra"

	"knative.dev/func/pkg/config"
	fn "knative.dev/func/pkg/functions"
	"knative.dev/func/pkg/utils"
)

func NewLabelsCmd(newClient ClientFactory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "labels",
		Short: "Manage function labels",
		Long: `{{rootCmdUse}} labels

Manages function labels.  Default is to list currently configured labels.  See
subcommands 'add' and 'remove'.`,
		Aliases:    []string{"label"},
		SuggestFor: []string{"albels", "abels"},
		PreRunE:    bindEnv("path", "output", "verbose"),
		RunE: func(cmd *cobra.Command, _ []string) (err error) {
			return runLabels(cmd, newClient)
		},
	}

	cfg, err := config.NewDefault()
	if err != nil {
		fmt.Fprintf(cmd.OutOrStdout(), "error loading config at '%v'. %v\n", config.File(), err)
	}

	// Flags
	addPathFlag(cmd)
	addVerboseFlag(cmd, cfg.Verbose)

	// TODO: use global config Output setting, treating empty string as deafult
	// to mean human-optimized.
	cmd.Flags().StringP("output", "o", "human", "Output format (human|json) (Env: $FUNC_OUTPUT)")

	// Subcommands
	cmd.AddCommand(NewLabelsAddCmd(newClient))
	cmd.AddCommand(NewLabelsRemoveCmd(newClient))

	return cmd
}

func runLabels(cmd *cobra.Command, newClient ClientFactory) (err error) {
	var (
		cfg metadataConfig
		f   fn.Function
	)
	if cfg, err = newMetadataConfig().Prompt(); err != nil {
		return
	}
	if err = cfg.Validate(); err != nil {
		return
	}
	if f, err = fn.NewFunction(cfg.Path); err != nil {
		return
	}
	if f, err = cfg.Configure(f); err != nil {
		return
	}

	switch Format(cfg.Output) {
	case Human:
		if len(f.Deploy.Labels) == 0 {
			fmt.Fprintln(cmd.OutOrStdout(), "No labels")
			return
		}
		fmt.Fprintln(cmd.OutOrStdout(), "Labels:")
		for _, l := range f.Deploy.Labels {
			fmt.Fprintf(cmd.OutOrStdout(), "  %v\n", l)
		}
		return
	case JSON:
		return json.NewEncoder(cmd.OutOrStdout()).Encode(f.Deploy.Labels)
	default:
		fmt.Fprintf(cmd.ErrOrStderr(), "invalid format: %v", cfg.Output)
		return
	}
}

func NewLabelsAddCmd(newClient ClientFactory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "add",
		Short: "Add label to the function",
		Long: `Add labels to the function 

Add labels to the function project in the current directory or from the
directory specified with --path.  If no flags are provided, addition is
completed using interactive prompts.

The label can be set directly from a value or from an environment variable on
the local machine.`,
		Example: `# add a label
{{rootCmdUse}} labels add --key=myLabel --value=myValue

# add a label from a local environment variable
{{rootCmdUse}} labels add --key=myLabel --value='{{"{{"}} env:LOC_ENV {{"}}"}}'`,
		SuggestFor: []string{"ad", "create", "insert", "append"},
		PreRunE:    bindEnv("verbose", "path"),
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runLabelsAdd(cmd, newClient)
		},
	}
	cfg, err := config.NewDefault()
	if err != nil {
		fmt.Fprintf(cmd.OutOrStdout(), "error loading config at '%v'. %v\n", config.File(), err)
	}
	addPathFlag(cmd)
	addVerboseFlag(cmd, cfg.Verbose)
	cmd.Flags().StringP("key", "", "", "Key of the label.")
	cmd.Flags().StringP("value", "", "", "Value of the label.")

	return cmd
}

func runLabelsAdd(cmd *cobra.Command, newClient ClientFactory) (err error) {
	var (
		cfg metadataConfig
		f   fn.Function
	)
	if cfg, err = newMetadataConfig().Prompt(); err != nil {
		return
	}
	if err = cfg.Validate(); err != nil {
		return
	}
	if f, err = fn.NewFunction(cfg.Path); err != nil {
		return
	}
	if f, err = cfg.Configure(f); err != nil {
		return
	}

	// TODO: The below implementation was was written prior to global config,
	// local configs and the general command structure now used.
	// As such, it needs to be refactored to use name and value flags from the
	// command's config struct, to use the validation function, and to have its
	// major sections extracted.
	// Furthermore, the core functionality here should probably be in the core
	// client library and merely invoked from this CLI such that users of the
	// client library also have accss to these features.

	// Noninteractive
	// --------------

	var np *string
	var vp *string

	if cmd.Flags().Changed("key") {
		s, e := cmd.Flags().GetString("key")
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

	// Validators
	if np != nil {
		if err := utils.ValidateLabelKey(*np); err != nil {
			return err
		}
	}
	if vp != nil {
		if err := utils.ValidateLabelValue(*vp); err != nil {
			return err
		}
	}

	// Set
	if np != nil || vp != nil {
		f.Deploy.Labels = append(f.Deploy.Labels, fn.Label{Key: np, Value: vp})
		return f.Write()
	}

	// Interactive
	// --------------

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

	err = f.Write()
	if err == nil {
		fmt.Println("Label entry was added to the function configuration")
	}

	return
}

func NewLabelsRemoveCmd(newClient ClientFactory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "remove",
		Short: "Remove labels from the function configuration",
		Long: `Remove labels from the function configuration

Interactive prompt to remove labels from the function project in the current
directory or from the directory specified with --path.
`,
		Aliases:    []string{"rm"},
		SuggestFor: []string{"del", "delete", "rmeove"},
		PreRunE:    bindEnv("verbose", "path"),
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runLabelsRemove(cmd, newClient)
		},
	}
	cfg, err := config.NewDefault()
	if err != nil {
		fmt.Fprintf(cmd.OutOrStdout(), "error loading config at '%v'. %v\n", config.File(), err)
	}
	addPathFlag(cmd)
	addVerboseFlag(cmd, cfg.Verbose)

	return cmd
}

func runLabelsRemove(cmd *cobra.Command, newClient ClientFactory) (err error) {
	var (
		cfg metadataConfig
		f   fn.Function
	)
	if cfg, err = newMetadataConfig().Prompt(); err != nil {
		return
	}
	if err = cfg.Validate(); err != nil {
		return
	}
	if f, err = fn.NewFunction(cfg.Path); err != nil {
		return
	}
	if f, err = cfg.Configure(f); err != nil { // update f with cfg if applicable
		return
	}

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
		err = f.Write()
		if err == nil {
			fmt.Println("Label was removed from the function configuration")
		}
	}
	return
}
