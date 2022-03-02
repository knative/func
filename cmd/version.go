package cmd

import (
	"fmt"
	"text/template"

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
	cmd.SetHelpFunc(runVersionHelp)

	// Run Action
	cmd.Run = func(cmd *cobra.Command, args []string) {
		runVersion(cmd, args, version)
	}
	cmd.SetHelpFunc(defaultTemplatedHelp)

	return cmd
}

// Run
func runVersion(cmd *cobra.Command, args []string, version Version) {
	version.Verbose = viper.GetBool("verbose")
	fmt.Fprintf(cmd.OutOrStdout(), "%v\n", version)
}

// Help
func runVersionHelp(cmd *cobra.Command, args []string) {
	var (
		body = cmd.Long + "\n\n" + cmd.UsageString()
		t    = template.New("version")
		tpl  = template.Must(t.Parse(body))
	)

	var data = struct {
		Name string
	}{
		Name: cmd.Root().Name(),
	}

	if err := tpl.Execute(cmd.OutOrStdout(), data); err != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "unable to display help text: %v", err)
	}
}
