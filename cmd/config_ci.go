package cmd

import (
	"github.com/spf13/cobra"

	"knative.dev/func/cmd/ci"
	"knative.dev/func/cmd/common"
)

func NewConfigCICmd(loaderSaver common.FunctionLoaderSaver, ciConfig ci.CIConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use: "ci",
		RunE: func(cmd *cobra.Command, args []string) (err error) {
			return runConfigCIGithub(loaderSaver, ciConfig)
		},
	}

	addGithubFlag(cmd)

	return cmd
}

func runConfigCIGithub(
	fnLoaderSaver common.FunctionLoaderSaver,
	ciConfig ci.CIConfig,
) error {
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
		ciConfig.UseRegistryLogin(),
		ciConfig.SelfHostedRunner(),
	)
	if err := githubWorkflow.Persist(ciConfig.FnGithubWorkflowFilepath(f.Root)); err != nil {
		return err
	}

	return nil
}

func addGithubFlag(cmd *cobra.Command) {
	cmd.Flags().BoolP(
		"github",
		"",
		false,
		"Generate GitHub Action ci workflow",
	)
}
