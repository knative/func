package cmd

import (
	"github.com/spf13/cobra"

	"knative.dev/func/cmd/ci"
	"knative.dev/func/cmd/common"
)

func NewConfigCICmd(loaderSaver common.FunctionLoaderSaver, ciConfig ci.CIConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use: "ci",
		// TODO(twoGiants): needs fix => see comment in runConfigCIGithub
		PreRunE: bindEnv(
			ci.UseRegistryLoginOption,
			ci.UseDebugOption,
			ci.UseRemoteBuild,
			ci.UseSelfHostedRunner,
			ci.WorkflowNameOption,
		),
		RunE: func(cmd *cobra.Command, args []string) (err error) {
			return runConfigCIGithub(cmd, loaderSaver, ciConfig)
		},
	}

	addGithubFlag(cmd)
	cmd.Flags().Bool(
		ci.UseRegistryLoginOption,
		ci.DefaultUseRegistryLoginValue,
		"Add a registry login step in the github workflow",
	)
	cmd.Flags().Bool(
		ci.UseDebugOption,
		ci.DefaultUseDebugValue,
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

	return cmd
}

func runConfigCIGithub(
	cmd *cobra.Command,
	fnLoaderSaver common.FunctionLoaderSaver,
	ciConfig ci.CIConfig,
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

	githubWorkflow := ci.NewGithubWorkflow(
		ciConfig.WorkflowName(),
		ciConfig.KubeconfigSecretKey(),
		ciConfig.RegistryUrlSecretKey(),
		ciConfig.RegistryUserSecretKey(),
		ciConfig.RegistryPassSecretKey(),
		cfg.UseRegistryLogin(),
		cfg.UseRemoteBuild(),
		cfg.SelfHostedRunner(),
		cfg.UseDebug(),
	)
	if err := githubWorkflow.Persist(ciConfig.FnGithubWorkflowFilepath(f.Root)); err != nil {
		return err
	}

	return nil
}

func addGithubFlag(cmd *cobra.Command) {
	cmd.Flags().Bool(
		"github",
		false,
		"Generate GitHub Action ci workflow",
	)
}
