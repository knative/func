package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v2"
	fn "knative.dev/func"
	"knative.dev/func/builders"
	"knative.dev/func/k8s"
	"knative.dev/func/openshift"
)

const (
	// Filename into which Config is serialized
	Filename = "config.yaml"

	// Repositories is the default directory for repositoires.
	Repositories = "repositories"

	// DefaultLanguage is intentionaly undefined.
	DefaultLanguage = ""

	// DefaultBuilder is statically defined by the builders package.
	DefaultBuilder = builders.Default
)

// DefaultNamespace for remote operations is the currently active
// context namespace (if available) or the fallback "default".
func DefaultNamespace() (namespace string) {
	var err error
	if namespace, err = k8s.GetNamespace(""); err != nil {
		return "default"
	}
	return
}

// Global configuration settings.
type Global struct {
	Builder   string `yaml:"builder,omitempty"`
	Confirm   bool   `yaml:"confirm,omitempty"`
	Language  string `yaml:"language,omitempty"`
	Namespace string `yaml:"namespace,omitempty"`
	Registry  string `yaml:"registry,omitempty"`
	Verbose   bool   `yaml:"verbose,omitempty"`
}

// New Config struct with all members set to static defaults.  See NewDefaults
// for one which further takes into account the optional config file.
func New() Global {
	return Global{
		Builder:   DefaultBuilder,
		Language:  DefaultLanguage,
		Namespace: DefaultNamespace(),
		// ...
	}
}

// RegistyDefault is a convenience method for deferred calculation of a
// default registry taking into account both the global config file and cluster
// detection.
func (c Global) RegistryDefault() string {
	// If defined, the user's choice for global registry default value is used
	if c.Registry != "" {
		return c.Registry
	}
	switch {
	case openshift.IsOpenShift():
		return openshift.GetDefaultRegistry()
	default:
		return ""
	}
}

// NewDefault returns a config populated by global defaults as defined by the
// config file located in .Path() (the global func settings path, which is
//
//	usually ~/.config/func).
//
// The config path is not required to be present.
func NewDefault() (cfg Global, err error) {
	cfg = New()
	cp := File()
	bb, err := os.ReadFile(cp)
	if err != nil {
		if os.IsNotExist(err) {
			err = nil // config file is not required
		}
		return
	}
	err = yaml.Unmarshal(bb, &cfg) // cfg now has applied config.yaml
	return
}

// Load the config exactly as it exists at path (no static defaults)
func Load(path string) (c Global, err error) {
	bb, err := os.ReadFile(path)
	if err != nil {
		return c, fmt.Errorf("error reading global config: %v", err)
	}
	err = yaml.Unmarshal(bb, &c)
	return
}

// Write the config to the given path
func (c Global) Write(path string) (err error) {
	bb, _ := yaml.Marshal(&c) // Marshaling no longer errors; this is back compat
	return os.WriteFile(path, bb, os.ModePerm)
}

// Apply populated values from a function to the config.
// The resulting config is global settings overridden by a given function.
func (c Global) Apply(f fn.Function) Global {
	// With no way to automate this mapping easily (even with reflection) because
	// the function now has a complex structure consiting of XSpec sub-structs,
	// and in some cases has differing member names (language).  While this is
	// yes a bit tedious, manually mapping each member (if defined) is simple,
	// easy to understand and support; with both mapping direction (Apply and
	// Configure) in one central place here... with tests.
	if f.Build.Builder != "" {
		c.Builder = f.Build.Builder
	}
	if f.Runtime != "" {
		c.Language = f.Runtime
	}
	if f.Deploy.Namespace != "" {
		c.Namespace = f.Deploy.Namespace
	}
	if f.Registry != "" {
		c.Registry = f.Registry
	}
	return c
}

// Configure a function with poipulated values of the config.
// The resulting function is the function overridden by values on config.
func (c Global) Configure(f fn.Function) fn.Function {
	if c.Builder != "" {
		f.Build.Builder = c.Builder
	}
	if c.Language != "" {
		f.Runtime = c.Language
	}
	if c.Namespace != "" {
		f.Deploy.Namespace = c.Namespace
	}
	if c.Registry != "" {
		f.Registry = c.Registry
	}
	return f
}

// Dir is derived in the following order, from lowest
// to highest precedence.
//  1. The default path is the zero value, indicating "no config path available",
//     and users of this package should act accordingly.
//  2. ~/.config/func if it exists (can be expanded: user has a home dir)
//  3. The value of $XDG_CONFIG_PATH/func if the environment variable exists.
//
// The path is created if it does not already exist.
func Dir() (path string) {
	// Use home if available
	if home, err := os.UserHomeDir(); err == nil {
		path = filepath.Join(home, ".config", "func")
	}

	// 'XDG_CONFIG_HOME/func' takes precedence if defined
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		path = filepath.Join(xdg, "func")
	}

	return
}

// File returns the full path at which to look for a config file.
// Use FUNC_CONFIG_FILE to override default.
func File() string {
	path := filepath.Join(Dir(), Filename)
	if e := os.Getenv("FUNC_CONFIG_FILE"); e != "" {
		path = e
	}
	return path
}

// RepositoriesPath returns the full path at which to look for repositories.
// Use FUNC_REPOSITORIES_PATH to override default.
func RepositoriesPath() string {
	path := filepath.Join(Dir(), Repositories)
	if e := os.Getenv("FUNC_REPOSITORIES_PATH"); e != "" {
		path = e
	}
	return path
}

// CreatePaths is a convenience function for creating the on-disk func config
// structure.  All operations should be tolerant of nonexistant disk
// footprint where possible (for example listing repositories should not
// require an extant path, but _adding_ a repository does require that the func
// config structure exist.
// Current structure is:
// ~/.config/func
// ~/.config/func/repositories
func CreatePaths() (err error) {
	if err = os.MkdirAll(Dir(), os.ModePerm); err != nil {
		return fmt.Errorf("error creating global config path: %v", err)
	}
	if err = os.MkdirAll(RepositoriesPath(), os.ModePerm); err != nil {
		return fmt.Errorf("error creating global config repositories path: %v", err)
	}
	return
}
