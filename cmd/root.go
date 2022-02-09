package cmd

import (
	"bytes"
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

var exampleTemplate = template.Must(template.New("example").Parse(`
# Create a node function called "node-sample" and enter the directory
{{.}} create myfunc && cd myfunc

# Build the container image, push it to a registry and deploy it to the connected Knative cluster
# (replace <registry/user> with something like quay.io/user with an account that have you access to)
{{.}} deploy --registry <registry/user>

# Curl the service with the service URL
curl $(kn service describe myfunc -o url)
`))

type RootCommandConfig struct {
	Name      string // usually `func` or `kn func`
	Date      string
	Version   string
	Hash      string
	NewClient ClientFactory
}

// NewRootCmd creates the root of the command tree defines the command name, description, globally
// available flags, etc.  It has no action of its own, such that running the
// resultant binary with no arguments prints the help/usage text.
func NewRootCmd(config RootCommandConfig) (*cobra.Command, error) {
	var err error

	root := &cobra.Command{
		Use:           config.Name,
		Short:         "Serverless Functions",
		SilenceErrors: true, // we explicitly handle errors in Execute()
		SilenceUsage:  true, // no usage dump on error
		Long: `Serverless Functions

Create, build and deploy Functions in serverless containers for multiple runtimes on Knative`,
	}

	root.Example, err = replaceNameInTemplate(config.Name, "example")
	if err != nil {
		root.Example = "Usage could not be loaded"
	}

	// read in environment variables that match
	viper.AutomaticEnv()

	verbose := viper.GetBool("verbose")

	// Populate the `verbose` flag with the value of --verbose, if provided,
	// which thus overrides both the default and the value read in from the
	// config file (i.e. flags always take highest precidence).
	root.PersistentFlags().BoolVarP(&verbose, "verbose", "v", verbose, "print verbose logs")
	err = viper.BindPFlag("verbose", root.PersistentFlags().Lookup("verbose"))
	if err != nil {
		return nil, err
	}

	// Override the --version template to match the output format from the
	// version subcommand: nothing but the version.
	root.SetVersionTemplate(`{{printf "%s\n" .Version}}`)

	// Prefix all environment variables with "FUNC_" to avoid collisions with other apps.
	viper.SetEnvPrefix("func")

	version := Version{
		Date: config.Date,
		Vers: config.Version,
		Hash: config.Hash,
	}

	root.Version = version.String()

	newClient := config.NewClient

	if newClient == nil {
		var cleanUp func() error
		newClient, cleanUp = NewDefaultClientFactory()
		root.PersistentPostRunE = func(cmd *cobra.Command, args []string) error {
			return cleanUp()
		}
	}

	root.AddCommand(NewVersionCmd(version))
	root.AddCommand(NewCreateCmd(newClient))
	root.AddCommand(NewConfigCmd())
	root.AddCommand(NewBuildCmd(newClient))
	root.AddCommand(NewDeployCmd(newClient))
	root.AddCommand(NewDeleteCmd(newClient))
	root.AddCommand(NewInfoCmd(newClient))
	root.AddCommand(NewListCmd(newClient))
	root.AddCommand(NewInvokeCmd(newClient))
	root.AddCommand(NewRepositoryCmd(newRepositoryClient))
	root.AddCommand(NewRunCmd(newRunClient))
	root.AddCommand(NewCompletionCmd())

	return root, nil
}

func replaceNameInTemplate(name, template string) (string, error) {
	var buffer bytes.Buffer
	err := exampleTemplate.ExecuteTemplate(&buffer, template, name)
	if err != nil {
		return "", err
	}
	return buffer.String(), nil
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
	Builder   string
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
		{overrides.Builder, &f.Builder},
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

func mergeEnvs(envs []fn.Env, envToUpdate *util.OrderedMap, envToRemove []string) ([]fn.Env, error) {
	updated := sets.NewString()

	for i := range envs {
		if envs[i].Name != nil {
			value, present := envToUpdate.GetString(*envs[i].Name)
			if present {
				envs[i].Value = &value
				updated.Insert(*envs[i].Name)
			}
		}
	}

	it := envToUpdate.Iterator()
	for name, value, ok := it.NextString(); ok; name, value, ok = it.NextString() {
		if !updated.Has(name) {
			n := name
			v := value
			envs = append(envs, fn.Env{Name: &n, Value: &v})
		}
	}

	for _, name := range envToRemove {
		for i, envVar := range envs {
			if *envVar.Name == name {
				envs = append(envs[:i], envs[i+1:]...)
				break
			}
		}
	}

	errMsg := fn.ValidateEnvs(envs)
	if len(errMsg) > 0 {
		return []fn.Env{}, fmt.Errorf(strings.Join(errMsg, "\n"))
	}

	return envs, nil
}

// setPathFlag ensures common text/wording when the --path flag is used
func setPathFlag(cmd *cobra.Command) {
	cmd.Flags().StringP("path", "p", cwd(), "Path to the project directory (Env: $FUNC_PATH)")
}

// setNamespaceFlag ensures common text/wording when the --namespace flag is used
func setNamespaceFlag(cmd *cobra.Command) {
	cmd.Flags().StringP("namespace", "n", "", "The namespace on the cluster. By default, the namespace in func.yaml is used or the currently active namespace if not set in the configuration. (Env: $FUNC_NAMESPACE)")
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

func (v Version) String() string {
	// If 'vers' is not a semver already, then the binary was built either
	// from an untagged git commit (set semver to v0.0.0), or was built
	// directly from source (set semver to v0.0.0-source).
	if strings.HasPrefix(v.Vers, "v") {
		// Was built via make with a tagged commit
		if v.Verbose {
			return fmt.Sprintf("%s-%s-%s", v.Vers, v.Hash, v.Date)
		} else {
			return v.Vers
		}
	} else if v.Vers == "tip" {
		// Was built via make from an untagged commit
		v.Vers = "v0.0.0"
		if v.Verbose {
			return fmt.Sprintf("%s-%s-%s", v.Vers, v.Hash, v.Date)
		} else {
			return v.Vers
		}
	} else {
		// Was likely built from source
		v.Vers = "v0.0.0"
		v.Hash = "source"
		if v.Verbose {
			return fmt.Sprintf("%s-%s", v.Vers, v.Hash)
		} else {
			return v.Vers
		}
	}
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
