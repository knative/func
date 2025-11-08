package ci

import "path/filepath"

type CIConfig struct {
	githubWorkflowDir,
	githubWorkflowFile,
	workflowName string
}

func NewDefaultCIConfigWithName(name string) CIConfig {
	result := NewCIConfig(
		".github/workflows",
		"remote-build-and-deploy.yaml",
		"Remote Build and Deploy",
	)
	result.workflowName = name

	return result
}

func NewDefaultCIConfig() CIConfig {
	return NewCIConfig(
		".github/workflows",
		"remote-build-and-deploy.yaml",
		"Remote Build and Deploy",
	)
}

func NewCIConfig(workflowDir, workflowFile, workflowName string) CIConfig {
	return CIConfig{
		workflowDir,
		workflowFile,
		workflowName,
	}
}

func (cc *CIConfig) FnGithubWorkflowDir(fnRoot string) string {
	return filepath.Join(fnRoot, cc.githubWorkflowDir)
}

func (cc *CIConfig) FnGithubWorkflowFilepath(fnRoot string) string {
	return filepath.Join(cc.FnGithubWorkflowDir(fnRoot), cc.githubWorkflowFile)
}

func (cc *CIConfig) WorkflowName() string {
	return cc.workflowName
}
