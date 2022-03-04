package cmd

import (
	"fmt"

	"github.com/ory/viper"
	"github.com/spf13/cobra"
)

func NewVersionCmd(version Version) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "version",
		Short: "Show the version",
		Long: `
NAME
	{{.Name}} version - Function version information.

SYNOPSIS
	{{.Name}} version [-v|--verbose]

DESCRIPTION
	Print version information.  Use the --verbose option to see date stamp and
	associated git source control hash if available.

	o Print the Functions version
	  $ {{.Name}} version

	o Print the Functions version along with date and associated git commit hash.
	  $ {{.Name}} version -v

`,
		SuggestFor: []string{"vers", "verison"}, //nolint:misspell
		PreRunE:    bindEnv("verbose"),
	}

	// Help Action
	cmd.SetHelpFunc(defaultTemplatedHelp)

	// Run Action
	cmd.Run = func(cmd *cobra.Command, args []string) {
		runVersion(cmd, args, version)
	}

	return cmd
}

// Run
func runVersion(cmd *cobra.Command, args []string, version Version) {
	version.Verbose = viper.GetBool("verbose")
	fmt.Fprintf(cmd.OutOrStdout(), "%v\n", version)
}
