package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/ory/viper"
	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/util/sets"
	"knative.dev/client/pkg/util"
	fn "knative.dev/kn-plugin-func"
)

type RootCommandConfig struct {
	Name string // usually `func` or `kn func`
	Version
	NewClient ClientFactory
}

// NewRootCmd creates the root of the command tree defines the command name, description, globally
// available flags, etc.  It has no action of its own, such that running the
// resultant binary with no arguments prints the help/usage text.
func NewRootCmd(config RootCommandConfig) *cobra.Command {
	cmd := &cobra.Command{
		// Use must be set to exactly config.Name, as this field is overloaded to
		// be used in subcommand help text as the command with possible prefix:
		Use:           config.Name,
		Short:         "Serverless Functions",
		SilenceErrors: true, // we explicitly handle errors in Execute()
		SilenceUsage:  true, // no usage dump on error
		Long: `Serverless Functions {{.Version}}

	Create, build and deploy Knative Functions

SYNOPSIS
	{{.Name}} [-v|--verbose] <command> [args]

EXAMPLES

	o Create a Node Function in the current directory
	  $ {{.Name}} create --language node .

	o Deploy the Function defined in the current working directory to the
	  currently connected cluster, specifying a container registry in place of
	  quay.io/user for the Function's container.
	  $ {{.Name}} deploy --registry quay.io.user

	o Invoke the Function defined in the current working directory with an example
	  request.
	  $ {{.Name}} invoke

	For more examples, see '{{.Name}} <command> --help'.`,
	}

	// Environment Variables
	// Evaluated first after static defaults, set all flags to be associated with
	// a version prefixed by "FUNC_"
	viper.AutomaticEnv()       // read in environment variables for FUNC_<flag>
	viper.SetEnvPrefix("func") // ensure thay all have the prefix

	// Flags
	// persistent flags are available to all subcommands implicitly
	// Note they are bound immediately here as opposed to other subcommands
	// because this root command is not actually executed during tests, and
	// therefore PreRunE and other event-based listeners are not invoked.
	cmd.PersistentFlags().BoolP("verbose", "v", false, "Print verbose logs ($FUNC_VERBOSE)")
	if err := viper.BindPFlag("verbose", cmd.PersistentFlags().Lookup("verbose")); err != nil {
		fmt.Fprintf(os.Stderr, "error binding flag: %v\n", err)
	}
	cmd.PersistentFlags().StringP("namespace", "n", "", "The namespace on the cluster used for remote commands. By default, the namespace func.yaml is used or the currently active namespace if not set in the configuration. (Env: $FUNC_NAMESPACE)")
	if err := viper.BindPFlag("namespace", cmd.PersistentFlags().Lookup("namespace")); err != nil {
		fmt.Fprintf(os.Stderr, "error binding flag: %v\n", err)
	}

	// Version
	cmd.Version = config.Version.String()
	cmd.SetVersionTemplate(`{{printf "%s\n" .Version}}`)

	// Client
	// Use the provided ClientFactory or default to NewClient
	newClient := config.NewClient
	if newClient == nil {
		newClient = NewClient
	}

	cmd.AddCommand(NewCreateCmd(newClient))
	cmd.AddCommand(NewConfigCmd())
	cmd.AddCommand(NewBuildCmd(newClient))
	cmd.AddCommand(NewDeployCmd(newClient))
	cmd.AddCommand(NewDeleteCmd(newClient))
	cmd.AddCommand(NewInfoCmd(newClient))
	cmd.AddCommand(NewListCmd(newClient))
	cmd.AddCommand(NewInvokeCmd(newClient))
	cmd.AddCommand(NewRepositoryCmd(newClient))
	cmd.AddCommand(NewRunCmd(newClient))
	cmd.AddCommand(NewCompletionCmd())
	cmd.AddCommand(NewVersionCmd(config.Version))

	// Help
	// Overridden to process the help text as a template and have
	// access to the provided Client instance.
	cmd.SetHelpFunc(func(cmd *cobra.Command, args []string) {
		runRootHelp(cmd, args, config.Version)
	})

	return cmd

	// NOTE Default Action
	// No default action is provided triggering the default of displaying the help
}

func runRootHelp(cmd *cobra.Command, args []string, version Version) {
	var (
		body = cmd.Long + "\n\n" + cmd.UsageString()
		t    = template.New("root")
		tpl  = template.Must(t.Parse(body))
	)
	var data = struct {
		Name    string
		Version Version
	}{
		Name:    cmd.Root().Use,
		Version: version,
	}

	if err := tpl.Execute(cmd.OutOrStdout(), data); err != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "unable to display help text: %v", err)
	}
}

