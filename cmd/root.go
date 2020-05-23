package cmd

import (
	"fmt"
	"os"

	"github.com/ory/viper"
	"github.com/spf13/cobra"
)

var (
	config  = "~/.faas/config" // Location of the optional config file.
	verbose = false            // Enable verbose logging (debug).
)

// The root of the command tree defines the command name, descriotion, globally
// available flags, etc.  It has no action of its own, such that running the
// resultant binary with no arguments prints the help/usage text.
var root = &cobra.Command{
	Use:           "faas",
	Short:         "Function as a Service",
	Version:       Version,
	SilenceErrors: true, // we explicitly handle errors in Execute()
	SilenceUsage:  true, // no usage dump on error
	Long: `Function as a Service

Manage your Service Functions.`,
}

// When the code is loaded into memory upon invocation, the cobra/viper packages
// are invoked to gather system context.  This includes reading the configuration
// file, environment variables, and parsing the command flags.
func init() {
	// Populate `config` var with the value of --config flag, if provided.
	root.PersistentFlags().StringVar(&config, "config", config, "config file path")

	// read in environment variables that match
	viper.AutomaticEnv()

	// Populate the `verbose` flag with the value of --verbose, if provided,
	// which thus overrides both the default and the value read in from the
	// config file (i.e. flags always take highest precidence).
	root.PersistentFlags().BoolVarP(&verbose, "verbose", "v", verbose, "print verbose logs")
	viper.BindPFlag("verbose", root.PersistentFlags().Lookup("verbose"))

	// Override the --version template to match the output format from the
	// version subcommand: nothing but the version.
	root.SetVersionTemplate(`{{printf "%s\n" .Version}}`)

	// Prefix all environment variables with "FAAS_" to avoid collisions with other apps.
	viper.SetEnvPrefix("faas")
}

// Execute the command tree by executing the root command, which runs
// according to the context defined by:  the optional config file,
// Environment Variables, command arguments and flags.
func Execute() {
	// Execute the root of the command tree.
	if err := root.Execute(); err != nil {
		// Errors are printed to STDERR output and the process exits with code of 1.
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

// isInteractive returns whether or not the currently attached process terminal
// is interactive.  Used for determining whether or not to interactively prompt
// the user to confirm default choices, etc.
func isInteractive() bool {
	fi, err := os.Stdin.Stat()
	return err == nil && ((fi.Mode() & os.ModeCharDevice) != 0)
}
