package cmd

import (
	"github.com/spf13/cobra"
	"knative.dev/func/cmd/common"
)

func NewConfigCICmd(loaderSaver common.FunctionLoaderSaver) *cobra.Command {
	cmd := &cobra.Command{
		Use: "ci",
		RunE: func(cmd *cobra.Command, args []string) (err error) {
			return runConfigCIGithub(loaderSaver)
		},
	}

	addGithubFlag(cmd)

	return cmd
}

func runConfigCIGithub(
	loaderSaver common.FunctionLoaderSaver,
) error {
	if _, err := initConfigCommand(loaderSaver); err != nil {
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
