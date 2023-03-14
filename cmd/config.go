package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/AlecAivazis/survey/v2"
	"github.com/ory/viper"
	"github.com/spf13/cobra"
	"knative.dev/func/pkg/config"
)

func NewConfigCmd(newClient ClientFactory) *cobra.Command {
	cmd := &cobra.Command{
		Short: "Manage global system settings",
		Use:   "config [list|get|set|unset]",
		Long: `
NAME
	{{rootCmdUse}} - Manage global configuration

SYNOPSIS
	{{rootCmdUse}} config list [-o|--output] [-c|--confirm] [-v|--verbose]
	{{rootCmdUse}} config get <name> [-o|--output] [-c|--confirm] [-v|--verbose]
	{{rootCmdUse}} config set <name> <value> [-o|--output] [-c|--confirm] [-v|--verbose]
	{{rootCmdUse}} config unset <name> [-o|--output] [-c|--confirm] [-v|--verbose]

DESCRIPTION

	Manage global configuration by listing current configuration, and either
	retreiving individual settings with 'get' or setting with 'set'.
	Values set will become the new default for all function commands.

	Settings are stored in ~/.config/func/config.yaml by default, but whose path
	can be altered using the XDG_CONFIG_HOME environment variable (by default
	~/.config).

	A specific global config file can also be used by specifying the environment
	variable FUNC_CONFIG_FILE, which takes highest precidence.

	Values defined in this global configuration are only defaults.  If a given
	function has a defined value for the option, that will be used. This value
	can also be superceded with environment variables or command line flags.  To
	see the final value of an option that will be used for a given command, see
	the given command's help text.

COMMANDS

	With no arguments, this help text is shown.  To manage global configuration
	using an interactive prompt, use the --confirm (-c) flag.
	  $ {{rootCmdUse}} config -c

	list
	  List all global configuration options and their current values.

	get
	  Get the current value of a named global configuration option.

	set
	  Set the named global configuration option to the value provided.

	unset
	  Remove any user-provided setting for the given option, resetting it to the
	  original default.

EXAMPLES
	o List all configuration options and their current values
	  $ {{rootCmdUse}} config list

	o Set a new global default value for Reggistry
	  $ {{rootCmdUse}} config set registry registry.example.com/bob

	o Get the current default value for Registry
	  $ {{rootCmdUse}} config get registry
		registry.example.com/bob

	o Unset the custom global registry default, reverting it to the static
	  default (aka factory default).  In the case of registry, there is no
	  static default, so unsetting the default will result in the 'deploy'
	  command requiring it to be provided when invoked.
	  $ {{rootCmdUse}} config unset registry

`,
		SuggestFor: []string{"cnofig", "donfig", "vonfig", "xonfig", "cinfig", "clnfig", "cpnfig"},
		PreRunE:    bindEnv("confirm", "output", "verbose"),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runConfigCmd(cmd, args, newClient)
		},
	}

	cfg, err := config.NewDefault()
	if err != nil {
		fmt.Fprintf(cmd.OutOrStdout(), "error loading config at '%v'. %v\n", config.File(), err)
	}

	addConfirmFlag(cmd, cfg.Confirm)
	addOutputFlag(cmd, cfg.Output)
	addVerboseFlag(cmd, cfg.Verbose)

	cmd.AddCommand(NewConfigListCmd(newClient, cfg))
	cmd.AddCommand(NewConfigGetCmd(newClient, cfg))
	cmd.AddCommand(NewConfigSetCmd(newClient, cfg))
	cmd.AddCommand(NewConfigUnsetCmd(newClient, cfg))

	return cmd
}

func NewConfigListCmd(newClient ClientFactory, cfg config.Global) *cobra.Command {
	cmd := &cobra.Command{
		Short:   "List global configuration options",
		Use:     "list",
		PreRunE: bindEnv("confirm", "output", "verbose"),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runConfigList(cmd, args, newClient)
		},
	}
	addConfirmFlag(cmd, cfg.Confirm)
	addOutputFlag(cmd, cfg.Output)
	addVerboseFlag(cmd, cfg.Verbose)
	return cmd
}

