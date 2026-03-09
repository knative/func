package cmd

import (
	"fmt"
	"strings"

	"github.com/ory/viper"
	"github.com/spf13/cobra"

	"knative.dev/func/cmd/common"
	"knative.dev/func/pkg/ci/github"
	fn "knative.dev/func/pkg/functions"
)

const (
	ConfigCIFeatureFlag              = "FUNC_ENABLE_CI_CONFIG"
	pathFlag                         = "path"
	platformFlag                     = "platform"
	branchFlag                       = "branch"
	workflowNameFlag                 = "workflow-name"
	kubeconfigSecretNameFlag         = "kubeconfig-secret-name"
	registryLoginUrlVariableNameFlag = "registry-login-url-variable-name"
	registryUserVariableNameFlag     = "registry-user-variable-name"
	registryPassSecretNameFlag       = "registry-pass-secret-name"
	registryUrlVariableNameFlag      = "registry-url-variable-name"
	registryLoginFlag                = "registry-login"
	workflowDispatchFlag             = "workflow-dispatch"
	remoteBuildFlag                  = "remote"
	selfHostedRunnerFlag             = "self-hosted-runner"
	testStepFlag                     = "test-step"
	forceFlag                        = "force"
	verboseFlag                      = "verbose"
)

func NewConfigCICmd(
	loaderSaver common.FunctionLoaderSaver,
	pathWriter fn.PathWriter,
	currentBranch common.CurrentBranchFunc,
	workingDir common.WorkDirFunc,
	newClient ClientFactory,
) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "ci",
		Short: "Generate a GitHub Workflow for function deployment",
		PreRunE: bindEnv(
			pathFlag,
			platformFlag,
			registryLoginFlag,
			workflowNameFlag,
			kubeconfigSecretNameFlag,
			registryLoginUrlVariableNameFlag,
			registryUserVariableNameFlag,
			registryPassSecretNameFlag,
			registryUrlVariableNameFlag,
			workflowDispatchFlag,
			remoteBuildFlag,
			selfHostedRunnerFlag,
			testStepFlag,
			branchFlag,
			forceFlag,
			verboseFlag,
		),
		RunE: func(cmd *cobra.Command, args []string) (err error) {
			// Detect explicit config via CLI flag or env var
			workflowNameExplicit :=
				cmd.Flags().Changed(workflowNameFlag) || viper.IsSet(workflowNameFlag)

			return runConfigCIGitHub(
				cmd,
				pathWriter,
				loaderSaver,
				currentBranch,
				workingDir,
				workflowNameExplicit,
				newClient,
			)
		},
	}

	addPathFlag(cmd)

	cmd.Flags().String(
		platformFlag,
		github.DefaultPlatform,
		"Pick a CI/CD platform for which a manifest will be generated. Currently only GitHub is supported.",
	)

	cmd.Flags().String(
		branchFlag,
		"",
		"Use a custom branch name in the workflow",
	)

	cmd.Flags().String(
		workflowNameFlag,
		github.DefaultWorkflowName,
		"Use a custom workflow name",
	)

	cmd.Flags().String(
		kubeconfigSecretNameFlag,
		github.DefaultKubeconfigSecretName,
		"Use a custom secret name in the workflow, e.g. secret.YOUR_CUSTOM_KUBECONFIG",
	)

	cmd.Flags().String(
		registryLoginUrlVariableNameFlag,
		github.DefaultRegistryLoginUrlVariableName,
		"Use a custom registry login url variable name in the workflow, e.g. vars.YOUR_REGISTRY_LOGIN_URL",
	)

	cmd.Flags().String(
		registryUserVariableNameFlag,
		github.DefaultRegistryUserVariableName,
		"Use a custom registry user variable name in the workflow, e.g. vars.YOUR_REGISTRY_USER",
	)

	cmd.Flags().String(
		registryPassSecretNameFlag,
		github.DefaultRegistryPassSecretName,
		"Use a custom registry pass secret name in the workflow, e.g. secret.YOUR_REGISTRY_PASSWORD",
	)

	cmd.Flags().String(
		registryUrlVariableNameFlag,
		github.DefaultRegistryUrlVariableName,
		"Use a custom registry url variable name in the workflow, e.g. vars.YOUR_REGISTRY_URL",
	)

	cmd.Flags().Bool(
		registryLoginFlag,
		github.DefaultRegistryLogin,
		"Add a registry login step in the github workflow",
	)

	cmd.Flags().Bool(
		workflowDispatchFlag,
		github.DefaultWorkflowDispatch,
		"Add a workflow dispatch trigger for manual workflow execution",
	)
	_ = cmd.Flags().MarkHidden(workflowDispatchFlag)

	cmd.Flags().Bool(
		remoteBuildFlag,
		github.DefaultRemoteBuild,
		"Build the function on a Tekton-enabled cluster",
	)

	cmd.Flags().Bool(
		selfHostedRunnerFlag,
		github.DefaultSelfHostedRunner,
		"Use a 'self-hosted' runner instead of the default 'ubuntu-latest' for local runner execution",
	)

	cmd.Flags().Bool(
		testStepFlag,
		github.DefaultTestStep,
		"Add a language-specific test step (supported: go, node, typescript, python, quarkus)",
	)

	cmd.Flags().Bool(
		forceFlag,
		github.DefaultForce,
		"Use to overwrite an existing GitHub workflow",
	)

	addVerboseFlag(cmd, github.DefaultVerbose)

	return cmd
}

