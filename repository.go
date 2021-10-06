package function

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-git/go-git/v5"
	"gopkg.in/yaml.v2"
)

const (
	repositoryManifest = "manifest.yaml"
	runtimeManifest    = "manifest.yaml"
	templateManifest   = "manifest.yaml"
)

const (
	// DefaultReadinessEndpoint for final deployed Function instances
	DefaultReadinessEndpoint = "/health/readiness"
	// DefaultLivenessEndpoint for final deployed Function instances
	DefaultLivenessEndpoint = "/health/liveness"
	// DefaultTemplatesPath is the root of the defined repository
	// They are expected to be grouped into directories named by their runtime.
	DefaultTemplatesPath = "."
)

// Repository represents a collection of runtimes, each containing templates.
type Repository0_18 struct {
	// Name of the repository.  Naming things and placing them in a hierarchy is
	// the responsibility of the filesystem; metadata the responsibility of the
	// files within this structure. Therefore the name is not part of the repo.
	// This is the same reason a git repository has its name nowhere in .git and
	// does not need a manifest of its contents:  the filesystem itself maintains
	// this information.  This name is the denormalized view of the filesystem,
	// which defines the name as the directory name, and supports being defaulted
	// to the value in the .yaml on initial add, which is stored as DefaultName.
	Name string `yaml:"-"`
	// DefaultName is the name indicated by the repository author.
	// Stored in the yaml attribute "name", it is only consulted during initial
	// addition of the repo as the default option.
	DefaultName string `yaml:"name,omitempty"`
	// Version of the repository.
	Version string `yaml:"version,omitempty"`
	// TemplatesPath defins an optional path within the repository at which
	// templates are stored.  By default this is the repository root.
	TemplatesPath string `yaml:"templates,omitempty"`
	// BuildConfig defines builders and buildpacks.  Here it serves as the default
	// option which may be overridden per runtim or per template.
	BuildConfig `yaml:",inline"`
	// HealthEndpoints for all templates in the repository.  Serves as the
	// default option which may be overridden per runtime and per template.
	HealthEndpoints `yaml:"healthEndpoints,omitempty"`
	// Runtimes defined within the repo.
	Runtimes []Runtime

	fs   filesystem // repo's files TODO: upgrade to fs.FS introduced in go1.16
	path string     // path (only valuable when non-embedded, locally installed)
}

// Runtime is a division of templates within a repository of templates for a
// given runtime (source language plus environmentally available services
// and libraries)
type Runtime struct {
	// Name of the runtime
	Name string `yaml:"-"`
	// HealthEndpoints for all templates in the repository.  Serves as the
	// default option which may be overridden per runtime and per template.
	HealthEndpoints `yaml:"healthEndpoints,omitempty"`
	// BuildConfig defines attriutes 'builders' and 'buildpacks'.  Here it serves
	// as the default option which may be overridden per template. Note that
	// unlike HealthEndpoints, it is inline, so no 'buildConfig' attribute is
	// added/expected; rather the Buildpacks and Builders are direct descendants
	// of Runtime.
	BuildConfig `yaml:",inline"`
	// Templates defined for the runtime
	Templates []Template
}

// BuildConfig defines settins for builders or buildpacks
type BuildConfig struct {
	Buildpacks []string          `yaml:"buildpacks,omitempty"`
	Builders   map[string]string `yaml:"builders,omitempty"`
}

// HealthEndpoints specify the liveness and readiness endpoints for a Runtime
type HealthEndpoints struct {
	Liveness  string `yaml:"liveness,omitempty"`
	Readiness string `yaml:"readiness,omitempty"`
}

// URL from whence it was cloned, if a git repo.
func (r *Repository0_18) URL() string {
	return readURL(r.path)
}

