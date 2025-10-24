package cmd

import (
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"knative.dev/func/cmd/common"
)

func NewConfigCICmd(loaderSaver common.FunctionLoaderSaver, ciConfig CIConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use: "ci",
		RunE: func(cmd *cobra.Command, args []string) (err error) {
			return runConfigCIGithub(loaderSaver, ciConfig)
		},
	}

	addGithubFlag(cmd)

	return cmd
}

type CIConfig struct {
	GithubWorkflowDir,
	GithubWorkflowFile string
	dirPerm,
	filePerm os.FileMode
}

func NewDefaultCIConfig() CIConfig {
	return newCIConfig(
		".github/workflows",
		"local-build-remote-deploy.yaml",
	)
}

func newCIConfig(workflowDir, workflowFile string) CIConfig {
	return CIConfig{
		workflowDir,
		workflowFile,
		0755, // o: rwx, g|u: r-x
		0644, // o: rw,  g|u: r
	}
}

func runConfigCIGithub(
	loaderSaver common.FunctionLoaderSaver,
	ciConfig CIConfig,
) error {
	f, err := initConfigCommand(loaderSaver)
	if err != nil {
		return err
	}

	fnWorkflowDirPath := filepath.Join(f.Root, ciConfig.GithubWorkflowDir)
	if err := os.MkdirAll(fnWorkflowDirPath, ciConfig.dirPerm); err != nil {
		return err
	}

	workflowYamlContent := "hello world"
	fnWorkflowYamlPath := filepath.Join(fnWorkflowDirPath, ciConfig.GithubWorkflowFile)
	if err := os.WriteFile(fnWorkflowYamlPath, []byte(workflowYamlContent), ciConfig.filePerm); err != nil {
		return err
	}

	return nil
}

func addGithubFlag(cmd *cobra.Command) {
	cmd.Flags().BoolP(
		"github",
		"",
		false,
		"Generate GitHub Action ci workflow",
	)
}
