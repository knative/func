package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"knative.dev/func/cmd/ci"
	"knative.dev/func/cmd/common"
)

func NewConfigCICmd(loaderSaver common.FunctionLoaderSaver) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "ci",
		Short: "Generate a Github Workflow for function deployment",
		PreRunE: bindEnv(
			ci.PathOption,
			ci.UseRegistryLoginOption,
			ci.UseDebugOption,
			ci.UseRemoteBuild,
			ci.UseSelfHostedRunner,
			ci.WorkflowNameOption,
			ci.BranchOption,
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

	addPathFlag(cmd)

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
	cmd.Flags().MarkHidden(ci.UseDebugOption)

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
		"Use a custom secret name in the workflow, e.g. secret.YOUR_CUSTOM_KUBECONFIG",
	)

	cmd.Flags().String(
		ci.RegistryLoginUrlVariableNameOption,
		ci.DefaultRegistryLoginUrlVariableName,
		"Use a custom registry login url variable name in the workflow, e.g. vars.YOUR_REGISTRY_LOGIN_URL",
	)

	cmd.Flags().String(
		ci.RegistryUserVariableNameOption,
		ci.DefaultRegistryUserVariableName,
		"Use a custom registry user variable name in the workflow, e.g. vars.YOUR_REGISTRY_USER",
	)

	cmd.Flags().String(
		ci.RegistryPassSecretNameOption,
		ci.DefaultRegistryPassSecretName,
		"Use a custom registry pass secret name in the workflow, e.g. secret.YOUR_REGISTRY_PASSWORD",
	)

	cmd.Flags().String(
		ci.RegistryUrlVariableNameOption,
		ci.DefaultRegistryUrlVariableName,
		"Use a custom registry url variable name in the workflow, e.g. vars.YOUR_REGISTRY_URL",
	)

	return cmd
}

func runConfigCIGithub(
	cmd *cobra.Command,
	// TODO(twoGiants): common.FunctionLoader is enough
	fnLoaderSaver common.FunctionLoaderSaver,
) error {
	if os.Getenv(ci.ConfigCIFeatureFlag) != "true" {
		return fmt.Errorf("set %s to 'true' to use this feature", ci.ConfigCIFeatureFlag)
	}

	cfg := ci.NewCiGithubConfig()

	f, err := fnLoaderSaver.Load(cfg.Path())
	if err != nil {
		return err
	}

	fmt.Fprintln(cmd.OutOrStdout(), "--------------------------- Function Github Workflow Generation ---------------------------")
	fmt.Fprintf(cmd.OutOrStdout(), "Func name: %s\n", f.Name)
	fmt.Fprintf(cmd.OutOrStdout(), "Func runtime: %s\n", f.Runtime)

	githubWorkflow := ci.NewGithubWorkflow(cfg)
	if err := githubWorkflow.Persist(cfg.FnGithubWorkflowFilepath(f.Root)); err != nil {
		return err
	}

	return nil
}
