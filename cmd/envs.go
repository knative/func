package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"github.com/AlecAivazis/survey/v2"
	"github.com/AlecAivazis/survey/v2/terminal"
	"github.com/spf13/cobra"

	"knative.dev/func/pkg/config"
	fn "knative.dev/func/pkg/functions"
	"knative.dev/func/pkg/k8s"
	"knative.dev/func/pkg/utils"
)

func NewEnvsCmd(newClient ClientFactory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "envs",
		Short: "Manage function environment variables",
		Long: `{{rootCmdUse}} envs

Manages function environment variables.  Default is to list currently configured
environment variables for the function.  See subcommands 'add' and 'remove'.`,
		Aliases:    []string{"env"},
		SuggestFor: []string{"ensv"},
		PreRunE:    bindEnv("path", "output", "verbose"),
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runEnvs(cmd, newClient)
		},
	}

	// Global Config
	cfg, err := config.NewDefault()
	if err != nil {
		fmt.Fprintf(cmd.OutOrStdout(), "error loading config at '%v'. %v\n", config.File(), err)
	}

	// Flags
	addVerboseFlag(cmd, cfg.Verbose)
	addPathFlag(cmd)

	// TODO: use global config Output setting, treating empty string as deafult
	// to mean human-optimized.
	cmd.Flags().StringP("output", "o", "human", "Output format (human|json) (Env: $FUNC_OUTPUT)")

	// Subcommands
	cmd.AddCommand(NewEnvsAddCmd(newClient))
	cmd.AddCommand(NewEnvsRemoveCmd(newClient))

	return cmd
}

func runEnvs(cmd *cobra.Command, newClient ClientFactory) (err error) {
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
	if f, err = cfg.Configure(f); err != nil { // update f with metadata cfg
		return
	}

	switch Format(cfg.Output) {
	case Human:
		if len(f.Run.Envs) == 0 {
			fmt.Fprintln(cmd.OutOrStdout(), "No environment variables")
			return
		}
		fmt.Fprintln(cmd.OutOrStdout(), "Environment variables:")
		for _, v := range f.Run.Envs {
			fmt.Fprintf(cmd.OutOrStdout(), "  %s\n", v)
		}
		return
	case JSON:
		return json.NewEncoder(cmd.OutOrStdout()).Encode(f.Run.Envs)
	default:
		fmt.Fprintf(cmd.ErrOrStderr(), "invalid format: %v", cfg.Output)
		return
	}
}

func NewEnvsAddCmd(newClient ClientFactory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "add",
		Short: "Add environment variable to the function",
		Long: `Add environment variable to the function.

If environment variable is not set explicitly by flag, interactive prompt is used.

The environment variable can be set directly from a value,
from an environment variable on the local machine or from Secrets and ConfigMaps.
It is also possible to import all keys as environment variables from a Secret or ConfigMap.`,
		Example: `# set environment variable directly
{{rootCmdUse}} envs add --name=VARNAME --value=myValue

# set environment variable from local env $LOC_ENV
{{rootCmdUse}} envs add --name=VARNAME --value='{{"{{"}} env:LOC_ENV {{"}}"}}'

set environment variable from a secret
{{rootCmdUse}} envs add --name=VARNAME --value='{{"{{"}} secret:secretName:key {{"}}"}}'

# set all key as environment variables from a secret
{{rootCmdUse}} envs add --value='{{"{{"}} secret:secretName {{"}}"}}'

# set environment variable from a configMap
{{rootCmdUse}} envs add --name=VARNAME --value='{{"{{"}} configMap:confMapName:key {{"}}"}}'

# set all key as environment variables from a configMap
{{rootCmdUse}} envs add --value='{{"{{"}} configMap:confMapName {{"}}"}}'`,
		SuggestFor: []string{"ad", "create", "insert", "append"},
		PreRunE:    bindEnv("verbose", "path", "name", "value"),
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runEnvsAdd(cmd, newClient)
		},
	}
	cfg, err := config.NewDefault()
	if err != nil {
		fmt.Fprintf(cmd.OutOrStdout(), "error loading config at '%v'. %v\n", config.File(), err)
	}

	addVerboseFlag(cmd, cfg.Verbose)
	addPathFlag(cmd)
	cmd.Flags().StringP("name", "", "", "Name of the environment variable.")
	cmd.Flags().StringP("value", "", "", "Value of the environment variable.")

	return cmd
}

