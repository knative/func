package cmd

import (
	"context"
	"fmt"
	"strings"

	"github.com/AlecAivazis/survey/v2"
	"github.com/spf13/cobra"

	"knative.dev/func/pkg/config"
	fn "knative.dev/func/pkg/functions"
	"knative.dev/func/pkg/k8s"
)

func NewConfigVolumesCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "volumes",
		Short: "List and manage configured volumes for a function",
		Long: `List and manage configured volumes for a function

Prints configured Volume mounts for a function project present in
the current directory or from the directory specified with --path.
`,
		Aliases:    []string{"volume"},
		SuggestFor: []string{"vol", "volums", "vols"},
		PreRunE:    bindEnv("path", "verbose"),
		RunE: func(cmd *cobra.Command, args []string) (err error) {
			function, err := initConfigCommand(defaultLoaderSaver)
			if err != nil {
				return
			}

			listVolumes(function)

			return
		},
	}
	cfg, err := config.NewDefault()
	if err != nil {
		fmt.Fprintf(cmd.OutOrStdout(), "error loading config at '%v'. %v\n", config.File(), err)
	}

	configVolumesAddCmd := NewConfigVolumesAddCmd()
	configVolumesRemoveCmd := NewConfigVolumesRemoveCmd()

	addPathFlag(cmd)
	addPathFlag(configVolumesAddCmd)
	addPathFlag(configVolumesRemoveCmd)

	addVerboseFlag(cmd, cfg.Verbose)
	addVerboseFlag(configVolumesAddCmd, cfg.Verbose)
	addVerboseFlag(configVolumesRemoveCmd, cfg.Verbose)

	cmd.AddCommand(configVolumesAddCmd)
	cmd.AddCommand(configVolumesRemoveCmd)

	return cmd
}

func NewConfigVolumesAddCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "add",
		Short: "Add volume to the function configuration",
		Long: `Add volume to the function configuration

Interactive prompt to add Secrets and ConfigMaps as Volume mounts to the function project
in the current directory or from the directory specified with --path.
`,
		SuggestFor: []string{"ad", "create", "insert", "append"},
		PreRunE:    bindEnv("path", "verbose"),
		RunE: func(cmd *cobra.Command, args []string) (err error) {
			function, err := initConfigCommand(defaultLoaderSaver)
			if err != nil {
				return
			}

			return runAddVolumesPrompt(cmd.Context(), function)
		},
	}

	return cmd
}

func NewConfigVolumesRemoveCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "remove",
		Short: "Remove volume from the function configuration",
		Long: `Remove volume from the function configuration

Interactive prompt to remove Volume mounts from the function project
in the current directory or from the directory specified with --path.
`,
		Aliases:    []string{"rm"},
		SuggestFor: []string{"del", "delete", "rmeove"},
		PreRunE:    bindEnv("path", "verbose"),
		RunE: func(cmd *cobra.Command, args []string) (err error) {
			function, err := initConfigCommand(defaultLoaderSaver)
			if err != nil {
				return
			}

			return runRemoveVolumesPrompt(function)
		},
	}

	return cmd
}

func listVolumes(f fn.Function) {
	if len(f.Run.Volumes) == 0 {
		fmt.Println("There aren't any configured Volume mounts")
		return
	}

	fmt.Println("Configured Volumes mounts:")
	for _, v := range f.Run.Volumes {
		fmt.Println(" - ", v.String())
	}
}

