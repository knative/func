package ci

import "path/filepath"

type CIConfig struct {
	githubWorkflowDir,
	githubWorkflowFilename,
	workflowName,
	kubeconfigSecretKey string
	selfHostedRunner bool
}

func NewDefaultCIConfig() CIConfig {
	return NewCIConfig(
		".github/workflows",
		"remote-build-and-deploy.yaml",
		"Remote Build and Deploy",
		"KUBECONFIG",
		false,
	)
}

func NewCIConfig(
	workflowDir,
	workflowFilename,
	workflowName,
	kubeconfigSecretKey string,
	selfHostedRunner bool) CIConfig {
	return CIConfig{
		workflowDir,
		workflowFilename,
		workflowName,
		kubeconfigSecretKey,
		selfHostedRunner,
	}
}

func (cc *CIConfig) FnGithubWorkflowDir(fnRoot string) string {
	return filepath.Join(fnRoot, cc.githubWorkflowDir)
}

func (cc *CIConfig) FnGithubWorkflowFilepath(fnRoot string) string {
	return filepath.Join(cc.FnGithubWorkflowDir(fnRoot), cc.githubWorkflowFilename)
}

func (cc *CIConfig) WorkflowName() string {
	return cc.workflowName
}

func (cc *CIConfig) SelfHostedRunner() bool {
	return cc.selfHostedRunner
}

func (cc *CIConfig) KubeconfigSecretKey() string {
	return cc.kubeconfigSecretKey
}