// Helpers
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

// bindFunc which conforms to the cobra PreRunE method signature
type bindFunc func(*cobra.Command, []string) error

// bindEnv returns a bindFunc that binds env vars to the named flags.
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

type functionOverrides struct {
	Image     string
	Namespace string
}

// functionWithOverrides sets the namespace and image strings for the
// Function project at root, if provided, and returns the Function
// configuration values.
// Please note that When this function is called, the overrides are not persisted.
func functionWithOverrides(root string, overrides functionOverrides) (f fn.Function, err error) {
	f, err = fn.NewFunction(root)
	if err != nil {
		return
	}

	overrideMapping := []struct {
		src  string
		dest *string
	}{
		{overrides.Image, &f.Image},
		{overrides.Namespace, &f.Namespace},
	}

	for _, m := range overrideMapping {
		if m.src != "" {
			*m.dest = m.src
		}
	}

	return
}

// deriveName returns the explicit value (if provided) or attempts to derive
// from the given path.  Path is defaulted to current working directory, where
// a Function configuration, if it exists and contains a name, is used.
func deriveName(explicitName string, path string) string {
	// If the name was explicitly provided, use it.
	if explicitName != "" {
		return explicitName
	}

	// If the directory at path contains an initialized Function, use the name therein
	f, err := fn.NewFunction(path)
	if err == nil && f.Name != "" {
		return f.Name
	}

	return ""
}

// deriveNameAndAbsolutePathFromPath returns resolved Function name and absolute path
// to the Function project root. The input parameter path could be one of:
// 'relative/path/to/foo', '/absolute/path/to/foo', 'foo' or ''
func deriveNameAndAbsolutePathFromPath(path string) (string, string) {
	var absPath string

	// If path is not specifed, we would like to use current working dir
	if path == "" {
		path = cwd()
	}

	// Expand the passed Function name to its absolute path
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", ""
	}

	// Get the name of the Function, which equals to name of the current directory
	pathParts := strings.Split(strings.TrimRight(path, string(os.PathSeparator)), string(os.PathSeparator))
	return pathParts[len(pathParts)-1], absPath
}

// deriveImage returns the same image name which will be used if no explicit
// image is provided.  I.e. derived from the configured registry (registry
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
// --image or $FUNC_IMAGE).  Otherwise, the Function at 'path' is loaded, and
// the Image name therein is used (i.e. it was previously calculated).
// Finally, the default registry is used, which is prepended to the Function
// name, and appended with ':latest':
func deriveImage(explicitImage, defaultRegistry, path string) string {
	if explicitImage != "" {
		return explicitImage // use the explicit value provided.
	}
	f, err := fn.NewFunction(path)
	if err != nil {
		return "" // unable to derive due to load error (uninitialized?)
	}
	if f.Image != "" {
		return f.Image // use value previously provided or derived.
	}
	derivedValue, _ := fn.DerivedImage(path, defaultRegistry)
	return derivedValue // Use the func system's derivation logic.
}

func envFromCmd(cmd *cobra.Command) (*util.OrderedMap, []string, error) {
	if cmd.Flags().Changed("env") {
		env, err := cmd.Flags().GetStringArray("env")
		if err != nil {
			return nil, []string{}, fmt.Errorf("Invalid --env: %w", err)
		}
		return util.OrderedMapAndRemovalListFromArray(env, "=")
	}
	return util.NewOrderedMap(), []string{}, nil
}

func mergeEnvs(envs []fn.Env, envToUpdate *util.OrderedMap, envToRemove []string) ([]fn.Env, int, error) {
	updated := sets.NewString()

	var counter int

	for i := range envs {
		if envs[i].Name != nil {
			value, present := envToUpdate.GetString(*envs[i].Name)
			if present {
				envs[i].Value = &value
				updated.Insert(*envs[i].Name)
				counter++
			}
		}
	}

	it := envToUpdate.Iterator()
	for name, value, ok := it.NextString(); ok; name, value, ok = it.NextString() {
		if !updated.Has(name) {
			n := name
			v := value
			envs = append(envs, fn.Env{Name: &n, Value: &v})
			counter++
		}
	}

	for _, name := range envToRemove {
		for i, envVar := range envs {
			if *envVar.Name == name {
				envs = append(envs[:i], envs[i+1:]...)
				counter++
				break
			}
		}
	}

	errMsg := fn.ValidateEnvs(envs)
	if len(errMsg) > 0 {
		return []fn.Env{}, 0, fmt.Errorf(strings.Join(errMsg, "\n"))
	}

	return envs, counter, nil
}