func runAddVolumesPrompt(ctx context.Context, f fn.Function) (err error) {

	secrets, err := k8s.ListSecretsNamesIfConnected(ctx, f.Deploy.Namespace)
	if err != nil {
		return
	}
	configMaps, err := k8s.ListConfigMapsNamesIfConnected(ctx, f.Deploy.Namespace)
	if err != nil {
		return
	}
	persistentVolumeClaims, err := k8s.ListPersistentVolumeClaimsNamesIfConnected(ctx, f.Deploy.Namespace)
	if err != nil {
		return
	}

	// SECTION - select resource type to be mounted
	options := []string{}
	selectedOption := ""
	const optionConfigMap = "ConfigMap"
	const optionSecret = "Secret"
	const optionPersistentVolumeClaim = "PersistentVolumeClaim"
	const optionEmptyDir = "EmptyDir"

	if len(configMaps) > 0 {
		options = append(options, optionConfigMap)
	}
	if len(secrets) > 0 {
		options = append(options, optionSecret)
	}
	if len(persistentVolumeClaims) > 0 {
		options = append(options, optionPersistentVolumeClaim)
	}
	options = append(options, optionEmptyDir)

	if len(options) == 1 {
		selectedOption = options[0]
	} else {
		err = survey.AskOne(&survey.Select{
			Message: "What do you want to mount as a Volume?",
			Options: options,
		}, &selectedOption)
		if err != nil {
			return
		}
	}

	// SECTION - display a help message to enable advanced features
	if selectedOption == optionEmptyDir || selectedOption == optionPersistentVolumeClaim {
		fmt.Printf("Please make sure to enable the %s extension flag: https://knative.dev/docs/serving/configuration/feature-flags/\n", selectedOption)
	}

	// SECTION - select the specific resource to be mounted
	optionsResoures := []string{}
	switch selectedOption {
	case optionConfigMap:
		optionsResoures = configMaps
	case optionSecret:
		optionsResoures = secrets
	case optionPersistentVolumeClaim:
		optionsResoures = persistentVolumeClaims
	}

	selectedResource := ""
	if selectedOption != optionEmptyDir {
		err = survey.AskOne(&survey.Select{
			Message: fmt.Sprintf("Which \"%s\" do you want to mount?", selectedOption),
			Options: optionsResoures,
		}, &selectedResource)
		if err != nil {
			return
		}
	}

	// SECTION - specify mount Path of the Volume

	path := ""
	err = survey.AskOne(&survey.Input{
		Message: fmt.Sprintf("Please specify the path where the %s should be mounted:", selectedOption),
	}, &path, survey.WithValidator(func(val interface{}) error {
		if str, ok := val.(string); !ok || len(str) <= 0 || !strings.HasPrefix(str, "/") {
			return fmt.Errorf("the input must be non-empty absolute path")
		}
		return nil
	}))
	if err != nil {
		return
	}

	// SECTION - is this read only for pvc
	readOnly := false
	if selectedOption == optionPersistentVolumeClaim {
		err = survey.AskOne(&survey.Confirm{
			Message: "Is this volume read-only?",
			Default: false,
		}, &readOnly)
		if err != nil {
			return
		}
	}

	// we have all necessary information -> let's store the new Volume
	newVolume := fn.Volume{Path: &path}
	switch selectedOption {
	case optionConfigMap:
		newVolume.ConfigMap = &selectedResource
	case optionSecret:
		newVolume.Secret = &selectedResource
	case optionPersistentVolumeClaim:
		newVolume.PresistentVolumeClaim = &fn.PersistentVolumeClaim{
			ClaimName: &selectedResource,
			ReadOnly:  readOnly,
		}
	case optionEmptyDir:
		newVolume.EmptyDir = &fn.EmptyDir{}
	}

	f.Run.Volumes = append(f.Run.Volumes, newVolume)

	err = f.Write()
	if err == nil {
		fmt.Println("Volume entry was added to the function configuration")
	}

	return
}

func runRemoveVolumesPrompt(f fn.Function) (err error) {
	if len(f.Run.Volumes) == 0 {
		fmt.Println("There aren't any configured Volume mounts")
		return
	}

	options := []string{}
	for _, v := range f.Run.Volumes {
		options = append(options, v.String())
	}

	selectedVolume := ""
	prompt := &survey.Select{
		Message: "Which Volume do you want to remove?",
		Options: options,
	}
	err = survey.AskOne(prompt, &selectedVolume)
	if err != nil {
		return
	}

	var newVolumes []fn.Volume
	removed := false
	for i, v := range f.Run.Volumes {
		if v.String() == selectedVolume {
			newVolumes = append(f.Run.Volumes[:i], f.Run.Volumes[i+1:]...)
			removed = true
			break
		}
	}

	if removed {
		f.Run.Volumes = newVolumes
		err = f.Write()
		if err == nil {
			fmt.Println("Volume entry was removed from the function configuration")
		}
	}

	return
}
