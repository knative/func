package ci

import (
	"path/filepath"

	"github.com/ory/viper"
)

const (
	ConfigCIFeatureFlag = "FUNC_ENABLE_CI_CONFIG"

	PathFlag = "path"

	DefaultGitHubWorkflowDir      = ".github/workflows"
	DefaultGitHubWorkflowFilename = "func-deploy.yaml"

	WorkflowNameFlag    = "workflow-name"
	DefaultWorkflowName = "Func Deploy"

	BranchFlag    = "branch"
	DefaultBranch = "main"

	KubeconfigSecretNameFlag    = "kubeconfig-secret-name"
	DefaultKubeconfigSecretName = "KUBECONFIG"

	RegistryUrlVariableNameFlag    = "registry-url-variable-name"
	DefaultRegistryUrlVariableName = "REGISTRY_URL"
)

// CIConfig readonly configuration
type CIConfig struct {
	githubWorkflowDir,
	githubWorkflowFilename,
	path,
	workflowName,
	branch,
	kubeconfigSecret,
	registryUrlVar string
}

func NewCIGitHubConfig() CIConfig {
	return CIConfig{
		githubWorkflowDir:      DefaultGitHubWorkflowDir,
		githubWorkflowFilename: DefaultGitHubWorkflowFilename,
		path:                   viper.GetString(PathFlag),
		workflowName:           viper.GetString(WorkflowNameFlag),
		branch:                 viper.GetString(BranchFlag),
		kubeconfigSecret:       viper.GetString(KubeconfigSecretNameFlag),
		registryUrlVar:         viper.GetString(RegistryUrlVariableNameFlag),
	}
}

func (cc *CIConfig) FnGitHubWorkflowDir(fnRoot string) string {
	return filepath.Join(fnRoot, cc.githubWorkflowDir)
}

func (cc *CIConfig) FnGitHubWorkflowFilepath(fnRoot string) string {
	return filepath.Join(cc.FnGitHubWorkflowDir(fnRoot), cc.githubWorkflowFilename)
}

func (cc *CIConfig) Path() string {
	return cc.path
}

func (cc *CIConfig) WorkflowName() string {
	return cc.workflowName
}

func (cc *CIConfig) Branch() string {
	return cc.branch
}

func (cc *CIConfig) KubeconfigSecret() string {
	return cc.kubeconfigSecret
}

func (cc *CIConfig) RegistryUrlVar() string {
	return cc.registryUrlVar
}
