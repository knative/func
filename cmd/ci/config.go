package ci

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/ory/viper"
	"knative.dev/func/cmd/common"
)

const (
	ConfigCIFeatureFlag = "FUNC_ENABLE_CI_CONFIG"

	PathFlag = "path"

	PlatformFlag    = "platform"
	DefaultPlatform = "github"

	DefaultGitHubWorkflowDir      = ".github/workflows"
	DefaultGitHubWorkflowFilename = "func-deploy.yaml"

	BranchFlag    = "branch"
	DefaultBranch = "main"

	WorkflowNameFlag               = "workflow-name"
	DefaultWorkflowName            = "Func Deploy"
	DefaultRemoteBuildWorkflowName = "Remote " + DefaultWorkflowName

	KubeconfigSecretNameFlag    = "kubeconfig-secret-name"
	DefaultKubeconfigSecretName = "KUBECONFIG"

	RegistryLoginUrlVariableNameFlag    = "registry-login-url-variable-name"
	DefaultRegistryLoginUrlVariableName = "REGISTRY_LOGIN_URL"

	RegistryUserVariableNameFlag    = "registry-user-variable-name"
	DefaultRegistryUserVariableName = "REGISTRY_USERNAME"

	RegistryPassSecretNameFlag    = "registry-pass-secret-name"
	DefaultRegistryPassSecretName = "REGISTRY_PASSWORD"

	RegistryUrlVariableNameFlag    = "registry-url-variable-name"
	DefaultRegistryUrlVariableName = "REGISTRY_URL"

	RegistryLoginFlag    = "registry-login"
	DefaultRegistryLogin = true

	WorkflowDispatchFlag    = "workflow-dispatch"
	DefaultWorkflowDispatch = false

	RemoteBuildFlag    = "remote"
	DefaultRemoteBuild = false

	SelfHostedRunnerFlag    = "self-hosted-runner"
	DefaultSelfHostedRunner = false

	TestStepFlag    = "test-step"
	DefaultTestStep = true

	ForceFlag    = "force"
	DefaultForce = false

	VerboseFlag    = "verbose"
	DefaultVerbose = false
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
	registryLogin,
	selfHostedRunner,
	remoteBuild,
	workflowDispatch,
	testStep,
	force,
	verbose bool
}

func NewCIConfig(
	currentBranch common.CurrentBranchFunc,
	workingDir common.WorkDirFunc,
	workflowNameExplicit bool,
) (CIConfig, error) {
	if err := resolvePlatform(); err != nil {
		return CIConfig{}, err
	}

	path, err := resolvePath(workingDir)
	if err != nil {
		return CIConfig{}, err
	}

	branch, err := resolveBranch(path, currentBranch)
	if err != nil {
		return CIConfig{}, err
	}

	workflowName := resolveWorkflowName(workflowNameExplicit)

	return CIConfig{
		githubWorkflowDir:      DefaultGitHubWorkflowDir,
		githubWorkflowFilename: DefaultGitHubWorkflowFilename,
		path:                   path,
		branch:                 branch,
		workflowName:           workflowName,
		kubeconfigSecret:       viper.GetString(KubeconfigSecretNameFlag),
		registryLoginUrlVar:    viper.GetString(RegistryLoginUrlVariableNameFlag),
		registryUserVar:        viper.GetString(RegistryUserVariableNameFlag),
		registryPassSecret:     viper.GetString(RegistryPassSecretNameFlag),
		registryUrlVar:         viper.GetString(RegistryUrlVariableNameFlag),
		registryLogin:          viper.GetBool(RegistryLoginFlag),
		selfHostedRunner:       viper.GetBool(SelfHostedRunnerFlag),
		remoteBuild:            viper.GetBool(RemoteBuildFlag),
		workflowDispatch:       viper.GetBool(WorkflowDispatchFlag),
		testStep:               viper.GetBool(TestStepFlag),
		force:                  viper.GetBool(ForceFlag),
		verbose:                viper.GetBool(VerboseFlag),
	}, nil
}

func resolvePlatform() error {
	platform := viper.GetString(PlatformFlag)
	if strings.ToLower(platform) != DefaultPlatform {
		return fmt.Errorf("%s support is not implemented", platform)
	}

	return nil
}

func resolvePath(workingDir common.WorkDirFunc) (string, error) {
	path := viper.GetString(PathFlag)
	if path != "" && path != "." {
		return path, nil
	}

	cwd, err := workingDir()
	if err != nil {
		return "", err
	}

	return cwd, nil
}

func resolveBranch(path string, currentBranch common.CurrentBranchFunc) (string, error) {
	branch := viper.GetString(BranchFlag)
	if branch != "" {
		return branch, nil
	}

	branch, err := currentBranch(path)
	if err != nil {
		return "", err
	}

	return branch, nil
}

func resolveWorkflowName(explicit bool) string {
	workflowName := viper.GetString(WorkflowNameFlag)
	if explicit {
		return workflowName
	}

	if viper.GetBool(RemoteBuildFlag) {
		return DefaultRemoteBuildWorkflowName
	}

	return DefaultWorkflowName
}

func (cc CIConfig) fnGitHubWorkflowDir(fnRoot string) string {
	return filepath.Join(fnRoot, cc.githubWorkflowDir)
}

func (cc CIConfig) FnGitHubWorkflowFilepath(fnRoot string) string {
	return filepath.Join(cc.fnGitHubWorkflowDir(fnRoot), cc.githubWorkflowFilename)
}

func (cc CIConfig) OutputPath() string {
	return filepath.Join(cc.githubWorkflowDir, cc.githubWorkflowFilename)
}

func (cc CIConfig) Path() string {
	return cc.path
}

func (cc CIConfig) Branch() string {
	return cc.branch
}

func (cc CIConfig) WorkflowName() string {
	return cc.workflowName
}

func (cc CIConfig) KubeconfigSecret() string {
	return cc.kubeconfigSecret
}

func (cc CIConfig) RegistryLoginUrlVar() string {
	return cc.registryLoginUrlVar
}

func (cc CIConfig) RegistryUserVar() string {
	return cc.registryUserVar
}

func (cc CIConfig) RegistryPassSecret() string {
	return cc.registryPassSecret
}

func (cc CIConfig) RegistryUrlVar() string {
	return cc.registryUrlVar
}

func (cc CIConfig) RegistryLogin() bool {
	return cc.registryLogin
}

func (cc CIConfig) SelfHostedRunner() bool {
	return cc.selfHostedRunner
}

func (cc CIConfig) RemoteBuild() bool {
	return cc.remoteBuild
}

func (cc CIConfig) WorkflowDispatch() bool {
	return cc.workflowDispatch
}

func (cc CIConfig) TestStep() bool {
	return cc.testStep
}

func (cc CIConfig) Force() bool {
	return cc.force
}

func (cc CIConfig) Verbose() bool {
	return cc.verbose
}
