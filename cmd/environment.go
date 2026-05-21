package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/ory/viper"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v2"

	"knative.dev/func/pkg/buildpacks"
	"knative.dev/func/pkg/config"
	fn "knative.dev/func/pkg/functions"
	"knative.dev/func/pkg/k8s"
	"knative.dev/func/pkg/pipelines/tekton"
	"knative.dev/func/pkg/s2i"
)

var format string = "json"

func NewEnvironmentCmd(newClient ClientFactory, version *Version) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "environment",
		Aliases: []string{"env"},
		Short:   "Display function execution environment information",
		Long: `
NAME
	{{rootCmdUse}} environment (env) - display function execution environment information

SYNOPSIS
	{{rootCmdUse}} environment [-f|--format] [-v|--verbose] [-p|--path]


DESCRIPTION
	Display information about the function execution environment, including
	the version of func, the version of the function spec, the default builder,
	available runtimes, and available templates.
`,
		PreRunE: bindEnv("verbose", "format", "path"),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runEnvironment(cmd, newClient, version)
		},
	}
	cfg, err := config.NewDefault()
	if err != nil {
		fmt.Fprintf(cmd.OutOrStdout(), "error loading config at '%v'. %v\n", config.File(), err)
	}

	cmd.Flags().StringP("format", "f", format, "Format of output environment information, 'json' or 'yaml'. ($FUNC_FORMAT)")
	addPathFlag(cmd)
	addVerboseFlag(cmd, cfg.Verbose)

	return cmd
}

type Environment struct {
	Version              string
	GitRevision          string
	SpecVersion          string
	SocatImage           string
	TarImage             string
	FuncUtilsImage       string
	DeployerImage        string
	ScaffoldImage        string
	S2IImage             string
	Languages            []string
	DefaultImageBuilders map[string]map[string]string
	Templates            map[string][]string
	Environment          []string
	Cluster              string
	Defaults             config.Global
	Function             *fn.Function `json:",omitempty" yaml:",omitempty"`
	Instance             *fn.Instance `json:",omitempty" yaml:",omitempty"`
}

func runEnvironment(cmd *cobra.Command, newClient ClientFactory, v *Version) (err error) {
	cfg, err := newEnvironmentConfig()
	if err != nil {
		return
	}

	// Create a client to get runtimes and templates
	client := fn.New(fn.WithVerbose(cfg.Verbose))

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

	f, _ := fn.NewFunction(cfg.Path)

	// Gets the cluster host — use the function's stored cluster if available
	var host string
	envKc, kcErr := newK8sClientFromConfig(f.Deploy.Cluster, "", f.Deploy.Namespace, f.Local)
	if kcErr == nil {
		restCfg, clientErr := envKc.ClientConfig()
		if clientErr == nil {
			host = restCfg.Host
		}
	}

	//Get default image builders
	builderimagesdefault := make(map[string]map[string]string)
	builderimagesdefault["s2i"] = s2i.DefaultBuilderImages
	builderimagesdefault["buildpacks"] = buildpacks.DefaultBuilderImages

	environment := Environment{
		Version:              v.String(),
		GitRevision:          v.Hash,
		SpecVersion:          fn.LastSpecVersion(),
		SocatImage:           k8s.SocatImage,
		TarImage:             k8s.TarImage,
		FuncUtilsImage:       tekton.FuncUtilImage,
		DeployerImage:        tekton.DeployerImage,
		ScaffoldImage:        tekton.ScaffoldImage,
		S2IImage:             tekton.S2IImage,
		Languages:            r,
		DefaultImageBuilders: builderimagesdefault,
		Templates:            t,
		Environment:          envs,
		Cluster:              host,
		Defaults:             defaults,
	}

	if f.Initialized() {
		environment.Function = &f
		if instance := describeFuncInformation(cmd.Context(), f, envKc, newClient, cfg); instance != nil {
			environment.Instance = instance
		}
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

func getRuntimes(client *fn.Client) ([]string, error) {
	runtimes, err := client.Runtimes()
	if err != nil {
		return nil, err
	}
	return runtimes, nil
}

func getTemplates(client *fn.Client, runtimes []string) (map[string][]string, error) {
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

func describeFuncInformation(ctx context.Context, f fn.Function, kc *k8s.Client, newClient ClientFactory, cfg environmentConfig) *fn.Instance {
	client, done := newClient(ClientConfig{Verbose: cfg.Verbose, K8sClient: kc})
	defer done()
	instance, err := client.Describe(ctx, f.Name, f.Deploy.Namespace, f)
	if err != nil {
		return nil
	}
	return &instance
}

type environmentConfig struct {
	Verbose bool
	Format  string
	Path    string
}

func newEnvironmentConfig() (cfg environmentConfig, err error) {
	cfg = environmentConfig{
		Verbose: viper.GetBool("verbose"),
		Format:  viper.GetString("format"),
		Path:    viper.GetString("path"),
	}
	return
}
