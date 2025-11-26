package ci

import (
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// CIConfig readonly configuration
type CIConfig struct {
	githubWorkflowDir,
	githubWorkflowFilename,
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

func (cc *CIConfig) FnGithubWorkflowDir(fnRoot string) string {
	return filepath.Join(fnRoot, cc.githubWorkflowDir)
}

func (cc *CIConfig) FnGithubWorkflowFilepath(fnRoot string) string {
	return filepath.Join(cc.FnGithubWorkflowDir(fnRoot), cc.githubWorkflowFilename)
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

type ciConfigBuilder struct {
	result CIConfig
}

func NewCIConfigBuilder() *ciConfigBuilder {
	return &ciConfigBuilder{
		result: CIConfig{
			githubWorkflowDir:      DefaultGithubWorkflowDir,
			githubWorkflowFilename: DefaultGithubWorkflowFilename,
			branch:                 DefaultBranch,
			workflowName:           DefaultWorkflowName,
			kubeconfigSecret:       DefaultKubeconfigSecretName,
			registryLoginUrlVar:    DefaultRegistryLoginUrlVariableName,
			registryUserVar:        DefaultRegistryUserVariableName,
			registryPassSecret:     DefaultRegistryPassSecretName,
			registryUrlVar:         DefaultRegistryUrlVariableName,
			useRegistryLogin:       DefaultUseRegistryLogin,
			useRemoteBuild:         DefaultUseRemoteBuild,
			useSelfHostedRunner:    DefaultUseSelfHostedRunner,
			debug:                  DefaultUseDebug,
		},
	}
}

func (b *ciConfigBuilder) WithWorkflowName(name string) *ciConfigBuilder {
	b.result.workflowName = name
	return b
}

func (b *ciConfigBuilder) WithBranch(v string) *ciConfigBuilder {
	b.result.branch = v
	return b
}

func (b *ciConfigBuilder) WithKubeconfigSecret(v string) *ciConfigBuilder {
	b.result.kubeconfigSecret = v
	return b
}

func (b *ciConfigBuilder) WithRegistryLoginUrlVar(v string) *ciConfigBuilder {
	b.result.registryLoginUrlVar = v
	return b
}

func (b *ciConfigBuilder) WithRegistryUrlVar(v string) *ciConfigBuilder {
	b.result.registryUrlVar = v
	return b
}

func (b *ciConfigBuilder) WithRegistryUserVar(v string) *ciConfigBuilder {
	b.result.registryUserVar = v
	return b
}

func (b *ciConfigBuilder) WithRegistryPassSecret(v string) *ciConfigBuilder {
	b.result.registryPassSecret = v
	return b
}

func (b *ciConfigBuilder) WithoutRegistryLogin() *ciConfigBuilder {
	b.result.useRegistryLogin = false
	return b
}

func (b *ciConfigBuilder) WithRemoteBuild() *ciConfigBuilder {
	b.result.useRemoteBuild = true
	return b
}

func (b *ciConfigBuilder) WithSelfHosted() *ciConfigBuilder {
	b.result.useSelfHostedRunner = true
	return b
}

func (b *ciConfigBuilder) WithDebug() *ciConfigBuilder {
	b.result.debug = true
	return b
}

func (b *ciConfigBuilder) Build() CIConfig {
	return b.result
}

const (
	GithubOption  = "github"
	DefaultGithub = false

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

func NewCiGithubConfigVia(cmd *cobra.Command) (CIConfig, error) {
	result := NewCIConfigBuilder()

	workflowName, err := cmd.Flags().GetString(WorkflowNameOption)
	if err != nil {
		return CIConfig{}, err
	}
	result.WithWorkflowName(workflowName)

	branch, err := cmd.Flags().GetString(BranchOption)
	if err != nil {
		return CIConfig{}, err
	}
	result.WithBranch(branch)

	kubeconfigSecretName, err := cmd.Flags().GetString(KubeconfigSecretNameOption)
	if err != nil {
		return CIConfig{}, err
	}
	result.WithKubeconfigSecret(kubeconfigSecretName)

	registryLoginUrlName, err := cmd.Flags().GetString(RegistryLoginUrlVariableNameOption)
	if err != nil {
		return CIConfig{}, err
	}
	result.WithRegistryLoginUrlVar(registryLoginUrlName)

	registryUserVarName, err := cmd.Flags().GetString(RegistryUserVariableNameOption)
	if err != nil {
		return CIConfig{}, err
	}
	result.WithRegistryUserVar(registryUserVarName)

	registryPassSecretName, err := cmd.Flags().GetString(RegistryPassSecretNameOption)
	if err != nil {
		return CIConfig{}, err
	}
	result.WithRegistryPassSecret(registryPassSecretName)

	registryUrlVarName, err := cmd.Flags().GetString(RegistryUrlVariableNameOption)
	if err != nil {
		return CIConfig{}, err
	}
	result.WithRegistryUrlVar(registryUrlVarName)

	registryLogin, err := cmd.Flags().GetBool(UseRegistryLoginOption)
	if err != nil {
		return CIConfig{}, err
	}
	if !registryLogin {
		result.WithoutRegistryLogin()
	}

	debug, err := cmd.Flags().GetBool(UseDebugOption)
	if err != nil {
		return CIConfig{}, err
	}
	if debug {
		result.WithDebug()
	}

	remoteBuild, err := cmd.Flags().GetBool(UseRemoteBuild)
	if err != nil {
		return CIConfig{}, err
	}
	if remoteBuild {
		result.WithRemoteBuild()
	}

	selfHosted, err := cmd.Flags().GetBool(UseSelfHostedRunner)
	if err != nil {
		return CIConfig{}, err
	}
	if selfHosted {
		result.WithSelfHosted()
	}

	return result.Build(), nil
}

// TODO(twoGiants): fix broken viper cmd options propagation
func NewCiGithubConfigViaViper() CIConfig {
	result := NewCIConfigBuilder().
		WithWorkflowName(viper.GetString(WorkflowNameOption))

	if !viper.GetBool(UseRegistryLoginOption) {
		result.WithoutRegistryLogin()
	}

	return result.Build()
}
