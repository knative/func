package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

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
	GitRevision string
	BuildDate   string
	SpecVersion string
	SocatImage  string
	TarImage    string
	Languages   []string
	Templates   map[string][]string
	Environment []string
	Cluster     string
	Defaults    config.Global
}

func runEnvironment(cmd *cobra.Command, newClient ClientFactory, v *Version) (err error) {
	cfg, err := newEnvironmentConfig()
	if err != nil {
		return
	}

	// Create a client to get runtimes and templates
	client := functions.New(functions.WithVerbose(cfg.Verbose))

	r, err := getRuntimes(client)
	if err != nil {
		return
	}
	t, err := getTemplates(client, r)
	if err != nil {
		return
	}

	// Get all environment variables that start with FUNC_
	var envs []string
	for _, e := range os.Environ() {
		if strings.HasPrefix(e, "FUNC_") {
			envs = append(envs, e)
		}
	}

	// If no environment variables are set, make sure we return an empty array
	// otherwise the output is "null" instead of "[]"
	if len(envs) == 0 {
		envs = make([]string, 0)
	}

	// Get global defaults
	defaults, err := config.NewDefault()
	if err != nil {
		return
	}

	// Gets the cluster host
	var host string
	cc, err := k8s.GetClientConfig().ClientConfig()
	if err != nil {
		fmt.Printf("error getting client config %v\n", err)
	} else {
		host = cc.Host
	}

	environment := Environment{
		Version:     v.String(),
		GitRevision: v.Hash,
		BuildDate:   v.Date,
		SpecVersion: functions.LastSpecVersion(),
		SocatImage:  k8s.SocatImage,
		TarImage:    k8s.TarImage,
		Languages:   r,
		Templates:   t,
		Environment: envs,
		Cluster:     host,
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
