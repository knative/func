package cmd

import (
	"github.com/ory/viper"
	"github.com/spf13/cobra"
)

func NewPipelineCmd(newClient ClientFactory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "pipeline",
		Short: "Apply/export pipeline definitions",
		Long: `Apply/export build pipeline definition

Pipeline definitions are meant to be versioned with the function files`,
		Example: `# Export existing pipeline data to local file
{{.Name}} pipeline --export`,
		SuggestFor: []string{"pipline", "pepline", "peipline"},
		PreRunE:    bindEnv("export", "path", "namespace"),
	}

	cmd.Flags().BoolP("export", "e", false, "Export cluster pipeline to a local file")
	cmd.Flags().StringP("namespace", "n", "default", "The namespace where the pipeline is located")
	setPathFlag(cmd)

	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		return runPipeline(cmd, args, newClient)
	}
	return cmd
}

func runPipeline(cmd *cobra.Command, args []string, newClient ClientFactory) (err error) {
	config := newPipelineConfig()
	client := newClient(ClientOptions{})
	if config.export {
		return client.Export(cmd.Context(), config.path, config.namespace)
	}
	return nil
}

func newPipelineConfig() pipelineConfig {
	return pipelineConfig{
		export:    viper.GetBool("export"),
		path:      viper.GetString("path"),
		namespace: viper.GetString("namespace"),
	}
}

type pipelineConfig struct {
	export    bool
	namespace string
	path      string
}
