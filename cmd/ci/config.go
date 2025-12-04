package ci

import (
	"path/filepath"

	"github.com/ory/viper"
)

const (
	ConfigCIFeatureFlag = "FUNC_ENABLE_CI_CONFIG"

	// TODO(twoGiants): *Option -> *Flag
	PathOption = "path"

	DefaultGithubWorkflowDir      = ".github/workflows"
	DefaultGithubWorkflowFilename = "func-deploy.yaml"

	BranchOption  = "branch"
	DefaultBranch = "main"

	WorkflowNameOption  = "workflow-name"
	DefaultWorkflowName = "Func Deploy"

	KubeconfigSecretNameOption  = "kubeconfig-secret-name"
	DefaultKubeconfigSecretName = "KUBECONFIG"

	RegistryLoginUrlVariableNameOption  = "registry-login-url-variable-name"
	DefaultRegistryLoginUrlVariableName = "REGISTRY_LOGIN_URL"

	RegistryUserVariableNameOption  = "registry-user-variable-name"
	DefaultRegistryUserVariableName = "REGISTRY_USERNAME"

	RegistryPassSecretNameOption  = "registry-pass-secret-name"
	DefaultRegistryPassSecretName = "REGISTRY_PASSWORD"

	RegistryUrlVariableNameOption  = "registry-url-variable-name"
	DefaultRegistryUrlVariableName = "REGISTRY_URL"

	UseRegistryLoginOption  = "use-registry-login"
	DefaultUseRegistryLogin = true

	UseDebugOption  = "debug"
	DefaultUseDebug = false

	UseRemoteBuild        = "remote"
	DefaultUseRemoteBuild = false

	UseSelfHostedRunner        = "self-hosted-runner"
	DefaultUseSelfHostedRunner = false
)

// CIConfig readonly configuration
type CIConfig struct {
	githubWorkflowDir,
	githubWorkflowFilename,
	path,
	branch,
	workflowName,
	kubeconfigSecret,
	registryLoginUrlVar,
	registryUserVar,
	registryPassSecret,
	registryUrlVar string
	useRegistryLogin,
	useRemoteBuild,
	useSelfHostedRunner,
	debug bool
}

func NewCiGithubConfig() CIConfig {
	return CIConfig{
		githubWorkflowDir:      DefaultGithubWorkflowDir,
		githubWorkflowFilename: DefaultGithubWorkflowFilename,
		path:                   viper.GetString(PathOption),
		branch:                 viper.GetString(BranchOption),
		workflowName:           viper.GetString(WorkflowNameOption),
		kubeconfigSecret:       viper.GetString(KubeconfigSecretNameOption),
		registryLoginUrlVar:    viper.GetString(RegistryLoginUrlVariableNameOption),
		registryUserVar:        viper.GetString(RegistryUserVariableNameOption),
		registryPassSecret:     viper.GetString(RegistryPassSecretNameOption),
		registryUrlVar:         viper.GetString(RegistryUrlVariableNameOption),
		useRegistryLogin:       viper.GetBool(UseRegistryLoginOption),
		useRemoteBuild:         viper.GetBool(UseRemoteBuild),
		useSelfHostedRunner:    viper.GetBool(UseSelfHostedRunner),
		debug:                  viper.GetBool(UseDebugOption),
	}
}

func (cc *CIConfig) FnGithubWorkflowDir(fnRoot string) string {
	return filepath.Join(fnRoot, cc.githubWorkflowDir)
}

func (cc *CIConfig) FnGithubWorkflowFilepath(fnRoot string) string {
	return filepath.Join(cc.FnGithubWorkflowDir(fnRoot), cc.githubWorkflowFilename)
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

func (cc *CIConfig) UseRegistryLogin() bool {
	return cc.useRegistryLogin
}

func (cc *CIConfig) UseRemoteBuild() bool {
	return cc.useRemoteBuild
}

func (cc *CIConfig) UseSelfHostedRunner() bool {
	return cc.useSelfHostedRunner
}

func (cc *CIConfig) UseDebug() bool {
	return cc.debug
}

func (cc *CIConfig) KubeconfigSecret() string {
	return cc.kubeconfigSecret
}

func (cc *CIConfig) RegistryLoginUrlVar() string {
	return cc.registryLoginUrlVar
}

func (cc *CIConfig) RegistryUserVar() string {
	return cc.registryUserVar
}

func (cc *CIConfig) RegistryPassSecret() string {
	return cc.registryPassSecret
}

func (cc *CIConfig) RegistryUrlVar() string {
	return cc.registryUrlVar
}
