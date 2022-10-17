package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/mitchellh/go-homedir"
	"gopkg.in/yaml.v2"
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

	// DefaultBuilder defined by builders
	DefaultBuilder = builders.Default
)

func DefaultRegistry() string {
	switch {
	case openshift.IsOpenShift():
		return openshift.GetDefaultRegistry()
	default:
		return ""
	}
}

// DefaultNamespace for remote operations is the currently active
// context namespace (if available) or the fallbacl "default".
func DefaultNamespace() (namespace string) {
	var err error
	if namespace, err = k8s.GetNamespace(""); err != nil {
		return "default"
	}
	return
}

// Global configuration settings.
type Config struct {
	Builder   string `yaml:"builder,omitempty"`
	Confirm   bool   `yaml:"confirm,omitempty"`
	Language  string `yaml:"language,omitempty"`
	Namespace string `yaml:"namespace,omitempty"`
	Registry  string `yaml:"registry,omitempty"`
	Verbose   bool   `yaml:"verbose,omitempty"`
}

// New Config struct with all members set to static defaults.  See NewDefaults
// for one which further takes into account the optional config file.
func New() Config {
	return Config{
		Builder:   DefaultBuilder,
		Language:  DefaultLanguage,
		Namespace: DefaultNamespace(),
		Registry:  DefaultRegistry(),
	}
}

// NewDefault returns a config populated by global defaults as defined by the
// config file located in .Path() (the global func settings path, which is
//  usually ~/.config/func).
// The config path is not required to be present.
func NewDefault() (cfg Config, err error) {
	cfg = New()
	cp := ConfigPath()
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
func Load(path string) (c Config, err error) {
	bb, err := os.ReadFile(path)
	if err != nil {
		return c, fmt.Errorf("error reading global config: %v", err)
	}
	err = yaml.Unmarshal(bb, &c)
	return
}

// Write the config to the given path
func (c Config) Write(path string) (err error) {
	bb, _ := yaml.Marshal(&c) // Marshaling no longer errors; this is back compat
	return os.WriteFile(path, bb, os.ModePerm)
}

// Path is derived in the following order, from lowest
// to highest precedence.
// 1.  The static default is DefaultConfigPath (./.config/func)
// 2.  ~/.config/func if it exists (can be expanded: user has a home dir)
// 3.  The value of $XDG_CONFIG_PATH/func if the environment variable exists.
// The path is created if it does not already exist.
func Path() (path string) {
	// default path is a relative path used in the unlikely event that
	// the user has no home directory (no ~), there is no
	// XDG_CONFIG_HOME set
	path = filepath.Join(".config", "func")

	// ~/.config/func is the default if ~ can be expanded
	if home, err := homedir.Expand("~"); err == nil {
		path = filepath.Join(home, ".config", "func")
	}

	// 'XDG_CONFIG_HOME/func' takes precidence if defined
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		path = filepath.Join(xdg, "func")
	}

	return
}

// ConfigPath returns the full path at which to look for a config file.
// Use FUNC_CONFIG_FILE to override default.
func ConfigPath() string {
	path := filepath.Join(Path(), Filename)
	if e := os.Getenv("FUNC_CONFIG_FILE"); e != "" {
		path = e
	}
	return path
}

// RepositoriesPath returns the full at which to look for repositories.
// Use FUNC_REPOSITORIES_PATH to override default.
func RepositoriesPath() string {
	path := filepath.Join(Path(), Repositories)
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
	if err = os.MkdirAll(Path(), os.ModePerm); err != nil {
		return fmt.Errorf("error creating global config path: %v", err)
	}
	if err = os.MkdirAll(RepositoriesPath(), os.ModePerm); err != nil {
		return fmt.Errorf("error creating global config repositories path: %v", err)
	}
	return
}