// setPathFlag ensures common text/wording when the --path flag is used
func setPathFlag(cmd *cobra.Command) {
	cmd.Flags().StringP("path", "p", cwd(), "Path to the project directory (Env: $FUNC_PATH)")
}

type Version struct {
	// Date of compilation
	Date string
	// Version tag of the git commit, or 'tip' if no tag.
	Vers string
	// Hash of the currently active git commit on build.
	Hash string
	// Verbose printing enabled for the string representation.
	Verbose bool
}

// Return the stringification of the Version struct, which takes into account
// the verbosity setting.
func (v Version) String() string {
	if v.Verbose {
		return v.StringVerbose()
	}

	// Ensure that the value returned is parseable as a semver, with the special
	// value v0.0.0 as the default indicating there is no version information
	// available.
	if strings.HasPrefix(v.Vers, "v") {
		// TODO: this is the naive approach, perhaps consider actually parse it
		// using the semver lib
		return v.Vers
	}

	// Any non-semver value is invalid, and thus indistinguishable from a
	// nonexistent version value, so the default zero value of v0.0.0 is used.
	return "v0.0.0"
}

// StringVerbose returns the verbose version of the version stringification.
// The format returned is [semver]-[hash]-[date] where the special value
// 'v0.0.0' and 'source' are used when version is not available and/or the
// libray has been built from source, respectively.
func (v Version) StringVerbose() string {
	var (
		vers = v.Vers
		hash = v.Hash
		date = v.Date
	)
	if vers == "" {
		vers = "v0.0.0"
	}
	if hash == "" {
		hash = "source"
	}
	return fmt.Sprintf("%s-%s-%s", vers, hash, date)
}

// surveySelectDefault returns 'value' if defined and exists in 'options'.
// Otherwise, options[0] is returned if it exists.  Empty string otherwise.
//
// Usage Example:
//
//  languages := []string{ "go", "node", "rust" },
//  survey.Select{
//    Options: options,
//    Default: surveySelectDefaut(cfg.Language, languages),
//  }
//
// Summary:
//
// This protects against an incorrectly initialized survey.Select when the user
// has provided a nonexistant option (validation is handled elsewhere) or
// when a value is required but there exists no defaults (no default value on
// the associated flag).
//
// Explanation:
//
// The above example chooses the default for the Survey (--confirm) question
// in a way that works with user-provided flag and environment variable values.
//  `cfg.Language` is the current value set in the config struct, which is
//     populated from (in ascending order of precedence):
//     static flag default, associated environment variable, or command flag.
//  `languages` are the options which are being used by the survey select.
//
// This cascade allows for the Survey questions to be properly pre-initialzed
// with their associated environment variables or flags.  For example,
// A user whose default language is set to 'node' using the global environment
// variable FUNC_LANGUAGE will have that option pre-selected when running
// `func create -c`.
//
// The 'survey' package expects the value of the Default member to exist
// in the 'Options' member.  This is not possible when user-provided data is
// allowed for the default, hence this logic is necessary.
//
// For example, when the user is using prompts (--confirm) to select from a set
// of options, but the associated flag either has an unrecognized value, or no
// value at all, without this logic the resulting select prompt would be
// initialized with this as the default value, and the act of what appears to
// be choose the first option displayed does not overwrite the invalid default.
// It could perhaps be argued this is a shortcoming in the survey package, but
// it is also clearly an error to provide invalid data for a default.
func surveySelectDefault(value string, options []string) string {
	for _, v := range options {
		if value == v {
			return v // The provided value is acceptable
		}
	}
	if len(options) > 0 {
		return options[0] // Sync with the option which will be shown by the UX
	}
	// Either the value is not an option or there are no options.  Either of
	// which should fail proper validation
	return ""
}

// defaultTemplatedHelp evaluates the given command's help text as a template
// some commands define their own help command when additional values are
// required beyond these basics.
func defaultTemplatedHelp(cmd *cobra.Command, args []string) {
	var (
		body = cmd.Long + "\n\n" + cmd.UsageString()
		t    = template.New("help")
		tpl  = template.Must(t.Parse(body))
	)
	var data = struct{ Name string }{Name: cmd.Root().Use}

	if err := tpl.Execute(cmd.OutOrStdout(), data); err != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "unable to display help text: %v", err)
	}
}
