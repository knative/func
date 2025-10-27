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
	return NewCIConfig(
		".github/workflows",
		"remote-build-and-deploy.yaml",
	)
}

func NewCIConfig(workflowDir, workflowFile string) CIConfig {
	return CIConfig{
		workflowDir,
		workflowFile,
		0755, // o: rwx, g|u: r-x
		0644, // o: rw,  g|u: r
	}
}

type GithubWorkflow struct {
	Name string           `yaml:"name"`
	On   WorkflowTriggers `yaml:"on"`
	Jobs map[string]Job   `yaml:"jobs"`
}

type WorkflowTriggers struct {
	Push *PushTrigger `yaml:"push,omitempty"`
}

type PushTrigger struct {
	Branches []string `yaml:"branches,omitempty"`
}

type Job struct {
	RunsOn string `yaml:"runs-on"`
	Steps  []Step `yaml:"steps"`
}

type Step struct {
	Name string            `yaml:"name,omitempty"`
	Uses string            `yaml:"uses,omitempty"`
	Run  string            `yaml:"run,omitempty"`
	With map[string]string `yaml:"with,omitempty"`
}

func runConfigCIGithub(
	fnLoaderSaver common.FunctionLoaderSaver,
	ciConfig CIConfig,
) error {
	f, err := initConfigCommand(fnLoaderSaver)
	if err != nil {
		return err
	}

	fnWorkflowDirPath := filepath.Join(f.Root, ciConfig.GithubWorkflowDir)
	if err := os.MkdirAll(fnWorkflowDirPath, ciConfig.dirPerm); err != nil {
		return err
	}

	workflowYamlContent := `name: Remote Build and Deploy

on:
  push:
    branches:
      - main

jobs:
  deploy:
    runs-on: ubuntu-latest
    steps:
    - name: Checkout code
      uses: actions/checkout@v4

    - name: Install func cli
      uses: gauron99/knative-func-action@main
      with:
        version: knative-v1.19.1
        name: func
    
    - name: Deploy function
      run: func deploy --remote`
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
