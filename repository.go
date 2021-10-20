package function

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-git/go-billy/v5/memfs"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/storage/memory"
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
	DefaultTemplatesPath = "."

	// Defaults for Builder and Builders not expressly defined as a pourposeful
	// delegation of choice.
)

// Repository represents a collection of runtimes, each containing templates.
type Repository struct {
	// Name of the repository.  Naming things and placing them in a hierarchy is
	// the responsibility of the filesystem; metadata the responsibility of the
	// files within this structure. Therefore the name is not part of the repo.
	// This is the same reason a git repository has its name nowhere in .git and
	// does not need a manifest of its contents:  the filesystem itself maintains
	// this information.  This name is the denormalized view of the filesystem,
	// which defines the name as the directory name, and supports being defaulted
	// to the value in the .yaml on initial add, which is stored as DefaultName.
	Name string `yaml:"-"` // use filesystem for names
	// DefaultName is the name indicated by the repository author.
	// Stored in the yaml attribute "name", it is only consulted during initial
	// addition of the repo as the default option.
	DefaultName string `yaml:"name,omitempty"`
	// Version of the repository.
	Version string `yaml:"version,omitempty"`
	// TemplatesPath defines an optional path within the repository at which
	// templates are stored.  By default this is the repository root.
	TemplatesPath string `yaml:"templates,omitempty"`
	// BuildConfig defines builders and buildpacks.  Here it serves as the default
	// option which may be overridden per runtime or per template.
	BuildConfig `yaml:",inline"`
	// HealthEndpoints for all templates in the repository.  Serves as the
	// default option which may be overridden per runtime and per template.
	HealthEndpoints `yaml:"healthEndpoints,omitempty"`
	// Runtimes containing Templates loaded from the repo
	Runtimes []Runtime
	// FS is the filesystem underlying the repository, loaded from URI
	// TODO upgrade to fs.FS introduced in go1.16
	FS Filesystem

	uri string // URI which was used when initially creating

}

