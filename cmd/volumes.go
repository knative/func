package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/AlecAivazis/survey/v2"
	"github.com/spf13/cobra"

	"knative.dev/func/pkg/config"
	fn "knative.dev/func/pkg/functions"
	"knative.dev/func/pkg/k8s"
)

func NewVolumesCmd(newClient ClientFactory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "volumes",
		Short: "Manage function volumes",
		Long: `{{rootCmdUse}} volumes 

Manges function volumes mounts.  Default is to list currently configured
volumes.  See subcommands 'add' and 'remove'.
`,
		Aliases:    []string{"volume", "vol", "vols"},
		SuggestFor: []string{"volums"},
		PreRunE:    bindEnv("path", "output", "verbose"),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runVolumes(cmd, newClient)
		},
	}

	cfg, err := config.NewDefault()
	if err != nil {
		fmt.Fprintf(cmd.OutOrStdout(), "error loading config at '%v'. %v\n", config.File(), err)
	}

	// Flags
	addPathFlag(cmd)
	addVerboseFlag(cmd, cfg.Verbose)

	// TODO: use glboal config Output setting, treating empty string as default
	// to mean human-optimized.
	cmd.Flags().StringP("output", "o", "human", "Output format (human|json) (Env: $FUNC_OUTPUT)")

	// Subcommands
	cmd.AddCommand(NewVolumesAddCmd(newClient))
	cmd.AddCommand(NewVolumesRemoveCmd(newClient))

	return cmd
}

func runVolumes(cmd *cobra.Command, newClient ClientFactory) (err error) {
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
		if len(f.Run.Volumes) == 0 {
			fmt.Fprintln(cmd.OutOrStdout(), "No volumes")
			return
		}
		fmt.Fprintln(cmd.OutOrStdout(), "Volumes:")
		for _, v := range f.Run.Volumes {
			fmt.Fprintf(cmd.OutOrStdout(), "  %s\n", v)
		}
		return
	case JSON:
		return json.NewEncoder(cmd.OutOrStdout()).Encode(f.Run.Volumes)
	default:
		fmt.Fprintf(cmd.ErrOrStderr(), "invalid format: %v", cfg.Output)
		return
	}
}

func NewVolumesAddCmd(newClient ClientFactory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "add",
		Short: "Add volume to the function",
		Long: `Add volume to the function

Add secrets and config maps as volume mounts to the function in the current
directory or from the directory specified by --path.  If no flags are provided,
addition is completed using interactive prompts.

The volume can be set 
`,
		SuggestFor: []string{"ad", "create", "insert", "append"},
		PreRunE:    bindEnv("verbose", "path", "configmap", "secret", "mount"),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runVolumesAdd(cmd, newClient)
		},
	}
	cfg, err := config.NewDefault()
	if err != nil {
		fmt.Fprintf(cmd.OutOrStdout(), "error loading config at '%v'. %v\n", config.File(), err)
	}
	addPathFlag(cmd)
	addVerboseFlag(cmd, cfg.Verbose)
	cmd.Flags().StringP("configmap", "", "", "Name of the config map to mount.")
	cmd.Flags().StringP("secret", "", "", "Name of the secret to mount.")
	cmd.Flags().StringP("mount", "", "", "Mount path.")

	return cmd
}

