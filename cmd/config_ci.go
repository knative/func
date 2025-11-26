package cmd

import (
	"github.com/spf13/cobra"

	"knative.dev/func/cmd/ci"
	"knative.dev/func/cmd/common"
)

func NewConfigCICmd(loaderSaver common.FunctionLoaderSaver) *cobra.Command {
	cmd := &cobra.Command{
		Use: "ci",
		// TODO(twoGiants): needs fix => see comment in runConfigCIGithub
		PreRunE: bindEnv(
			ci.GithubOption,
			ci.UseRegistryLoginOption,
			ci.UseDebugOption,
			ci.UseRemoteBuild,
			ci.UseSelfHostedRunner,
			ci.WorkflowNameOption,
			ci.KubeconfigSecretNameOption,
			ci.RegistryLoginUrlVariableNameOption,
			ci.RegistryUserVariableNameOption,
			ci.RegistryPassSecretNameOption,
			ci.RegistryUrlVariableNameOption,
		),
		RunE: func(cmd *cobra.Command, args []string) (err error) {
			return runConfigCIGithub(cmd, loaderSaver)
		},
	}

	cmd.Flags().Bool(
		ci.GithubOption,
		ci.DefaultGithub,
		"Generate GitHub Action ci workflow",
	)
	cmd.Flags().Bool(
		ci.UseRegistryLoginOption,
		ci.DefaultUseRegistryLogin,
		"Add a registry login step in the github workflow",
	)
	cmd.Flags().Bool(
		ci.UseDebugOption,
		ci.DefaultUseDebug,
		"Add a workflow dispatch trigger and a cli caching for fast iterations on runs",
	)
	cmd.Flags().Bool(
		ci.UseRemoteBuild,
		ci.DefaultUseRemoteBuild,
		"Build the function on a Tekton-enabled cluster",
	)
	cmd.Flags().Bool(
		ci.UseSelfHostedRunner,
		ci.DefaultUseSelfHostedRunner,
		"Use a 'self-hosted' runner instead of the default 'ubuntu-latest' for local runner execution",
	)
	cmd.Flags().String(
		ci.WorkflowNameOption,
		ci.DefaultWorkflowName,
		"Use a custom workflow name",
	)
	cmd.Flags().String(
		ci.BranchOption,
		ci.DefaultBranch,
		"Use a custom branch name in the workflow",
	)
	cmd.Flags().String(
		ci.KubeconfigSecretNameOption,
		ci.DefaultKubeconfigSecretName,
		"Use a custom secret name in the workflow, e.g. ${{ secret.CUSTOM_NAME }}",
	)
	cmd.Flags().String(
		ci.RegistryLoginUrlVariableNameOption,
		ci.DefaultRegistryLoginUrlVariableName,
		"Use a custom registry login url variable name in the workflow, e.g. ${{ vars.CUSTOM_NAME }}",
	)
	cmd.Flags().String(
		ci.RegistryUserVariableNameOption,
		ci.DefaultRegistryUserVariableName,
		"Use a custom registry user variable name in the workflow, e.g. ${{ vars.CUSTOM_NAME }}",
	)
	cmd.Flags().String(
		ci.RegistryPassSecretNameOption,
		ci.DefaultRegistryPassSecretName,
		"Use a custom registry pass secret name in the workflow, e.g. ${{ secret.CUSTOM_NAME }}",
	)
	cmd.Flags().String(
		ci.RegistryUrlVariableNameOption,
		ci.DefaultRegistryUrlVariableName,
		"Use a custom registry url variable name in the workflow, e.g. ${{ vars.CUSTOM_NAME }}",
	)

	return cmd
}

func runConfigCIGithub(
	cmd *cobra.Command,
	fnLoaderSaver common.FunctionLoaderSaver,
) error {

	// TODO(twoGiants): viper propagation broken => can't test with flags --use-registry-login
	// or --workflow-name => flags aren't propagated to viper
	// _ = ci.NewCiGithubConfigViaViper()
	cfg, err := ci.NewCiGithubConfigVia(cmd)
	if err != nil {
		return err
	}

	f, err := initConfigCommand(fnLoaderSaver)
	if err != nil {
		return err
	}

	githubWorkflow := ci.NewGithubWorkflow(cfg)
	if err := githubWorkflow.Persist(cfg.FnGithubWorkflowFilepath(f.Root)); err != nil {
		return err
	}

	return nil
}
