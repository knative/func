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
			// TODO(twoGiants): extract into constants
			githubWorkflowDir:      ".github/workflows",
			githubWorkflowFilename: "remote-build-and-deploy.yaml",
			workflowName:           "Remote Build and Deploy",
			kubeconfigSecret:       "KUBECONFIG",
			registryLoginUrlVar:    "REGISTRY_LOGIN_URL",
			registryUserVar:        "REGISTRY_USERNAME",
			registryPassSecret:     "REGISTRY_PASSWORD",
			registryUrlVar:         "REGISTRY_URL",
			useRegistryLogin:       true,
			useRemoteBuild:         false,
			useSelfHostedRunner:    false,
			debug:                  false,
		},
	}
}

func (b *ciConfigBuilder) WithWorkflowName(name string) *ciConfigBuilder {
	b.result.workflowName = name
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
	UseRegistryLoginOption       = "use-registry-login"
	DefaultUseRegistryLoginValue = true

	WorkflowNameOption  = "workflow-name"
	DefaultWorkflowName = "Local Build and Remote Deploy"

	UseDebugOption       = "debug"
	DefaultUseDebugValue = false

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

func NewCiGithubConfigViaViper() CIConfig {
	result := NewCIConfigBuilder().
		WithWorkflowName(viper.GetString(WorkflowNameOption))

	if !viper.GetBool(UseRegistryLoginOption) {
		result.WithoutRegistryLogin()
	}

	return result.Build()
}