func NewConfigGetCmd(newClient ClientFactory, cfg config.Global) *cobra.Command {
	cmd := &cobra.Command{
		Short:   "Get a global configuration option",
		Use:     "get <name>",
		PreRunE: bindEnv("confirm", "output", "verbose"),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runConfigGet(cmd, args, newClient)
		},
	}
	addConfirmFlag(cmd, cfg.Confirm)
	addOutputFlag(cmd, cfg.Output)
	addVerboseFlag(cmd, cfg.Verbose)
	return cmd
}

func NewConfigSetCmd(newClient ClientFactory, cfg config.Global) *cobra.Command {
	cmd := &cobra.Command{
		Short:   "Get a global configuration option",
		Use:     "set <name> <value>",
		PreRunE: bindEnv("confirm", "output", "verbose"),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runConfigSet(cmd, args, newClient)
		},
	}
	addConfirmFlag(cmd, cfg.Confirm)
	addOutputFlag(cmd, cfg.Output)
	addVerboseFlag(cmd, cfg.Verbose)
	return cmd
}

func NewConfigUnsetCmd(newClient ClientFactory, cfg config.Global) *cobra.Command {
	cmd := &cobra.Command{
		Short:   "Get a global configuration option",
		Use:     "unset <name>",
		PreRunE: bindEnv("confirm", "output", "verbose"),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runConfigUnset(cmd, args, newClient)
		},
	}
	addConfirmFlag(cmd, cfg.Confirm)
	addOutputFlag(cmd, cfg.Output)
	addVerboseFlag(cmd, cfg.Verbose)
	return cmd
}

func runConfigCmd(cmd *cobra.Command, args []string, newClient ClientFactory) (err error) {
	var cfg configConfig // global configuration settings command config
	if cfg, err = newConfigConfig("", args).Prompt(); err != nil {
		return
	}

	if cfg.Action == "" {
		return cmd.Help()
	}

	switch cfg.Action {
	case "list":
		return runConfigList(cmd, args, newClient)
	case "get":
		return runConfigGet(cmd, args, newClient)
	case "set":
		return runConfigSet(cmd, args, newClient)
	case "unset":
		return runConfigUnset(cmd, args, newClient)
	default:
		return fmt.Errorf("invalid action '%v'", cfg.Action)
	}
}

func runConfigList(cmd *cobra.Command, args []string, newClient ClientFactory) (err error) {
	var cfg configConfig
	if cfg, err = newConfigConfig("list", args).Prompt(); err != nil {
		return
	}

	// Global Settings to Query
	settings, err := config.NewDefault()
	if err != nil {
		return fmt.Errorf("error loading config at '%v'. %v\n", config.File(), err)
	}

	switch Format(cfg.Output) {
	case Human:
		for _, v := range config.List() {
			fmt.Fprintf(cmd.OutOrStdout(), "%v=%v\n", v, config.Get(settings, v))
		}
		return
	case JSON:
		var bb []byte
		bb, err = json.MarshalIndent(settings, "", "  ")
		fmt.Fprintln(cmd.OutOrStdout(), string(bb))
		return
	default:
		return fmt.Errorf("invalid format: %v", cfg.Output)
	}
}

func runConfigGet(cmd *cobra.Command, args []string, newClient ClientFactory) (err error) {
	// Config
	var cfg configConfig
	if cfg, err = newConfigConfig("get", args).Prompt(); err != nil {
		return
	}

	// Preconditions
	if cfg.Name == "" {
		return fmt.Errorf("Setting name is requred: get <name>")
	}

	// Global Settings to Query
	settings, err := config.NewDefault()
	if err != nil {
		return fmt.Errorf("error loading config at '%v'. %v\n", config.File(), err)
	}

	// Output named attribute value as output type
	value := config.Get(settings, cfg.Name)
	switch Format(cfg.Output) {
	case Human:
		fmt.Fprintf(cmd.OutOrStdout(), "%v\n", value)
		return
	case JSON:
		var bb []byte
		bb, err = json.MarshalIndent(value, "", "  ")
		fmt.Fprintln(cmd.OutOrStdout(), string(bb))
		return
	default:
		return fmt.Errorf("invalid format: %v", cfg.Output)
	}

	return nil
}

