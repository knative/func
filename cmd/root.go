package cmd

import (
	"fmt"
	"os"

	"github.com/mitchellh/go-homedir"
	"github.com/ory/viper"
	"github.com/spf13/cobra"
)

// Version of this command
const Version = "v0.0.1"

var (
	// Location of the optional config file.
	config = "~/.faas/config"

	// Enable verbose logging (debug).
	verbose = false
)

// The root of the command tree defines the command name, descriotion, globally
// available flags, etc.  It has no action of its own, such that running the
// resultant binary with no arguments prints the help/usage text.
var root = &cobra.Command{
	Use:   "faas",
	Short: "Function as a Service Manager",
	Long: `Funcion as a Service Manager

Create, run and deploy.`,
	Version:       Version,
	SilenceErrors: true, // we explicitly handle errors in Execute()
	SilenceUsage:  true, // no usage dump on error

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