func runEnvsAdd(cmd *cobra.Command, newClient ClientFactory) (err error) {
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
	if f, err = cfg.Configure(f); err != nil { // update f with metadata cfg
		return
	}

	// TODO:  the below implementation was imported verbatim from config_envs.go
	//        which was written before global and local config was created.
	//  As such it needs to be refactored to use name and value flags from config,
	// the validation function of config, and have its major sections extracted.
	// Furthermore, the core functionality should probably be in the core
	// client library and merely invoked from this CLI such that users of the
	// client library have access to these features.

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

	if np != nil || vp != nil {
		if np != nil {
			if err := utils.ValidateEnvVarName(*np); err != nil {
				return err
			}
		}

		f.Run.Envs = append(f.Run.Envs, fn.Env{Name: np, Value: vp})
		return f.Write()
	}

	insertToIndex := 0

	// SECTION - if there are some envs already set, let choose the position of the new entry
	if len(f.Run.Envs) > 0 {
		options := []string{}
		for _, e := range f.Run.Envs {
			options = append(options, fmt.Sprintf("Insert before:  %s", e.String()))
		}
		options = append(options, "Insert here.")

		selectedEnv := ""
		prompt := &survey.Select{
			Message: "Where do you want to add the Environment variable?",
			Options: options,
			Default: options[len(options)-1],
		}
		err = survey.AskOne(prompt, &selectedEnv)
		if err != nil {
			return
		}

		for i, option := range options {
			if option == selectedEnv {
				insertToIndex = i
				break
			}
		}
	}

	// SECTION - select the type of Environment variable to be added
	secrets, err := k8s.ListSecretsNamesIfConnected(cmd.Context(), f.Deploy.Namespace)
	if err != nil {
		return
	}
	configMaps, err := k8s.ListConfigMapsNamesIfConnected(cmd.Context(), f.Deploy.Namespace)
	if err != nil {
		return
	}

	selectedOption := ""
	const (
		optionEnvValue        = "Environment variable with a specified value"
		optionEnvLocal        = "Value from a local environment variable"
		optionEnvConfigMap    = "ConfigMap: all key=value pairs as environment variables"
		optionEnvConfigMapKey = "ConfigMap: value from a key"
		optionEnvSecret       = "Secret: all key=value pairs as environment variables"
		optionEnvSecretKey    = "Secret: value from a key"
	)
	options := []string{optionEnvValue, optionEnvLocal}

	if len(configMaps) > 0 {
		options = append(options, optionEnvConfigMap)
		options = append(options, optionEnvConfigMapKey)
	}
	if len(secrets) > 0 {
		options = append(options, optionEnvSecret)
		options = append(options, optionEnvSecretKey)
	}

	err = survey.AskOne(&survey.Select{
		Message: "What type of Environment variable do you want to add?",
		Options: options,
	}, &selectedOption)
	if err != nil {
		return
	}

	newEnv := fn.Env{}

	switch selectedOption {
	// SECTION - add new Environment variable with the specified value
	case optionEnvValue:
		qs := []*survey.Question{
			{
				Name:   "name",
				Prompt: &survey.Input{Message: "Please specify the Environment variable name:"},
				Validate: func(val interface{}) error {
					return utils.ValidateEnvVarName(val.(string))
				},
			},
			{
				Name:   "value",
				Prompt: &survey.Input{Message: "Please specify the Environment variable value:"},
			},
		}
		answers := struct {
			Name  string
			Value string
		}{}

		err = survey.Ask(qs, &answers)
		if err != nil {
			if errors.Is(err, terminal.InterruptErr) {
				return nil
			}
			return
		}

		newEnv.Name = &answers.Name
		newEnv.Value = &answers.Value

	// SECTION - add new Environment variable with value from a local environment variable
	case optionEnvLocal:
		qs := []*survey.Question{
			{
				Name:   "name",
				Prompt: &survey.Input{Message: "Please specify the Environment variable name:"},
				Validate: func(val interface{}) error {
					return utils.ValidateEnvVarName(val.(string))
				},
			},
			{
				Name:     "value",
				Prompt:   &survey.Input{Message: "Please specify the local Environment variable:"},
				Validate: survey.Required,
			},
		}
		answers := struct {
			Name  string
			Value string
		}{}

		err = survey.Ask(qs, &answers)
		if err != nil {
			if errors.Is(err, terminal.InterruptErr) {
				return nil
			}
			return
		}

		if _, ok := os.LookupEnv(answers.Value); !ok {
			fmt.Printf("Warning: specified local environment variable %q is not set\n", answers.Value)
		}

		value := fmt.Sprintf("{{ env:%s }}", answers.Value)
		newEnv.Name = &answers.Name
		newEnv.Value = &value

	// SECTION - Add all key=value pairs from ConfigMap as environment variables
	case optionEnvConfigMap:
		selectedResource := ""
		err = survey.AskOne(&survey.Select{
			Message: "Which ConfigMap do you want to use?",
			Options: configMaps,
		}, &selectedResource)
		if err != nil {
			if errors.Is(err, terminal.InterruptErr) {
				return nil
			}
			return
		}

		value := fmt.Sprintf("{{ configMap:%s }}", selectedResource)
		newEnv.Value = &value

	// SECTION - Environment variable with value from a key from ConfigMap
	case optionEnvConfigMapKey:
		qs := []*survey.Question{
			{
				Name: "configmap",
				Prompt: &survey.Select{
					Message: "Which ConfigMap do you want to use?",
					Options: configMaps,
				},
			},
			{
				Name:   "name",
				Prompt: &survey.Input{Message: "Please specify the Environment variable name:"},
				Validate: func(val interface{}) error {
					return utils.ValidateEnvVarName(val.(string))
				},
			},
			{
				Name:   "key",
				Prompt: &survey.Input{Message: "Please specify a key from the selected ConfigMap:"},
				Validate: func(val interface{}) error {
					return utils.ValidateConfigMapKey(val.(string))
				},
			},
		}
		answers := struct {
			ConfigMap string
			Name      string
			Key       string
		}{}

		err = survey.Ask(qs, &answers)
		if err != nil {
			if errors.Is(err, terminal.InterruptErr) {
				return nil
			}
			return
		}

		value := fmt.Sprintf("{{ configMap:%s:%s }}", answers.ConfigMap, answers.Key)
		newEnv.Name = &answers.Name
		newEnv.Value = &value

	// SECTION - Add all key=value pairs from Secret as environment variables
	case optionEnvSecret:
		selectedResource := ""
		err = survey.AskOne(&survey.Select{
			Message: "Which Secret do you want to use?",
			Options: secrets,
		}, &selectedResource)
		if err != nil {
			if err == terminal.InterruptErr {
				return nil
			}
			return
		}

		value := fmt.Sprintf("{{ secret:%s }}", selectedResource)
		newEnv.Value = &value

	// SECTION - Environment variable with value from a key from Secret
	case optionEnvSecretKey:
		qs := []*survey.Question{
			{
				Name: "secret",
				Prompt: &survey.Select{
					Message: "Which Secret do you want to use?",
					Options: secrets,
				},
			},
			{
				Name:   "name",
				Prompt: &survey.Input{Message: "Please specify the Environment variable name:"},
				Validate: func(val interface{}) error {
					return utils.ValidateEnvVarName(val.(string))
				},
			},
			{
				Name:   "key",
				Prompt: &survey.Input{Message: "Please specify a key from the selected Secret:"},
				Validate: func(val interface{}) error {
					return utils.ValidateSecretKey(val.(string))
				},
			},
		}
		answers := struct {
			Secret string
			Name   string
			Key    string
		}{}

		err = survey.Ask(qs, &answers)
		if err != nil {
			if err == terminal.InterruptErr {
				return nil
			}
			return
		}

		value := fmt.Sprintf("{{ secret:%s:%s }}", answers.Secret, answers.Key)
		newEnv.Name = &answers.Name
		newEnv.Value = &value
	}

	// we have all necessary information -> let's insert the env to the selected position in the list
	if insertToIndex == len(f.Run.Envs) {
		f.Run.Envs = append(f.Run.Envs, newEnv)
	} else {
		f.Run.Envs = append(f.Run.Envs[:insertToIndex+1], f.Run.Envs[insertToIndex:]...)
		f.Run.Envs[insertToIndex] = newEnv
	}

	err = f.Write()
	if err == nil {
		fmt.Println("Environment variable entry was added to the function configuration")
	}

	return
}

