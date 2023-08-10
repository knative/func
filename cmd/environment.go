package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/ory/viper"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v2"

	"knative.dev/func/pkg/builders/buildpacks"
	"knative.dev/func/pkg/builders/s2i"
	"knative.dev/func/pkg/config"
	"knative.dev/func/pkg/functions"
	"knative.dev/func/pkg/k8s"
	"knative.dev/func/pkg/pipelines/tekton"
)

var format string = "json"

func NewEnvironmentCmd(newClient ClientFactory, version *Version) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "environment",
		Short: "Display function execution environment information",
		Long: `
NAME
	{{rootCmdUse}} environment - display function execution environment information

SYNOPSIS
	{{rootCmdUse}} environment [-e|--env-format] [-v|--verbose]


DESCRIPTION
	Display information about the function execution environment, including
	the version of func, the version of the function spec, the default builder,
	available runtimes, and available templates.
`,
		SuggestFor: []string{"env", "environemtn", "enviroment", "enviornment", "enviroment"},
		PreRunE:    bindEnv("verbose", "format"),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runEnvironment(cmd, newClient, version)
		},
	}
	cfg, err := config.NewDefault()
	if err != nil {
		fmt.Fprintf(cmd.OutOrStdout(), "error loading config at '%v'. %v\n", config.File(), err)
	}

	cmd.Flags().StringP("format", "f", format, "Format of output environment information, 'json' or 'yaml'. ($FUNC_FORMAT)")
	addVerboseFlag(cmd, cfg.Verbose)

	return cmd
}

type Environment struct {
	Version              string
	GitRevision          string
	SpecVersion          string
	SocatImage           string
	TarImage             string
	Languages            []string
	DefaultImageBuilders map[string]map[string]string
	Templates            map[string][]string
	Environment          []string
	Cluster              string
	TektonTasks          map[string]string
	Defaults             config.Global
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

	//Get default image builders
	builderimagesdefault := make(map[string]map[string]string)
	builderimagesdefault["s2i"] = s2i.DefaultBuilderImages
	builderimagesdefault["buildpacks"] = buildpacks.DefaultBuilderImages

	environment := Environment{
		Version:              v.String(),
		GitRevision:          v.Hash,
		SpecVersion:          functions.LastSpecVersion(),
		SocatImage:           k8s.SocatImage,
		TarImage:             k8s.TarImage,
		Languages:            r,
		DefaultImageBuilders: builderimagesdefault,
		Templates:            t,
		Environment:          envs,
		Cluster:              host,
		Defaults:             defaults,
		TektonTasks: map[string]string{
			"func-buildpack": tekton.BuildpackTaskURL,
			"func-s2i":       tekton.S2ITaskURL,
			"func-deploy":    tekton.DeployTaskURL,
		},
	}

	var s []byte
	switch cfg.Format {
	case "json":
		s, err = json.MarshalIndent(environment, "", "  ")
	case "yaml":
		s, err = yaml.Marshal(&environment)
	default:
		err = fmt.Errorf("unsupported format: %s", cfg.Format)
	}
	if err != nil {
		return err
	}
	fmt.Fprintln(cmd.OutOrStdout(), string(s))

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
	Format  string
}

func newEnvironmentConfig() (cfg environmentConfig, err error) {
	cfg = environmentConfig{
		Verbose: viper.GetBool("verbose"),
		Format:  viper.GetString("format"),
	}

	return
}
