package cmd

import (
	"fmt"
	"os"

	"github.com/mitchellh/go-homedir"
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
	Short:         "Function as a Service Manager",
	Version:       Version,
	SilenceErrors: true, // we explicitly handle errors in Execute()
	SilenceUsage:  true, // no usage dump on error
	Long: `Function as a Service Manager

Create, run and deploy.`,
}

// When the code is loaded into memory upon invocation, the cobra/viper packages
// are invoked to gather system context.  This includes reading the configuration
// file, environment variables, and parsing the command flags.
func init() {
	// Populate `config` var with the value of --config flag, if provided.
	root.PersistentFlags().StringVar(&config, "config", config, "config file path")

	// Read in the config file specified by `config`, overriding defaults.
	cobra.OnInitialize(readConfig)

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

// readConfig populates variables (overriding defaults) from the config file
// and environment variables.
func readConfig() {
	if config != "" {
		viper.SetConfigFile(config) // Use config file from the flag.
	} else {
		home, err := homedir.Dir() // Find home directory.
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
		viper.AddConfigPath(home) // Search for cconfig in home
		viper.SetConfigName(".faascfg")
	}

	// read in environment variables that match
	viper.AutomaticEnv()

	// If a config file is found, read it in.
	if err := viper.ReadInConfig(); err == nil {
		fmt.Println("Using config file:", viper.ConfigFileUsed())
	}
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
