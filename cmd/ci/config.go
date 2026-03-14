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
	fnRuntime,
	fnRoot,
	fnBuilder string
}

func NewCIConfig(
	fnLoader common.FunctionLoader,
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

	f, err := fnLoader.Load(path)
	if err != nil {
		return CIConfig{}, err
	}

	remoteBuild := viper.GetBool(RemoteBuildFlag)
	fnBuilder, err := resolveBuilder(f.Runtime, remoteBuild)
	if err != nil {
		return CIConfig{}, err
	}

	return CIConfig{
		githubWorkflowDir:      DefaultGitHubWorkflowDir,
		githubWorkflowFilename: DefaultGitHubWorkflowFilename,
		branch:                 branch,
		workflowName:           workflowName,
		kubeconfigSecret:       viper.GetString(KubeconfigSecretNameFlag),
		registryLoginUrlVar:    viper.GetString(RegistryLoginUrlVariableNameFlag),
		registryUserVar:        viper.GetString(RegistryUserVariableNameFlag),
		registryPassSecret:     viper.GetString(RegistryPassSecretNameFlag),
		registryUrlVar:         viper.GetString(RegistryUrlVariableNameFlag),
		registryLogin:          viper.GetBool(RegistryLoginFlag),
		selfHostedRunner:       viper.GetBool(SelfHostedRunnerFlag),
		remoteBuild:            remoteBuild,
		workflowDispatch:       viper.GetBool(WorkflowDispatchFlag),
		testStep:               viper.GetBool(TestStepFlag),
		force:                  viper.GetBool(ForceFlag),
		verbose:                viper.GetBool(VerboseFlag),
		fnRuntime:              f.Runtime,
		fnRoot:                 f.Root,
		fnBuilder:              fnBuilder,
	}, nil
}

func resolvePlatform() error {
	platform := viper.GetString(PlatformFlag)
	if platform == "" {
		return fmt.Errorf("platform must not be empty, supported: %s", DefaultPlatform)
	}
	if strings.ToLower(platform) != DefaultPlatform {
		return fmt.Errorf("%s support is not implemented, supported: %s", platform, DefaultPlatform)
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

func resolveBuilder(runtime string, remote bool) (string, error) {
	switch runtime {
	case "go":
		if remote {
			return "pack", nil
		}
		return "host", nil

	case "node", "typescript", "rust", "quarkus", "springboot":
		return "pack", nil

	case "python":
		if remote {
			return "s2i", nil
		}
		return "host", nil

	default:
		return "", fmt.Errorf("no builder support for runtime: %s", runtime)
	}
}

func (cc CIConfig) FnGitHubWorkflowFilepath() string {
	fnGitHubWorkflowDir := filepath.Join(cc.fnRoot, cc.githubWorkflowDir)
	return filepath.Join(fnGitHubWorkflowDir, cc.githubWorkflowFilename)
}

func (cc CIConfig) OutputPath() string {
	return filepath.Join(cc.githubWorkflowDir, cc.githubWorkflowFilename)
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

func (cc CIConfig) FnRuntime() string {
	return cc.fnRuntime
}

func (cc CIConfig) FnBuilder() string {
	return cc.fnBuilder
}
