package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/ory/viper"
	"github.com/spf13/cobra"

	"knative.dev/func/pkg/config"
	"knative.dev/func/pkg/functions"
	"knative.dev/func/pkg/k8s"
)

func NewEnvironmentCmd(newClient ClientFactory, version *Version) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "environment",
		Short: "Display function execution environment information",
		Long: `
NAME
	{{rootCmdUse}} environment

SYNOPSIS
	{{rootCmdUse}} environment - display function execution environment information

DESCRIPTION
	Display information about the function execution environment, including
	the version of func, the version of the function spec, the default builder,
	available runtimes, and available templates.
`,
		SuggestFor: []string{"env", "environemtn", "enviroment", "enviornment", "enviroment"},
		RunE: func(cmd *cobra.Command, args []string) error {
			return runEnvironment(cmd, newClient, version)
		},
	}
	cfg, err := config.NewDefault()
	if err != nil {
		fmt.Fprintf(cmd.OutOrStdout(), "error loading config at '%v'. %v\n", config.File(), err)
	}

	addVerboseFlag(cmd, cfg.Verbose)

	return cmd
}

type Environment struct {
	Version     string
	Build       string
	SpecVersion string
	SocatImage  string
	TarImage    string
	Languages   []string
	Templates   map[string][]string
	Defaults    config.Global
}

func runEnvironment(cmd *cobra.Command, newClient ClientFactory, v *Version) (err error) {
	cfg, err := newEnvironmentConfig()
	if err != nil {
		return
	}

	client := functions.New(functions.WithVerbose(cfg.Verbose))
	r, err := getRuntimes(client)
	if err != nil {
		return
	}
	t, err := getTemplates(client, r)
	if err != nil {
		return
	}

	defaults, err := config.NewDefault()
	if err != nil {
		fmt.Fprintf(cmd.OutOrStdout(), "error loading config at '%v'. %v\n", config.File(), err)
	}

	environment := Environment{
		Version:     v.String(),
		Build:       v.Hash,
		SpecVersion: functions.LastSpecVersion(),
		SocatImage:  k8s.SocatImage,
		TarImage:    k8s.TarImage,
		Languages:   r,
		Templates:   t,
		Defaults:    defaults,
	}

	if s, err := json.MarshalIndent(environment, "", "  "); err != nil {
		return err
	} else {
		fmt.Fprintln(cmd.OutOrStdout(), string(s))
	}

	return nil
}

func getRuntimes(client *functions.Client) ([]string, error) {
	runtimes, err := client.Runtimes()
	if err != nil {
		return nil, err
	}
	return runtimes, nil
}

func getTemplates(client *functions.Client, runtimes []string) (map[string][]string, error) {
	templateMap := make(map[string][]string)
	for _, runtime := range runtimes {
		templates, err := client.Templates().List(runtime)
		if err != nil {
			return nil, err
		}
		templateMap[runtime] = templates
	}
	return templateMap, nil
}

type environmentConfig struct {
	Verbose bool
	// TODO: add format (e.g. JSON/YAML)
}

func newEnvironmentConfig() (cfg environmentConfig, err error) {
	cfg = environmentConfig{
		Verbose: viper.GetBool("verbose"),
	}

	return
}
