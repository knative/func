package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/mitchellh/go-homedir"
	"github.com/ory/viper"
	"github.com/spf13/cobra"

	"github.com/boson-project/faas"
)

// The root of the command tree defines the command name, descriotion, globally
// available flags, etc.  It has no action of its own, such that running the
// resultant binary with no arguments prints the help/usage text.
var root = &cobra.Command{
	Use:           "faas",
	Short:         "Function as a Service",
	SilenceErrors: true, // we explicitly handle errors in Execute()
	SilenceUsage:  true, // no usage dump on error
	Long: `Function as a Service

Create and run Functions as a Service.`,
}

// When the code is loaded into memory upon invocation, the cobra/viper packages
// are invoked to gather system context.  This includes reading the configuration
// file, environment variables, and parsing the command flags.
func init() {
	// read in environment variables that match
	viper.AutomaticEnv()

	verbose := viper.GetBool("verbose")

	// Populate the `verbose` flag with the value of --verbose, if provided,
	// which thus overrides both the default and the value read in from the
	// config file (i.e. flags always take highest precidence).
	root.PersistentFlags().BoolVarP(&verbose, "verbose", "v", verbose, "print verbose logs")
	err := viper.BindPFlag("verbose", root.PersistentFlags().Lookup("verbose"))
	if err != nil {
		panic(err)
	}

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
	// Sets version to a string partially populated by compile-time flags.
	root.Version = version.String()

	// Execute the root of the command tree.
	if err := root.Execute(); err != nil {
		// Errors are printed to STDERR output and the process exits with code of 1.
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

// Helper functions used by multiple commands
// ------------------------------------------

// interactiveTerminal returns whether or not the currently attached process
// terminal is interactive.  Used for determining whether or not to
// interactively prompt the user to confirm default choices, etc.
func interactiveTerminal() bool {
	fi, err := os.Stdin.Stat()
	return err == nil && ((fi.Mode() & os.ModeCharDevice) != 0)
}

// cwd returns the current working directory or exits 1 printing the error.
func cwd() (cwd string) {
	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Unable to determine current working directory: %v", err)
		os.Exit(1)
	}
	return cwd
}

// configPath is the effective path to the optional config directory used for
// function defaults and extensible templates.
func configPath() (path string) {
	if path = os.Getenv("XDG_CONFIG_HOME"); path != "" {
		path = filepath.Join(path, "faas")
		return
	}
	home, err := homedir.Expand("~")
	if err != nil {
		fmt.Fprintf(os.Stderr, "could not derive home directory for use as default templates path: %v", err)
		path = filepath.Join(".config", "faas")
	} else {
		path = filepath.Join(home, ".config", "faas")
	}
	return
}

// bindFunc which conforms to the cobra PreRunE method signature
type bindFunc func(*cobra.Command, []string) error

// bindEnv returns a bindFunc that binds env vars to the namd flags.
func bindEnv(flags ...string) bindFunc {
	return func(cmd *cobra.Command, args []string) (err error) {
		for _, flag := range flags {
			if err = viper.BindPFlag(flag, cmd.Flags().Lookup(flag)); err != nil {
				return
			}
		}
		return
	}
}

// overrideImage overwrites (or sets) the value of the Function's .Image
// property, which preempts the default functionality of deriving the value as:
// Deafult:  [config.Repository]/[config.Name]:latest
func overrideImage(root, override string) (err error) {
	if override == "" {
		return
	}
	f, err := faas.NewFunction(root)
	if err != nil {
		return err
	}
	f.Image = override
	return f.WriteConfig()
}

// overrideNamespace overwrites (or sets) the value of the Function's .Namespace
// property, which preempts the default functionality of using the underlying
// platform configuration (if supported).  In the case of Kubernetes, this
// overrides the configured namespace (usually) set in ~/.kube.config.
func overrideNamespace(root, override string) (err error) {
	if override == "" {
		return
	}
	f, err := faas.NewFunction(root)
	if err != nil {
		return err
	}
	f.Namespace = override
	return f.WriteConfig()
}

// functionWithOverrides sets the namespace and image strings for the
// Function project at root, if provided, and returns the Function
// configuration values
func functionWithOverrides(root, namespace, image string) (f faas.Function, err error) {
	if err = overrideNamespace(root, namespace); err != nil {
		return
	}
	if err = overrideImage(root, image); err != nil {
		return
	}
	return faas.NewFunction(root)
}

// deriveName returns the explicit value (if provided) or attempts to derive
// from the given path.  Path is defaulted to current working directory, where
// a function configuration, if it exists and contains a name, is used.  Lastly
// derivation using the path us used.
func deriveName(explicitName string, path string) string {
	// If the name was explicitly provided, use it.
	if explicitName != "" {
		return explicitName
	}
	// If the directory at path contains an initialized Function, use the name therein
	f, err := faas.NewFunction(path)
	if err == nil && f.Name != "" {
		return f.Name
	}
	maxRecursion := faas.DefaultMaxRecursion
	derivedName, _ := faas.DerivedName(path, maxRecursion)
	return derivedName
}

// deriveImage returns the same image name which will be used if no explicit
// image is provided.  I.e. derived from the configured repository (registry
// plus username) and the Function's name.
//
// This is calculated preemptively here in the CLI (prior to invoking the
// client), only in order to provide information to the user via the prompt.
// The client will calculate this same value if the image override is not
// provided.
//
// Derivation logic:
// deriveImage attempts to arrive at a final, full image name:
//   format:  [registry]/[username]/[FunctionName]:[tag]
//   example: quay.io/myname/my.function.name:tag.
//
// Registry can optionally be omitted, in which case DefaultRegistry
// will be prepended.
//
// If the image flag is provided, this value is used directly (the user supplied
// --image or $FAAS_IMAGE).  Otherwise, the Function at 'path' is loaded, and
// the Image name therein is used (i.e. it was previously calculated).
// Finally, the default repository is used, which is prepended to the Function
// name, and appended with ':latest':
func deriveImage(explicitImage, defaultRepo, path string) string {
	if explicitImage != "" {
		return explicitImage // use the explicit value provided.
	}
	f, err := faas.NewFunction(path)
	if err != nil {
		return "" // unable to derive due to load error (uninitialized?)
	}
	if f.Image != "" {
		return f.Image // use value previously provided or derived.
	}
	derivedValue, _ := faas.DerivedImage(path, defaultRepo)
	return derivedValue // Use the faas system's derivation logic.
}
