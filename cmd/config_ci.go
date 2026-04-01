package cmd

import (
	"io"

	"github.com/ory/viper"
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
			ci.PathFlag,
			ci.PlatformFlag,
			ci.RegistryLoginFlag,
			ci.WorkflowNameFlag,
			ci.KubeconfigSecretNameFlag,
			ci.RegistryLoginUrlVariableNameFlag,
			ci.RegistryUserVariableNameFlag,
			ci.RegistryPassSecretNameFlag,
			ci.RegistryUrlVariableNameFlag,
			ci.WorkflowDispatchFlag,
			ci.RemoteBuildFlag,
			ci.SelfHostedRunnerFlag,
			ci.TestStepFlag,
			ci.BranchFlag,
			ci.ForceFlag,
			ci.VerboseFlag,
		),
		RunE: func(cmd *cobra.Command, args []string) (err error) {
			// Detect explicit config via CLI flag or env var
			workflowNameExplicit :=
				cmd.Flags().Changed(ci.WorkflowNameFlag) || viper.IsSet(ci.WorkflowNameFlag)

			return runConfigCIGitHub(
				loaderSaver,
				writer,
				currentBranch,
				workingDir,
				cmd.OutOrStdout(),
				workflowNameExplicit,
			)
		},
	}

	addPathFlag(cmd)

	cmd.Flags().String(
		ci.PlatformFlag,
		ci.DefaultPlatform,
		"Pick a CI/CD platform for which a manifest will be generated. Currently only GitHub is supported.",
	)

	cmd.Flags().String(
		ci.BranchFlag,
		"",
		"Use a custom branch name in the workflow",
	)

	cmd.Flags().String(
		ci.WorkflowNameFlag,
		ci.DefaultWorkflowName,
		"Use a custom workflow name",
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

	cmd.Flags().Bool(
		ci.RegistryLoginFlag,
		ci.DefaultRegistryLogin,
		"Add a registry login step in the github workflow",
	)

	cmd.Flags().Bool(
		ci.WorkflowDispatchFlag,
		ci.DefaultWorkflowDispatch,
		"Add a workflow dispatch trigger for manual workflow execution",
	)
	_ = cmd.Flags().MarkHidden(ci.WorkflowDispatchFlag)

	cmd.Flags().Bool(
		ci.RemoteBuildFlag,
		ci.DefaultRemoteBuild,
		"Build the function on a Tekton-enabled cluster",
	)

	cmd.Flags().Bool(
		ci.SelfHostedRunnerFlag,
		ci.DefaultSelfHostedRunner,
		"Use a 'self-hosted' runner instead of the default 'ubuntu-latest' for local runner execution",
	)

	cmd.Flags().Bool(
		ci.TestStepFlag,
		ci.DefaultTestStep,
		"Add a language-specific test step (supported: go, node, typescript, python, quarkus)",
	)

	cmd.Flags().Bool(
		ci.ForceFlag,
		ci.DefaultForce,
		"Use to overwrite an existing GitHub workflow",
	)

	addVerboseFlag(cmd, ci.DefaultVerbose)

	return cmd
}

func runConfigCIGitHub(
	fnLoaderSaver common.FunctionLoaderSaver,
	writer ci.WorkflowWriter,
	currentBranch common.CurrentBranchFunc,
	workingDir common.WorkDirFunc,
	messageWriter io.Writer,
	workflowNameExplicit bool,
) error {
	cfg, err := ci.NewCIConfig(fnLoaderSaver, currentBranch, workingDir, workflowNameExplicit)
	if err != nil {
		return err
	}

	githubWorkflow := ci.NewGitHubWorkflow(cfg, messageWriter)
	if err := githubWorkflow.Export(cfg.FnGitHubWorkflowFilepath(), writer, cfg.Force(), messageWriter); err != nil {
		return err
	}

	if cfg.Verbose() {
		// best-effort user message; errors are non-critical
		_ = ci.PrintConfiguration(messageWriter, cfg)
		return nil
	}

	// best-effort user message; errors are non-critical
	_ = ci.PrintPostExportMessage(messageWriter, cfg)
	return nil
}
