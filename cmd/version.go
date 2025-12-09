package cmd

import (
	"fmt"

	"github.com/ory/viper"
	"github.com/spf13/cobra"
	"knative.dev/func/pkg/config"
	"knative.dev/func/pkg/k8s"
)

func NewVersionCmd(version Version) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "version",
		Short: "Function client version information",
		Long: `
NAME
	{{rootCmdUse}} version - function version information.

SYNOPSIS
	{{rootCmdUse}} version [-v|--verbose] [-o|--output]

DESCRIPTION
	Print version information.  Use the --verbose option to see date stamp and
	associated git source control hash if available.  Use the --output option
	to specify the output format (human|json|yaml).

	o Print the functions version
	  $ {{rootCmdUse}} version

	o Print the functions version along with source git commit hash and other
	  metadata.
	  $ {{rootCmdUse}} version -v

	o Print the version information in JSON format
	  $ {{rootCmdUse}} version --output json

	o Print verbose version information in YAML format
	  $ {{rootCmdUse}} version -v -o yaml

`,
		SuggestFor: []string{"vers", "version"}, //nolint:misspell
		PreRunE:    bindEnv("verbose", "output"),
		Run: func(cmd *cobra.Command, _ []string) {
			runVersion(cmd, version)
		},
	}
	cfg, err := config.NewDefault()
	if err != nil {
		fmt.Fprintf(cmd.OutOrStdout(), "error loading config at '%v'. %v\n", config.File(), err)
	}

	// Add flags
	cmd.Flags().StringP("output", "o", "human", "Output format (human|json|yaml) ($FUNC_OUTPUT)")
	addVerboseFlag(cmd, cfg.Verbose)

	// Add flag completion
	if err := cmd.RegisterFlagCompletionFunc("output", CompleteOutputFormatList); err != nil {
		fmt.Println("internal: error while calling RegisterFlagCompletionFunc: ", err)
	}

	return cmd
}

// Run
func runVersion(cmd *cobra.Command, v Version) {
	verbose := viper.GetBool("verbose")
	output := viper.GetString("output")

	// Set verbose flag
	v.Verbose = verbose

	// Initialize the default value to the zero semver with a descriptive
	// metadta tag indicating this must have been built from source if
	// undefined:
	if v.Vers == "" {
		v.Vers = DefaultVersion
	}

	// Kver, Hash are already set from build
	// Populate image fields from k8s package constants
	v.SocatImage = k8s.SocatImage
	v.TarImage = k8s.TarImage

	write(cmd.OutOrStdout(), v, output)
}