func runVolumesAdd(cmd *cobra.Command, newClient ClientFactory) (err error) {
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

	secrets, err := k8s.ListSecretsNamesIfConnected(cmd.Context(), f.Deploy.Namespace)
	if err != nil {
		return
	}
	configMaps, err := k8s.ListConfigMapsNamesIfConnected(cmd.Context(), f.Deploy.Namespace)
	if err != nil {
		return
	}

	// TODO: the below implementation was created prior to global and local
	// configuration, or the general command structure used in other commands.
	// It is also a monolithic function.
	// As such it is in need of some refactoring:

	// Noninteractive:
	// ------------

	var sp *string
	var cp *string
	var vp *string

	if cmd.Flags().Changed("secret") {
		s, e := cmd.Flags().GetString("secret")
		if e != nil {
			return e
		}
		sp = &s
	}
	if cmd.Flags().Changed("configmap") {
		s, e := cmd.Flags().GetString("configmap")
		if e != nil {
			return e
		}
		cp = &s
	}
	if cmd.Flags().Changed("mount") {
		s, e := cmd.Flags().GetString("mount")
		if e != nil {
			return e
		}
		vp = &s
	}

	// Validators
	if cp != nil && sp != nil {
		return errors.New("Only one of Confg Map OR secret may be specified when adding a volume.")
	}
	if (cp != nil || sp != nil) && vp == nil {
		return errors.New("Path is required when either config map or secret name provided")
	}
	func() { // Warn if secret specified but not found
		if sp == nil {
			return
		}
		for _, v := range secrets {
			if v == *sp {
				return
			}
		}
		fmt.Fprintf(cmd.ErrOrStderr(), "Warning: the secret name '%v' was not found in the remote\n", *sp)
	}()
	func() { // Warn if config map specified but not found
		if cp == nil {
			return
		}
		for _, v := range configMaps {
			if v == *cp {
				return
			}
		}
		fmt.Fprintf(cmd.ErrOrStderr(), "Warning: the config map name '%v' was not found in the remote\n", *cp)
	}()

	// Add and rturn
	if cp != nil {
		f.Run.Volumes = append(f.Run.Volumes, fn.Volume{ConfigMap: cp, Path: vp})
		return f.Write()
	} else if sp != nil {
		f.Run.Volumes = append(f.Run.Volumes, fn.Volume{Secret: sp, Path: vp})
		return f.Write()
	}

	// Interactive:
	// ------------

	// SECTION - select resource type to be mounted
	options := []string{}
	selectedOption := ""
	const optionConfigMap = "ConfigMap"
	const optionSecret = "Secret"

	if len(configMaps) > 0 {
		options = append(options, optionConfigMap)
	}
	if len(secrets) > 0 {
		options = append(options, optionSecret)
	}

	switch len(options) {
	case 0:
		fmt.Printf("There aren't any Secrets or ConfiMaps in the namespace \"%s\"\n", f.Deploy.Namespace)
		return
	case 1:
		selectedOption = options[0]
	case 2:
		err = survey.AskOne(&survey.Select{
			Message: "What do you want to mount as a Volume?",
			Options: options,
		}, &selectedOption)
		if err != nil {
			return
		}
	}

	// SECTION - select the specific resource to be mounted
	optionsResoures := []string{}
	resourceType := ""
	switch selectedOption {
	case optionConfigMap:
		resourceType = optionConfigMap
		optionsResoures = configMaps
	case optionSecret:
		resourceType = optionSecret
		optionsResoures = secrets
	}

	selectedResource := ""
	err = survey.AskOne(&survey.Select{
		Message: fmt.Sprintf("Which \"%s\" do you want to mount?", resourceType),
		Options: optionsResoures,
	}, &selectedResource)
	if err != nil {
		return
	}

	// SECTION - specify mount Path of the Volume

	path := ""
	err = survey.AskOne(&survey.Input{
		Message: fmt.Sprintf("Please specify the path where the %s should be mounted:", resourceType),
	}, &path, survey.WithValidator(func(val interface{}) error {
		if str, ok := val.(string); !ok || len(str) <= 0 || !strings.HasPrefix(str, "/") {
			return fmt.Errorf("The input must be non-empty absolute path.")
		}
		return nil
	}))
	if err != nil {
		return
	}

	// we have all necessary information -> let's store the new Volume
	newVolume := fn.Volume{Path: &path}
	switch selectedOption {
	case optionConfigMap:
		newVolume.ConfigMap = &selectedResource
	case optionSecret:
		newVolume.Secret = &selectedResource
	}

	f.Run.Volumes = append(f.Run.Volumes, newVolume)

	err = f.Write()
	if err == nil {
		fmt.Println("Volume entry was added to the function configuration")
	}

	return
}

func NewVolumesRemoveCmd(newClient ClientFactory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "remove",
		Short: "Remove volume from the function configuration",
		Long: `Remove volume from the function configuration

Interactive prompt to remove Volume mounts from the function project
in the current directory or from the directory specified with --path.
`,
		Aliases:    []string{"rm"},
		SuggestFor: []string{"del", "delete", "rmeove"},
		PreRunE:    bindEnv("verbose", "path"),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runVolumesRemove(cmd, newClient)
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

func runVolumesRemove(cmd *cobra.Command, newClient ClientFactory) (err error) {
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