// Runtime is a division of templates within a repository of templates for a
// given runtime (source language plus environmentally available services
// and libraries)
type Runtime struct {
	// Name of the runtime
	Name string `yaml:"-"` // use filesysem for names

	// HealthEndpoints for all templates in the runtime.  May be overridden
	// per template.
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

// HealthEndpoints specify the liveness and readiness endpoints for a Runtime
type HealthEndpoints struct {
	Liveness  string `yaml:"liveness,omitempty"`
	Readiness string `yaml:"readiness,omitempty"`
}

// BuildConfig defines builders and buildpacks
type BuildConfig struct {
	Buildpacks []string          `yaml:"buildpacks,omitempty"`
	Builders   map[string]string `yaml:"builders,omitempty"`
}

// NewRepository creates a repository instance from any of: a path on disk, a
// remote or local URI, or from the embedded default repo if uri not provided.
// Name (optional), if provided takes precidence over name derived from repo at
//   the given URI.
// URI (optional), the path either locally or remote from which to load the
//    the repository files.  If not provided, the internal default is assumed.
func NewRepository(name, uri string) (r Repository, err error) {
	r = Repository{
		uri: uri,
		HealthEndpoints: HealthEndpoints{
			Liveness:  DefaultLivenessEndpoint,
			Readiness: DefaultLivenessEndpoint,
		},
	}
	r.FS, err = filesystemFromURI(uri) // Get a Filesystem from the URI
	if err != nil {
		return
	}
	r, err = applyRepositoryManifest(r) // apply optional manifest to r
	if err != nil {
		return
	}
	r.Name, err = repositoryDefaultName(r.DefaultName, uri) // choose default name
	if err != nil {
		return
	}
	if name != "" { // If provided, the explicit name takes precidence
		r.Name = name
	}
	r.Runtimes, err = repositoryRuntimes(r) // load templates grouped by runtime
	return
}

// filesystemFromURI returns a filesystem from the data located at the
// given URI.  If URI is not provided, indicates the embedded repo should
// be loaded.  URI can be a remote git repository (http:// https:// etc),
// or a local file path (file://) which can be a git repo or a plain directory.
func filesystemFromURI(uri string) (f Filesystem, err error) {
	// If not provided, indicates embedded.
	if uri == "" {
		return pkgerFilesystem{}, nil
	}

	// Attempt to get a filesystm from the uri as a remote repo.
	f, err = filesystemFromRepo(uri)
	if f != nil || err != nil {
		return // found a filesystem and/or an error
	}

	// Attempt to get a filesystem from the uri as a file path.
	return filesystemFromPath(uri)
}

// filesystemFromRepo attempts to fetch a filesystem from a git repository
// indicated by the given URI.  Returns nil if there is not a repo at the URI.
func filesystemFromRepo(uri string) (Filesystem, error) {
	clone, err := git.Clone(
		memory.NewStorage(),
		memfs.New(),
		&git.CloneOptions{URL: uri, Depth: 1, Tags: git.NoTags,
			RecurseSubmodules: git.NoRecurseSubmodules,
		})
	if err != nil {
		if isRepoNotFoundError(err) {
			err = nil // no repo at location is an expected condition
		}
		return nil, err
	}
	wt, err := clone.Worktree()
	if err != nil {
		return nil, err
	}
	return billyFilesystem{fs: wt.Filesystem}, nil
}

// isRepoNotFoundError returns true if the error is a
// "repository not found" error.
func isRepoNotFoundError(err error) bool {
	// This would be better if the error being tested for was typed, but it is
	// currently a simple string value comparison.
	return (err != nil && err.Error() == "repository not found")
}

// filesystemFromPath attempts to return a filesystem from a URI as a file:// path
func filesystemFromPath(uri string) (f Filesystem, err error) {
	parsed, err := url.Parse(uri)
	if err != nil {
		return
	}
	if _, err := os.Stat(parsed.Path); os.IsNotExist(err) {
		return nil, fmt.Errorf("path does not exist: %v", parsed.Path)
	}
	return osFilesystem{root: parsed.Path}, nil
}

// repositoryRuntimes returns runtimes defined in this repository's filesytem.
// The views are denormalized, using the parent repository's values
// for inherited fields BuildConfig and HealthEndpoints as the default values
// for the runtimes and templates.  The runtimes and templates themselves can
// override these values by specifying new values in thir config files.
func repositoryRuntimes(r Repository) (runtimes []Runtime, err error) {
	runtimes = []Runtime{}

	// Validate custom path if defined
	if r.TemplatesPath != "" {
		if err = checkDir(r.FS, r.TemplatesPath); err != nil {
			err = fmt.Errorf("templates path '%v' does not exist in repo '%v'. %v",
				r.TemplatesPath, r.Name, err)
			return
		}
	}

	// Load runtimes
	if r.TemplatesPath == "" {
		r.TemplatesPath = "/"
	}

	fis, err := r.FS.ReadDir(r.TemplatesPath)
	if err != nil {
		return
	}
	for _, fi := range fis {
		// ignore files and hidden dirs
		if !fi.IsDir() || strings.HasPrefix(fi.Name(), ".") {
			continue
		}
		// Runtime, defaulted to values inherited from the repository
		runtime := Runtime{
			Name:            fi.Name(),
			BuildConfig:     r.BuildConfig,
			HealthEndpoints: r.HealthEndpoints,
		}
		// Runtime Manifest
		// Load the file if it exists, which may override values inherited from the
		// repo such as builders, buildpacks and health endponts.
		runtime, err = applyRuntimeManifest(r, runtime)
		if err != nil {
			return
		}

		// Runtime Templates
		// Load from repo filesystem for runtime. Will inherit values from the
		// runtime such as BuildConfig, HealthEndpoints etc.
		runtime.Templates, err = runtimeTemplates(r, runtime)
		if err != nil {
			return
		}
		runtimes = append(runtimes, runtime)
	}
	return
}

// runtimeTemplates returns templates currently defined in the given runtime's
// filesystem.  The view is denormalized, using the inherited fields from the
// runtime for defaults of BuildConfig andHealthEndpoints.  The template itself
// can override these by including a manifest.
func runtimeTemplates(r Repository, runtime Runtime) (templates []Template, err error) {
	templates = []Template{}

	// Validate runtime directory exists and is a directory
	runtimePath := filepath.Join(r.TemplatesPath, runtime.Name)
	if err = checkDir(r.FS, runtimePath); err != nil {
		err = fmt.Errorf("runtime path '%v' not found. %v", runtimePath, err)
		return
	}

	// Read the directory, loading each template.
	fis, err := r.FS.ReadDir(runtimePath)
	if err != nil {
		return
	}
	for _, fi := range fis {
		// ignore files and hidden dirs
		if !fi.IsDir() || strings.HasPrefix(fi.Name(), ".") {
			continue
		}
		// Template, defaulted to values inherited from the runtime
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
		t, err = applyTemplateManifest(r, t)
		if err != nil {
			return
		}
		templates = append(templates, t)
	}
	return
}

// repositoryDefaultName returns the given name, which if empty falls back to
// deriving a name from the URI, which if empty then falls back to the
// statically defined default DefaultRepositoryName.
func repositoryDefaultName(name, uri string) (string, error) {
	// explicit name takes precidence
	if name != "" {
		return name, nil
	}
	// URI-derived is second precidence
	if uri != "" {
		parsed, err := url.Parse(uri)
		if err != nil {
			return "", err
		}
		ss := strings.Split(parsed.Path, "/")
		if len(ss) > 0 {
			// name is the last token with optional '.git' suffix removed
			return strings.TrimSuffix(ss[len(ss)-1], ".git"), nil
		}
	}
	// static default
	return DefaultRepositoryName, nil
}

// applyRepositoryManifest from the root of the repository's filesystem if it
// exists.  Returned is the repository with any values from the manifest
// set to those of the manifest.
func applyRepositoryManifest(r Repository) (Repository, error) {
	file, err := r.FS.Open(repositoryManifest)
	if err != nil {
		if os.IsNotExist(err) {
			return r, nil
		}
		return r, err
	}
	decoder := yaml.NewDecoder(file)
	return r, decoder.Decode(&r)
}

// applyRuntimeManifest from the directory specified (runtime root).  Returned
// is the runtime with values from the manifest populated preferentially.  An
// error is not returned for a missing manifest file (the passed runtime is
// returned), but errors decoding the file are.
func applyRuntimeManifest(repo Repository, runtime Runtime) (Runtime, error) {
	file, err := repo.FS.Open(filepath.Join(repo.TemplatesPath, runtime.Name, runtimeManifest))
	if err != nil {
		if os.IsNotExist(err) {
			return runtime, nil
		}
		return runtime, err
	}
	decoder := yaml.NewDecoder(file)
	return runtime, decoder.Decode(&runtime)
}

// applyTemplateManifest from the directory specified (template root).  Returned
// is the template with values from the manifest populated preferentailly.  An
// error is not returned for a missing manifest file (the passed template is
// returned), but errors decoding the file are.
func applyTemplateManifest(repo Repository, t Template) (Template, error) {
	file, err := repo.FS.Open(filepath.Join(repo.TemplatesPath, t.Runtime, t.Name, templateManifest))
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
func checkDir(fs Filesystem, path string) error {
	fi, err := fs.Stat(path)
	if err != nil && os.IsNotExist(err) {
		err = fmt.Errorf("path '%v' not found", path)
	} else if err == nil && !fi.IsDir() {
		err = fmt.Errorf("path '%v' is not a directory", path)
	}
	return err
}

// Template from repo for given runtime.
func (r *Repository) Template(runtimeName, name string) (t Template, err error) {
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
func (r *Repository) Templates(runtimeName string) ([]Template, error) {
	for _, runtime := range r.Runtimes {
		if runtime.Name == runtimeName {
			return runtime.Templates, nil
		}
	}
	return []Template{}, nil
}

// Runtime of the given name within the repository.
func (r *Repository) Runtime(name string) (runtime Runtime, err error) {
	for _, runtime = range r.Runtimes {
		if runtime.Name == name {
			return runtime, err
		}
	}
	return Runtime{}, ErrRuntimeNotFound
}

// Write all files in the repository to the given path.
func (r *Repository) Write(path string) error {
	// NOTE: Writing internal .git directory does not work
	//
	// A quirk of the git library's implementation is that the filesytem
	// returned does not include the .git directory.  This is usually not an
	// issue when utilizing the repository's filesystem (for writing templates),
	// but it does cause problems here (used for installing a repo locally) where
	// we effectively want a full clone.
	// TODO: switch to using a temp directory?

	return copy("/", path, r.FS) // copy 'all' to 'dest' from 'FS'
}

// URL attempts to read the remote git origin URL of the repository.  Best
// effort; returns empty string if the repository is not a git repo or the repo
// has been mutated beyond recognition on disk (ex: removing the origin remote)
func (r *Repository) URL() string {
	repo, err := git.PlainOpen(r.uri)
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
