package cmd

import (
	"fmt"

	"github.com/ory/viper"
	"github.com/spf13/cobra"
	"knative.dev/func/pkg/config"
)

func NewVersionCmd(version Version) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "version",
		Short: "Function client version information",
		Long: `
NAME
	{{rootCmdUse}} version - function version information.

SYNOPSIS
	{{rootCmdUse}} version [-v|--verbose]

DESCRIPTION
	Print version information.  Use the --verbose option to see date stamp and
	associated git source control hash if available.

	o Print the functions version
	  $ {{rootCmdUse}} version

	o Print the functions version along with source git commit hash and other
	  metadata.
	  $ {{rootCmdUse}} version -v

`,
		SuggestFor: []string{"vers", "version"}, //nolint:misspell
		PreRunE:    bindEnv("verbose"),
		Run: func(cmd *cobra.Command, _ []string) {
			runVersion(cmd, version)
		},
	}
	cfg, err := config.NewDefault()
	if err != nil {
		fmt.Fprintf(cmd.OutOrStdout(), "error loading config at '%v'. %v\n", config.File(), err)
	}
	addVerboseFlag(cmd, cfg.Verbose)

	return cmd
}

// Run
func runVersion(cmd *cobra.Command, version Version) {
	version.Verbose = viper.GetBool("verbose")
	fmt.Fprintf(cmd.OutOrStdout(), "%v\n", version)
}
