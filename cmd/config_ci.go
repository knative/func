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
			ci.WorkflowNameOption,
		),
		RunE: func(cmd *cobra.Command, args []string) (err error) {
			return runConfigCIGithub(cmd, loaderSaver)
		},
	}

	addPathFlag(cmd)
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

	return fmt.Errorf("not implemented")
}
