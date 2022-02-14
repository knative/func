package cmd

import (
	"fmt"

	"github.com/ory/viper"
	"github.com/spf13/cobra"
)

func NewVersionCmd(version Version) *cobra.Command {
	runVersion := func(cmd *cobra.Command, args []string) {
		version.Verbose = viper.GetBool("verbose")
		fmt.Fprintf(cmd.OutOrStdout(), "%v\n", version)
	}

	cmd := &cobra.Command{
		Use:   "version",
		Short: "Show the version",
		Long: `Show the version

Use the --verbose option to include the build date stamp and commit hash"
`,
		SuggestFor: []string{"vers", "verison"}, //nolint:misspell
		Run:        runVersion,
	}

	return cmd
}