func NewEnvsRemoveCmd(newClient ClientFactory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "remove",
		Short: "Remove environment variable from the function configuration",
		Long: `Remove environment variable from the function configuration

Interactive prompt to remove Environment variables from the function project
in the current directory or from the directory specified with --path.
`,
		Aliases:    []string{"rm"},
		SuggestFor: []string{"del", "delete", "rmeove"},
		PreRunE:    bindEnv("path", "verbose"),
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runEnvsRemove(cmd, newClient)
		},
	}
	cfg, err := config.NewDefault()
	if err != nil {
		fmt.Fprintf(cmd.OutOrStdout(), "error loading config at '%v'. %v\n", config.File(), err)
	}
	addVerboseFlag(cmd, cfg.Verbose)
	addPathFlag(cmd)
	return cmd
}

func runEnvsRemove(cmd *cobra.Command, newClient ClientFactory) (err error) {
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
	if f, err = cfg.Configure(f); err != nil { // update f with metadata cfg
		return
	}

	if len(f.Run.Envs) == 0 {
		fmt.Println("There aren't any configured Environment variables")
		return
	}

	options := []string{}
	for _, e := range f.Run.Envs {
		options = append(options, e.String())
	}

	selectedEnv := ""
	prompt := &survey.Select{
		Message: "Which Environment variables do you want to remove?",
		Options: options,
	}
	err = survey.AskOne(prompt, &selectedEnv)
	if err != nil {
		if err == terminal.InterruptErr {
			return nil
		}
		return
	}

	var newEnvs []fn.Env
	removed := false
	for i, e := range f.Run.Envs {
		if e.String() == selectedEnv {
			newEnvs = append(f.Run.Envs[:i], f.Run.Envs[i+1:]...)
			removed = true
			break
		}
	}

	if removed {
		f.Run.Envs = newEnvs
		err = f.Write()
		if err == nil {
			fmt.Println("Environment variable entry was removed from the function configuration")
		}
	}

	return
}
