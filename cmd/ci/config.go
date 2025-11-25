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
	kubeconfigSecretKey,
	registryUrlSecretKey,
	registryUserSecretKey,
	registryPassSecretKey string
	useRegistryLogin,
	useRemoteBuild,
	selfHostedRunner,
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

func (cc *CIConfig) SelfHostedRunner() bool {
	return cc.selfHostedRunner
}

func (cc *CIConfig) UseDebug() bool {
	return cc.debug
}

func (cc *CIConfig) KubeconfigSecretKey() string {
	return cc.kubeconfigSecretKey
}

func (cc *CIConfig) RegistryUrlSecretKey() string {
	return cc.registryUrlSecretKey
}

func (cc *CIConfig) RegistryUserSecretKey() string {
	return cc.registryUserSecretKey
}

func (cc *CIConfig) RegistryPassSecretKey() string {
	return cc.registryPassSecretKey
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
			kubeconfigSecretKey:    "KUBECONFIG",
			registryUrlSecretKey:   "REGISTRY_URL",
			registryUserSecretKey:  "REGISTRY_USERNAME",
			registryPassSecretKey:  "REGISTRY_PASSWORD",
			useRegistryLogin:       true,
			useRemoteBuild:         false,
			selfHostedRunner:       false,
			debug:                  false,
		},
	}
}

func (b *ciConfigBuilder) WithWorkflowName(name string) *ciConfigBuilder {
	b.result.workflowName = name
	return b
}

func (b *ciConfigBuilder) WithKubeconfigKey(key string) *ciConfigBuilder {
	b.result.kubeconfigSecretKey = key
	return b
}

func (b *ciConfigBuilder) WithRegistryUrlKey(key string) *ciConfigBuilder {
	b.result.registryUrlSecretKey = key
	return b
}

func (b *ciConfigBuilder) WithRegistryUserKey(key string) *ciConfigBuilder {
	b.result.registryUserSecretKey = key
	return b
}

func (b *ciConfigBuilder) WithRegistryPassKey(key string) *ciConfigBuilder {
	b.result.registryPassSecretKey = key
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

func (b *ciConfigBuilder) WithSelfHosted(useSelfHosted bool) *ciConfigBuilder {
	b.result.selfHostedRunner = useSelfHosted
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
	DefaultWorkflowName = "Remote Build and Deploy"
)

func NewCiGithubConfigViaViper() CIConfig {
	result := NewCIConfigBuilder().
		WithWorkflowName(viper.GetString(WorkflowNameOption))

	if !viper.GetBool(UseRegistryLoginOption) {
		result.WithoutRegistryLogin()
	}

	return result.Build()
}

func NewCiGithubConfigVia(cmd *cobra.Command) (CIConfig, error) {
	result := NewCIConfigBuilder()

	workflowName, err := cmd.Flags().GetString(WorkflowNameOption)
	if err != nil {
		return CIConfig{}, err
	}
	result.WithWorkflowName(workflowName)

	useRegistryLogin, err := cmd.Flags().GetBool(UseRegistryLoginOption)
	if err != nil {
		return CIConfig{}, err
	}
	if !useRegistryLogin {
		result.WithoutRegistryLogin()
	}

	return result.Build(), nil
}
