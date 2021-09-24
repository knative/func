package function

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/go-git/go-git/v5"
	"github.com/markbates/pkger"
	"github.com/markbates/pkger/pkging"
	"gopkg.in/yaml.v2"
)

// Path to builtin repositories.
// note: this constant must be defined in the same file in which it is used due
// to pkger performing static analysis on source files separately.
const builtinRepositories = "/templates"

const ManifestYaml = "manifest.yaml"
const RuntimeYaml = "runtime.yaml"

// Repository represents a collection of one or more runtimes, each with one or more templates
type Repository struct {
	Name            string `yaml:"name"`
	Version         string `yaml:"version,omitempty"`
	URL             string `yaml:"url,omitempty"`
	Runtimes        []Runtime
	HealthEndpoints HealthEndpoints
}

// FunctionTemplates is the collection of templates for a Runtime
type FunctionTemplates []struct {
	Name string `yaml:"name,omitempty"`
	Path string `yaml:"path"`
}

// HealthEndpoints specify the liveness and readiness endpoints for a Runtime
type HealthEndpoints struct {
	Liveness  string `yaml:"liveness,omitempty"`
	Readiness string `yaml:"readiness,omitempty"`
}

// Runtime is a path to a directory containing one or more function templates
type Runtime struct {
	Path       string            `yaml:"path"`
	Name       string            `yaml:"name,omitempty"`
	Buildpacks []string          `yaml:"buildpacks,omitempty"`
	Builders   map[string]string `yaml:"builders,omitempty"`
	Templates  FunctionTemplates
	HealthEndpoints
}

// NewRepositoryFromPath represents the file structure of 'path' at time of construction as
// a Repository with Templates, each of which has a Name and its Runtime.
// a convenience member of Runtimes is the unique, sorted list of all
// runtimes
func NewRepositoryFromPath(path string) (r Repository, err error) {
	// A repository must contain a manifest.yaml at the top level
	manifest := filepath.Join(path, ManifestYaml)
	var bytes []byte
	if bytes, err = os.ReadFile(manifest); err != nil {
		if !os.IsNotExist(err) {
			return
		}
		// If no manifest.yaml file, at least be sure that the repo exists
		_, err := os.Stat(path)
		if os.IsNotExist(err) {
			return r, ErrRepositoryNotFound
		}
		// The repo exists, but no manifest.yaml
		err = nil
		fmt.Println("TODO: Restore directory traversal")
		// TODO: Restore directory traversal
	} else {
		err = yaml.Unmarshal(bytes, &r)
		if err != nil {
			return
		}
	}
	// Read any runtime.yaml files found
	for i, rr := range r.Runtimes {
		yamlPath := filepath.Join(path, rr.Path, RuntimeYaml)
		if _, err = os.Stat(yamlPath); err == nil {
			if bytes, err = ioutil.ReadFile(yamlPath); err == nil {
				if err = yaml.Unmarshal(bytes, &rr); err != nil {
					return
				}
				r.Runtimes[i] = rr
			}
		} else {
			// We don't really care if os.Stat finds that the runtime.yaml
			// file is absent. It's not required. Nilify the error and continue
			if os.IsNotExist(err) {
				err = nil
			}
		}
	}
	// Repository manifest.yaml does not require a URL field
	// If it's not there, check the git remote
	if r.URL == "" {
		r.URL = readURL(path)
	}
	return
}

// NewRepository from builtin (encoded ./templates)
// Reads /templates/manifest.yaml and any /template/$RUNTIME/runtime.yaml
// configuration files to populate the Repository struct
func NewRepositoryFromBuiltin() (r Repository, err error) {
	var (
		path  string // file path
		bytes []byte // bytes from yaml file
	)

	// Read the repository manifest.yaml
	path = filepath.Join(builtinRepositories, ManifestYaml)
	if bytes, err = getBytesFromBuiltinFile(path); err != nil {
		if !os.IsNotExist(err) {
			return
		}
	} else {
		err = yaml.Unmarshal(bytes, &r)
		if err != nil {
			return
		}
	}

	// Read any runtime.yaml files found
	for i, rr := range r.Runtimes {
		path = filepath.Join(builtinRepositories, rr.Path, RuntimeYaml)
		if bytes, err = getBytesFromBuiltinFile(path); err != nil {
			// If we don't find a runtime.yaml file, that's ok
			if os.IsNotExist(err) {
				err = nil
			} else {
				// but if the error is something other than NotExist, bail
				return
			}
		}
		err = yaml.Unmarshal(bytes, &rr)
		r.Runtimes[i] = rr
	}
	return
}

// getBytesFromBuiltinFile reads a file at `path` in the embedded
// template file system and returns the bytes
func getBytesFromBuiltinFile(path string) (bytes []byte, err error) {
	// If the file does not exist return an error
	if _, err = pkger.Stat(path); err != nil {
		return
	}
	var manifest pkging.File
	manifest, err = pkger.Open(path)
	if err != nil {
		return
	}
	bytes, err = ioutil.ReadAll(manifest)
	return
}

// GetTemplate from repo with given runtime
func (r *Repository) GetTemplate(runtime, name string) (t Template, err error) {
	var l Runtime
	l, err = r.GetRuntime(runtime)
	if err != nil {
		return
	}

	for _, t := range l.Templates {
		if t.Path == name {
			return Template{
				Repository: r.Name,
				Runtime:    runtime,
				Name:       t.Path,
			}, nil
		}
	}

	return Template{}, ErrTemplateNotFound
}

// GetTemplates returns the set of function templates for a given runtime
func (r *Repository) GetTemplates(runtime string) (FunctionTemplates, error) {
	for _, l := range r.Runtimes {
		if l.Path == runtime {
			return l.Templates, nil
		}
	}
	return nil, ErrTemplateNotFound
}

// GetRuntime returns the Runtime for the given name in a repository
func (r *Repository) GetRuntime(runtime string) (l Runtime, err error) {
	for _, l = range r.Runtimes {
		if l.Path == runtime {
			return l, err
		}
	}
	return Runtime{}, ErrRuntimeNotFound
}

// readURL attempts to read the remote git origin URL of the repository.  Best
// effort; returns empty string if the repository is not a git repo or the repo
// has been mutated beyond recognition on disk (ex: removing the origin remote)
func readURL(path string) string {
	repo, err := git.PlainOpen(path)
	if err != nil {
		return "" // not a git repository
	}

	c, err := repo.Config()
	if err != nil {
		return "" // Has no .git/config or other error.
	}

	if _, ok := c.Remotes["origin"]; ok {
		urls := c.Remotes["origin"].URLs
		if len(urls) > 0 {
			return urls[0]
		}
	}
	return ""
}
