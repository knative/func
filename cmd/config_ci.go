package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"knative.dev/func/cmd/ci"
	"knative.dev/func/cmd/common"
)

func NewConfigCICmd(
	loaderSaver common.FunctionLoaderSaver,
	writer ci.WorkflowWriter,
	currentBranch common.CurrentBranchFunc,
	workingDir common.WorkDirFunc,
) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "ci",
		Short: "Generate a GitHub Workflow for function deployment",
		PreRunE: bindEnv(
			ci.CICDPlatformFlag,
			ci.PathFlag,
			ci.UseRegistryLoginFlag,
			ci.WorkflowDispatchFlag,
			ci.UseRemoteBuildFlag,
			ci.UseSelfHostedRunnerFlag,
			ci.WorkflowNameFlag,
			ci.BranchFlag,
			ci.KubeconfigSecretNameFlag,
			ci.RegistryLoginUrlVariableNameFlag,
			ci.RegistryUserVariableNameFlag,
			ci.RegistryPassSecretNameFlag,
			ci.RegistryUrlVariableNameFlag,
		),
		RunE: func(cmd *cobra.Command, args []string) (err error) {
			return runConfigCIGitHub(cmd, loaderSaver, writer, currentBranch, workingDir)
		},
	}

	cmd.Flags().String(
		ci.CICDPlatformFlag,
		ci.DefaultCICDPlatform,
		"Pick a CI/CD platform for which a manifest will be generated. Currently only GitHub is supported.",
	)

	addPathFlag(cmd)

	cmd.Flags().Bool(
		ci.UseRegistryLoginFlag,
		ci.DefaultUseRegistryLogin,
		"Add a registry login step in the github workflow",
	)

	cmd.Flags().Bool(
		ci.WorkflowDispatchFlag,
		ci.DefaultWorkflowDispatch,
		"Add a workflow dispatch trigger for manual workflow execution",
	)
	_ = cmd.Flags().MarkHidden(ci.WorkflowDispatchFlag)

	cmd.Flags().Bool(
		ci.UseRemoteBuildFlag,
		ci.DefaultUseRemoteBuild,
		"Build the function on a Tekton-enabled cluster",
	)

	cmd.Flags().Bool(
		ci.UseSelfHostedRunnerFlag,
		ci.DefaultUseSelfHostedRunner,
		"Use a 'self-hosted' runner instead of the default 'ubuntu-latest' for local runner execution",
	)

	cmd.Flags().String(
		ci.WorkflowNameFlag,
		ci.DefaultWorkflowName,
		"Use a custom workflow name",
	)

	cmd.Flags().String(
		ci.BranchFlag,
		"",
		"Use a custom branch name in the workflow",
	)

	cmd.Flags().String(
		ci.KubeconfigSecretNameFlag,
		ci.DefaultKubeconfigSecretName,
		"Use a custom secret name in the workflow, e.g. secret.YOUR_CUSTOM_KUBECONFIG",
	)

	cmd.Flags().String(
		ci.RegistryLoginUrlVariableNameFlag,
		ci.DefaultRegistryLoginUrlVariableName,
		"Use a custom registry login url variable name in the workflow, e.g. vars.YOUR_REGISTRY_LOGIN_URL",
	)

	cmd.Flags().String(
		ci.RegistryUserVariableNameFlag,
		ci.DefaultRegistryUserVariableName,
		"Use a custom registry user variable name in the workflow, e.g. vars.YOUR_REGISTRY_USER",
	)

	cmd.Flags().String(
		ci.RegistryPassSecretNameFlag,
		ci.DefaultRegistryPassSecretName,
		"Use a custom registry pass secret name in the workflow, e.g. secret.YOUR_REGISTRY_PASSWORD",
	)

	cmd.Flags().String(
		ci.RegistryUrlVariableNameFlag,
		ci.DefaultRegistryUrlVariableName,
		"Use a custom registry url variable name in the workflow, e.g. vars.YOUR_REGISTRY_URL",
	)

	return cmd
}

func runConfigCIGitHub(
	cmd *cobra.Command,
	fnLoaderSaver common.FunctionLoaderSaver,
	writer ci.WorkflowWriter,
	currentBranch common.CurrentBranchFunc,
	workingDir common.WorkDirFunc,
) error {
	cfg, err := ci.NewCIConfig(currentBranch, workingDir)
	if err != nil {
		return err
	}

	f, err := fnLoaderSaver.Load(cfg.Path())
	if err != nil {
		return err
	}

	fmt.Fprintln(cmd.OutOrStdout(), "--------------------------- Function GitHub Workflow Generation ---------------------------")
	fmt.Fprintf(cmd.OutOrStdout(), "Func name: %s\n", f.Name)
	fmt.Fprintf(cmd.OutOrStdout(), "Func runtime: %s\n", f.Runtime)

	githubWorkflow := ci.NewGitHubWorkflow(cfg)
	path := cfg.FnGitHubWorkflowFilepath(f.Root)
	return githubWorkflow.Export(path, writer)
}