// newRepository creates a repository instance from a path on disk.
func newRepository(path string) (r Repository0_18, err error) {
	// Build the repository from disk state as the default
	r = Repository0_18{
		Name: filepath.Base(path),
		HealthEndpoints: HealthEndpoints{
			Liveness:  DefaultLivenessEndpoint,
			Readiness: DefaultLivenessEndpoint,
		},
		fs:   osFilesystem{},
		path: path,
	}

	// Validate path
	if err = checkDir(r.fs, path); err != nil { // validates repo path
		return r, fmt.Errorf("repository path invald. %v", err)
	}

	// Load repository manifest
	r, err = loadRepositoryManifest(r, path)
	if err != nil {
		return
	}

	// The templates are located at the repo root plus the defined relative
	// templates path (from the manifest)
	templatesPath := filepath.Join(path, r.TemplatesPath)

	// Load templates (by runtime), which use settings on repository as defaults.
	r.Runtimes, err = r.runtimes(templatesPath)
	return
}

// newEmbeddedRepository (encoded ./templates)
// Reads /templates/manifest.yaml and any /template/$RUNTIME/runtime.yaml
// configuration files to populate the Repository struct
func newEmbeddedRepository() (r Repository0_18, err error) {
	r = Repository0_18{
		Name: DefaultRepositoryName,
		fs:   pkgerFilesystem{},
	}
	r.Runtimes, err = r.runtimes(embeddedPath) // special path in embedded
	return
	// The embedded repository does not have a manifest because:
	// 1.  It has no name
	// 2.  Its version is the version of the parent package.
	// 3.  It is not a langauge pack and as such need not redefine where templates
	//     are located within.
	// 4.  BuildConfig and HealthEndpoints are definitionally defaults.
	// The embedded repository also does not valide path, because it is implicit.
}

// newRemoteRepository returns a repository instantiated from a uri of a
// remote git repository.
func newRemoteRepository(uri string) (r Repository0_18, err error) {
	return Repository0_18{}, errors.New("Remote Repositories Not Implemented")
}

// runtimes returns runtimes currently defined in this repository's filesytem.
// The views are denormalized, using the parent repository's values
// for inherited fields BuildConfig and HealthEndpoints as the default values
// for the runtimes and templates.  The runtimes and templates themselves can
// override these values by specifying new values in thir config files.
func (r Repository0_18) runtimes(path string) (runtimes []Runtime, err error) {
	runtimes = []Runtime{}

	// Validate template path. Redundant with the same check in the repo
	// constructor unless alternate templates location is defind.
	if err = checkDir(r.fs, path); err != nil {
		err = fmt.Errorf("templates path invalid. %v", err)
		return
	}

	// Read the templates directory, loading each runtime
	fis, err := r.fs.ReadDir(path)
	if err != nil {
		return
	}

	for _, fi := range fis {
		if !fi.IsDir() || strings.HasPrefix(fi.Name(), ".") {
			continue // ignore files and hidden dirs
		}
		// Runtime
		// Defaults set to the values of the repo for some members, as these are
		// the repo-wide defaults
		runtime := Runtime{
			Name:            fi.Name(),
			BuildConfig:     r.BuildConfig,
			HealthEndpoints: r.HealthEndpoints,
		}
		// Runtime Manifest
		// Load the file if it exists, which may override values inherited from the
		// repo such as builders, buildpacks and health endponts.
		runtime, err = loadRuntimeManifest(r.fs, runtime, filepath.Join(path, fi.Name()))
		// Runtime Templates
		// Load from repo filesystem for runtime. Will inherit values from the
		// runtime such as BuildConfig, HealthEndpoints etc.
		runtime.Templates, err = r.templates(runtime, path)
		runtimes = append(runtimes, runtime)
	}
	return
}