func runConfigCIGitHub(
	cmd *cobra.Command,
	pathWriter fn.PathWriter,
	fnLoaderSaver common.FunctionLoaderSaver,
	currentBranch common.CurrentBranchFunc,
	workingDir common.WorkDirFunc,
	workflowNameExplicit bool,
	newClient ClientFactory,
) error {
	cfg, err := newCIConfig(
		fnLoaderSaver,
		currentBranch,
		workingDir,
		workflowNameExplicit,
	)
	if err != nil {
		return err
	}

	client, done := newClient(
		ClientConfig{Verbose: viper.GetBool(verboseFlag)},
		fn.WithPathWriter(pathWriter),
		fn.WithStdout(cmd.OutOrStdout()),
	)
	defer done()

	if err := client.GenerateCIWorkflow(cmd.Context(), cfg); err != nil {
		return err
	}

	return nil
}

func newCIConfig(
	fnLoader common.FunctionLoader,
	currentBranch common.CurrentBranchFunc,
	workingDir common.WorkDirFunc,
	workflowNameExplicit bool,
) (github.Config, error) {
	if err := resolvePlatform(); err != nil {
		return github.Config{}, err
	}

	path, err := resolvePath(workingDir)
	if err != nil {
		return github.Config{}, err
	}

	branch, err := resolveBranch(path, currentBranch)
	if err != nil {
		return github.Config{}, err
	}

	workflowName := resolveWorkflowName(workflowNameExplicit)

	f, err := fnLoader.Load(path)
	if err != nil {
		return github.Config{}, err
	}

	return github.Config{
		GithubWorkflowDir:      github.DefaultGitHubWorkflowDir,
		GithubWorkflowFilename: github.DefaultGitHubWorkflowFilename,
		Branch:                 branch,
		WorkflowName:           workflowName,
		KubeconfigSecret:       viper.GetString(kubeconfigSecretNameFlag),
		RegistryLoginUrlVar:    viper.GetString(registryLoginUrlVariableNameFlag),
		RegistryUserVar:        viper.GetString(registryUserVariableNameFlag),
		RegistryPassSecret:     viper.GetString(registryPassSecretNameFlag),
		RegistryUrlVar:         viper.GetString(registryUrlVariableNameFlag),
		RegistryLogin:          viper.GetBool(registryLoginFlag),
		SelfHostedRunner:       viper.GetBool(selfHostedRunnerFlag),
		RemoteBuild:            viper.GetBool(remoteBuildFlag),
		WorkflowDispatch:       viper.GetBool(workflowDispatchFlag),
		TestStep:               viper.GetBool(testStepFlag),
		Force:                  viper.GetBool(forceFlag),
		FnRuntime:              f.Runtime,
		FnRoot:                 f.Root,
	}, nil
}

func resolvePlatform() error {
	platform := viper.GetString(platformFlag)
	if platform == "" {
		return fmt.Errorf("platform must not be empty, supported: %s", github.DefaultPlatform)
	}
	if strings.ToLower(platform) != github.DefaultPlatform {
		return fmt.Errorf("%s support is not implemented, supported: %s", platform, github.DefaultPlatform)
	}

	return nil
}

func resolvePath(workingDir common.WorkDirFunc) (string, error) {
	path := viper.GetString(pathFlag)
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
	branch := viper.GetString(branchFlag)
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
	workflowName := viper.GetString(workflowNameFlag)
	if explicit {
		return workflowName
	}

	if viper.GetBool(remoteBuildFlag) {
		return github.DefaultRemoteBuildWorkflowName
	}

	return github.DefaultWorkflowName
}
