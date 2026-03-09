package github

import (
	"path/filepath"
)

const (
	DefaultPlatform                     = "github"
	DefaultGitHubWorkflowDir            = ".github/workflows"
	DefaultGitHubWorkflowFilename       = "func-deploy.yaml"
	DefaultBranch                       = "main"
	DefaultWorkflowName                 = "Func Deploy"
	DefaultRemoteBuildWorkflowName      = "Remote " + DefaultWorkflowName
	DefaultKubeconfigSecretName         = "KUBECONFIG"
	DefaultRegistryLoginUrlVariableName = "REGISTRY_LOGIN_URL"
	DefaultRegistryUserVariableName     = "REGISTRY_USERNAME"
	DefaultRegistryPassSecretName       = "REGISTRY_PASSWORD"
	DefaultRegistryUrlVariableName      = "REGISTRY_URL"
	DefaultRegistryLogin                = true
	DefaultWorkflowDispatch             = false
	DefaultRemoteBuild                  = false
	DefaultSelfHostedRunner             = false
	DefaultTestStep                     = true
	DefaultForce                        = false
)

type Config struct {
	GithubWorkflowDir,
	GithubWorkflowFilename,
	Branch,
	WorkflowName,
	KubeconfigSecret,
	RegistryLoginUrlVar,
	RegistryUserVar,
	RegistryPassSecret,
	RegistryUrlVar string
	RegistryLogin,
	SelfHostedRunner,
	RemoteBuild,
	WorkflowDispatch,
	TestStep,
	Force bool
	FnRuntime,
	FnRoot string
}

func (cc Config) FnGitHubWorkflowFilepath() string {
	fnGitHubWorkflowDir := filepath.Join(cc.FnRoot, cc.GithubWorkflowDir)
	return filepath.Join(fnGitHubWorkflowDir, cc.GithubWorkflowFilename)
}

func (cc Config) OutputPath() string {
	return filepath.Join(cc.GithubWorkflowDir, cc.GithubWorkflowFilename)
}