// templates returns templates currently defined in the given runtime's
// filesystem.  The view is denormalized, using the inherited fields from
// the runtime for fileds such as BuildConfig and HealthEndpoints.  The
// template itself can override these by including a manifest.
func (r Repository0_18) templates(runtime Runtime, path string) (templates []Template, err error) {
	templates = []Template{}
	runtimePath := filepath.Join(path, runtime.Name)

	// Validate directory exists
	if err = checkDir(r.fs, runtimePath); err != nil {
		err = fmt.Errorf("runtime path invald. %v", err)
		return
	}

	// Read the directory, loading each template.
	fis, err := r.fs.ReadDir(runtimePath)
	if err != nil {
		return
	}
	for _, fi := range fis {
		if !fi.IsDir() || strings.HasPrefix(fi.Name(), ".") {
			continue // ignore files and hidden dirs
		}
		// Template
		// Defaults set to the values from the runtime for some members, as these
		// are the runtime-wide defaults possibly themselves inherited from repo.
		t := Template{
			Name:            fi.Name(),
			Repository:      r.Name,
			Runtime:         runtime.Name,
			BuildConfig:     runtime.BuildConfig,
			HealthEndpoints: runtime.HealthEndpoints,
		}
		// Template Manifeset
		// Load manifest file if it exists, which may override values inherited from
		// the runtime/repo.
		t, err = loadTemplateManifest(r.fs, t, filepath.Join(runtimePath, fi.Name()))
		templates = append(templates, t)
	}
	return
}

// loadRepositoryManfist from the directory specifed by path (should be the
// repository root).  Returned is the repository with values from the manifest
// populated preferentially.  An error is not returned for a missinge manifest
// (the passed repo is returned unmodifed), but errors decoding the file are.
func loadRepositoryManifest(r Repository0_18, path string) (Repository0_18, error) {
	file, err := r.fs.Open(filepath.Join(path, repositoryManifest))
	if err != nil {
		if os.IsNotExist(err) {
			return r, nil
		}
		return r, err
	}
	decoder := yaml.NewDecoder(file)
	return r, decoder.Decode(&r)
}

// loadRuntimeManifest from the directory specified (runtime root).  Returned
// is the runtime with values from the manifest populated preferentially.  An
// error is not returned for a missing manifest file (the passed runtime is
// returned), but errors decoding the file are.
func loadRuntimeManifest(fs filesystem, r Runtime, path string) (Runtime, error) {
	file, err := fs.Open(filepath.Join(path, runtimeManifest))
	if err != nil {
		if os.IsNotExist(err) {
			return r, nil
		}
		return r, err
	}

	decoder := yaml.NewDecoder(file)
	return r, decoder.Decode(&r)
}

// loadTemplateManifest from the directory specified (template root).  Returned
// is the template with values from the manifest populated preferentailly.  An
// error is not returned for a missing manifest file (the passed template is
// returned), but errors decoding the file are.
func loadTemplateManifest(fs filesystem, t Template, path string) (Template, error) {
	file, err := fs.Open(filepath.Join(path, templateManifest))
	if err != nil {
		if os.IsNotExist(err) {
			return t, nil
		}
		return t, err
	}
	decoder := yaml.NewDecoder(file)
	return t, decoder.Decode(&t)
}

// check that the given path is an accessible directory or error.
// this checks within the given filesystem, which may have its own root.
func checkDir(fs filesystem, path string) error {
	fi, err := fs.Stat(path)
	if err != nil && os.IsNotExist(err) {
		err = fmt.Errorf("path '%v' not found", path)
	} else if err == nil && !fi.IsDir() {
		err = fmt.Errorf("path '%v' is not a directory", path)
	}
	return err
}

// Template from repo for given runtime.
func (r *Repository0_18) Template(runtimeName, name string) (t Template, err error) {
	runtime, err := r.Runtime(runtimeName)
	if err != nil {
		return
	}
	for _, t := range runtime.Templates {
		if t.Name == name {
			return t, nil
		}
	}
	return Template{}, ErrTemplateNotFound
}

// Templates returns the set of all templates for a given runtime.
// If runtime not found, an empty list is returned.
func (r *Repository0_18) Templates(runtimeName string) ([]Template, error) {
	for _, runtime := range r.Runtimes {
		if runtime.Name == runtimeName {
			return runtime.Templates, nil
		}
	}
	return []Template{}, nil
}

// Runtime of the given name within the repository.
func (r *Repository0_18) Runtime(name string) (runtime Runtime, err error) {
	for _, runtime = range r.Runtimes {
		if runtime.Name == name {
			return runtime, err
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
