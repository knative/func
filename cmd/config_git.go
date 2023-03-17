package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"knative.dev/func/pkg/config"
	fn "knative.dev/func/pkg/functions"
)

func NewConfigGitCmd(newClient ClientFactory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "git",
		Short: "Manage Git configuration of a function",
		Long: `Manage Git configuration of a function

Prints Git configuration for a function project present in
the current directory or from the directory specified with --path.
`,
		SuggestFor: []string{"gti", "Git", "Gti"},
		PreRunE:    bindEnv("path"),
		RunE: func(cmd *cobra.Command, args []string) (err error) {
			return runConfigGitCmd(cmd, newClient)
		},
	}
	// Global Config
	cfg, err := config.NewDefault()
	if err != nil {
		fmt.Fprintf(cmd.OutOrStdout(), "error loading config at '%v'. %v\n", config.File(), err)
	}

	// Function Context
	f, _ := fn.NewFunction(effectivePath())
	if f.Initialized() {
		cfg = cfg.Apply(f)
	}

	configGitSetCmd := NewConfigGitSetCmd(newClient)
	configGitRemoveCmd := NewConfigGitRemoveCmd(newClient)

	addPathFlag(cmd)
	addVerboseFlag(cmd, cfg.Verbose)

	cmd.AddCommand(configGitSetCmd)
	cmd.AddCommand(configGitRemoveCmd)

	return cmd
}

func runConfigGitCmd(cmd *cobra.Command, newClient ClientFactory) (err error) {
	fmt.Printf("--------------------------- Function Git config ---------------------------\n")
	fmt.Printf("Not implemented yet.\n")

	return nil
}