func runConfigSet(cmd *cobra.Command, args []string, newClient ClientFactory) (err error) {
	// Config
	var cfg configConfig
	if cfg, err = newConfigConfig("set", args).Prompt(); err != nil {
		return
	}

	// Preconditions
	if cfg.Name == "" {
		return fmt.Errorf("Setting name is requred: set <name> <value>")
	}
	if cfg.Value == "" {
		return fmt.Errorf("Setting value is requred: set <name> <value>")
	}

	// Global Settings to Mutate
	settings, err := config.NewDefault()
	if err != nil {
		return fmt.Errorf("error loading config at '%v'. %v\n", config.File(), err)
	}

	// Set
	settings, err = config.Set(settings, cfg.Name, cfg.Value)
	if err != nil {
		return
	}
	return settings.Write(config.File())
}

func runConfigUnset(cmd *cobra.Command, args []string, newClient ClientFactory) (err error) {
	// Config
	var cfg configConfig
	if cfg, err = newConfigConfig("unset", args).Prompt(); err != nil {
		return
	}

	// Preconditions
	if cfg.Name == "" {
		return fmt.Errorf("Setting name is requred: unset <name>")
	}

	defaults := config.New()             // static defaults
	settings, err := config.NewDefault() // customized
	if err != nil {
		return fmt.Errorf("error loading config at '%v'. %v\n", config.File(), err)
	}

	// Reset
	// The setter should alwasy be able to accept the string serialized value
	// of the default value returned.
	defaultValue := config.Get(defaults, cfg.Name)
	settings, err = config.Set(settings, cfg.Name, fmt.Sprintf("%s", defaultValue))
	if err != nil {
		return
	}
	return settings.Write(config.File())
}

type configConfig struct {
	config.Global
	Action string
	Name   string
	Value  string
}

func newConfigConfig(action string, args []string) configConfig {
	cfg := configConfig{
		Global: config.Global{
			Confirm: viper.GetBool("confirm"),
			Verbose: viper.GetBool("verbose"),
			Output:  viper.GetString("output"),
		},
		Action: action,
	}

	if action != "" {
		cfg.Action = action
		if len(args) > 0 {
			cfg.Name = args[0]
		}
		if len(args) > 1 {
			cfg.Value = args[1]
		}
	}

	return cfg
}

func (c configConfig) Prompt() (configConfig, error) {
	if !interactiveTerminal() || !c.Confirm {
		return c, nil
	}
	fmt.Printf("Prompt with c.Action =%v, c.Name=%v, c.Value=%v\n", c.Action, c.Name, c.Value)

	// TODO: validators

	// If action is not explicitly provided, this is the root command asking
	// which action to perform.
	if c.Action == "" {
		qs := []*survey.Question{{
			Name: "Action",
			Prompt: &survey.Select{
				Message: "Operation to perform:",
				Options: []string{"list", "get", "set", "unset"},
				Default: "list",
			}}}
		if err := survey.Ask(qs, &c); err != nil {
			return c, err
		}
		// The only way action can be empty is if we're configuring via the
		// root 'config' command, so just return.  the subcommand invoked
		// will re-run prompt to get the remaining values.
		return c, nil
	}

	// Prompt for 'name' if the action is 'get','set' or 'unset'
	if c.Name == "" && (c.Action == "get" || c.Action == "set" || c.Action == "unset") {
		qs := []*survey.Question{{
			Name: "Name",
			Prompt: &survey.Input{
				Message: "Name of Global Default:",
				Default: c.Name,
			}}}
		if err := survey.Ask(qs, &c); err != nil {
			return c, err
		}
	}

	// Prompt for 'value' the action is a 'set'
	if c.Value == "" && c.Action == "set" {
		qs := []*survey.Question{{
			Name: "Value",
			Prompt: &survey.Input{
				Message: "Global Default Value:",
				Default: c.Value,
			}}}
		if err := survey.Ask(qs, &c); err != nil {
			return c, err
		}
	}

	return c, nil
}
