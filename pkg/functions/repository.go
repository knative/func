package functions

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/go-git/go-billy/v5/memfs"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/storage/memory"
	"gopkg.in/yaml.v2"

	"knative.dev/func/pkg/filesystem"
)

const (
	repositoryManifest = "manifest.yaml"
	runtimeManifest    = "manifest.yaml"
	templateManifest   = "manifest.yaml"
)

const (
	// DefaultReadinessEndpoint for final deployed function instances
	DefaultReadinessEndpoint = "/health/readiness"
	// DefaultLivenessEndpoint for final deployed function instances
	DefaultLivenessEndpoint = "/health/liveness"
	// DefaultTemplatesPath is the root of the defined repository
	DefaultTemplatesPath = "."
	// DefaultInvocationFormat is a named invocation hint for the convenience
	// helper .Invoke.  It is usually set at the template level.  The default
	// ('http') is a plain HTTP POST.
	DefaultInvocationFormat = "http"

	// Defaults for Builder and Builders not expressly defined as a purposeful
	// delegation of choice.
)

// Repository represents a collection of runtimes, each containing templates.
type Repository struct {
	// Name of the repository
	// This can be for instance:
	// directory name on FS or last part of git URL or arbitrary value defined by the Template author.
	Name string
	// Runtimes containing Templates loaded from the repo
	Runtimes []Runtime
	// TODO get rid of fs and uri members
	fs  filesystem.Filesystem
	uri string // URI which was used when initially creating
}

// Runtime is a division of templates within a repository of templates for a
// given runtime (source language plus environmentally available services
// and libraries)
type Runtime struct {
	// Name of the runtime
	Name string
	// Templates defined for the runtime
	Templates []Template
}

// This structure defines defaults for a function when generating project by a template.Write().
// The structure can be read from manifest.yaml which can exist at level of repository, runtime or template.
type funcDefaults struct {
	// BuildConfig defines builders and buildpacks.  the denormalized view of
	// members which can be defined per repo or per runtime first.
	BuildConfig `yaml:",inline"`

	// HealthEndpoints.  The denormalized view of members which can be defined
	// first per repo or per runtime.
	HealthEndpoints `yaml:"healthEndpoints,omitempty"`

	// BuildEnvs defines environment variables related to the builders,
	// this can be used to parameterize the builders
	BuildEnvs []Env `yaml:"envs,omitempty"`

	// RunEnvs defines environment variables used in runtime.
	RunEnvs []Env `yaml:"runEnvs,omitempty"`

	// Invoke defines invocation hints for a functions which is created
	// from this template prior to being materially modified.
	Invoke string `yaml:"invoke,omitempty"`
}

type repositoryConfig struct {
	// repository manifest.yaml can define some default values for func.yaml
	funcDefaults `yaml:",inline"`
	// DefaultName is the name indicated by the repository author.
	// Stored in the yaml attribute "name", it is only consulted during initial
	// addition of the repo as the default option.
	DefaultName string `yaml:"name,omitempty"`
	// Version of the repository.
	Version string `yaml:"version,omitempty"`
	// TemplatesPath defines an optional path within the repository at which
	// templates are stored.  By default this is the repository root.
	TemplatesPath string `yaml:"templates,omitempty"`
}

type runtimeConfig = funcDefaults
type templateConfig = funcDefaults

// NewRepository creates a repository instance from any of: a path on disk, a
// remote or local URI, or from the embedded default repo if uri not provided.
// Name (optional), if provided takes precedence over name derived from repo at
// the given URI.
//
// uri (optional), the path either locally or remote from which to load
// the repository files.  If not provided, the internal default is assumed.
func NewRepository(name, uri string) (r Repository, err error) {
	r = Repository{
		uri: uri,
	}

	fs, err := filesystemFromURI(uri) // Get a Filesystem from the URI
	if err != nil {
		return Repository{}, fmt.Errorf("failed to get repository from URI (%q): %w", uri, err)
	}

	r.fs = fs // needed for Repository.Write()

	repoConfig := repositoryConfig{
		funcDefaults: funcDefaults{
			Invoke: DefaultInvocationFormat,
		},
	}

	repoConfig, err = applyRepositoryManifest(fs, repoConfig) // apply optional manifest to r
	if err != nil {
		return
	}

	// Validate custom path if defined
	if repoConfig.TemplatesPath != "" {
		if err = checkDir(r.fs, repoConfig.TemplatesPath); err != nil {
			err = fmt.Errorf("templates path '%v' does not exist in repo '%v'. %v",
				repoConfig.TemplatesPath, r.Name, err)
			return
		}
	} else {
		repoConfig.TemplatesPath = DefaultTemplatesPath
	}

	r.Name, err = repositoryDefaultName(repoConfig.DefaultName, uri) // choose default name
	if err != nil {
		return
	}
	if name != "" { // If provided, the explicit name takes precedence
		r.Name = name
	}
	r.Runtimes, err = repositoryRuntimes(fs, r.Name, repoConfig) // load templates grouped by runtime
	return
}

// FS returns the underlying filesystem of this repository.
func (r Repository) FS() filesystem.Filesystem {
	return r.fs
}

// filesystemFromURI returns a filesystem from the data located at the
// given URI.  If URI is not provided, indicates the embedded repo should
// be loaded.  URI can be a remote git repository (http:// https:// etc.),
// or a local file path (file://) which can be a git repo or a plain directory.
func filesystemFromURI(uri string) (f filesystem.Filesystem, err error) {
	// If not provided, indicates embedded.
	if uri == "" {
		return EmbeddedTemplatesFS, nil
	}

	if isNonBareGitRepo(uri) {
		return filesystemFromPath(uri)
	}

	// Attempt to get a filesystem from the uri as a remote repo.
	f, err = FilesystemFromRepo(uri)
	if f != nil || err != nil {
		return // found a filesystem and/or an error
	}

	// Attempt to get a filesystem from the uri as a file path.
	return filesystemFromPath(uri)
}

func isNonBareGitRepo(uri string) bool {
	parsed, err := url.Parse(uri)
	if err != nil {
		return false
	}
	if parsed.Scheme != "file" {
		return false
	}
	p := filepath.Join(filepath.FromSlash(uri[7:]), ".git")
	fi, err := os.Stat(p)
	if err != nil {
		return false
	}
	return fi.IsDir()
}

// FilesystemFromRepo attempts to fetch a filesystem from a git repository
// indicated by the given URI.  Returns nil if there is not a repo at the URI.
func FilesystemFromRepo(uri string) (filesystem.Filesystem, error) {
	clone, err := git.Clone(
		memory.NewStorage(),
		memfs.New(),
		getGitCloneOptions(uri),
	)
	if err != nil {
		if isRepoNotFoundError(err) {
			return nil, nil
		}
		if isBranchNotFoundError(err) {
			return nil, fmt.Errorf("failed to clone repository: branch not found for uri %s", uri)
		}
		return nil, fmt.Errorf("failed to clone repository: %w", err)
	}
	wt, err := clone.Worktree()
	if err != nil {
		return nil, err
	}
	return filesystem.NewBillyFilesystem(wt.Filesystem), nil
}

// isRepoNotFoundError returns true if the error is a
// "repository not found" error.
func isRepoNotFoundError(err error) bool {
	// This would be better if the error being tested for was typed, but it is
	// currently a simple string value comparison.
	return (err != nil && err.Error() == "repository not found")
}

func isBranchNotFoundError(err error) bool {
	// This would be better if the error being tested for was typed, but it is
	// currently a simple string value comparison.
	return (err != nil && err.Error() == "reference not found")
}

// filesystemFromPath attempts to return a filesystem from a URI as a file:// path
func filesystemFromPath(uri string) (f filesystem.Filesystem, err error) {
	parsed, err := url.Parse(uri)
	if err != nil {
		return
	}

	if parsed.Scheme != "file" {
		return nil, fmt.Errorf("only file scheme is supported")
	}

	path := filepath.FromSlash(uri[7:])

	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil, fmt.Errorf("path does not exist: %v", path)
	}
	return filesystem.NewOsFilesystem(path), nil
}

// repositoryRuntimes returns runtimes defined in this repository's filesystem.
// The views are denormalized, using the parent repository's values
// for inherited fields BuildConfig and HealthEndpoints as the default values
// for the runtimes and templates.  The runtimes and templates themselves can
// override these values by specifying new values in thir config files.
func repositoryRuntimes(fs filesystem.Filesystem, repoName string, repoConfig repositoryConfig) (runtimes []Runtime, err error) {
	runtimes = []Runtime{}

	// Load runtimes
	fis, err := fs.ReadDir(repoConfig.TemplatesPath)
	if err != nil {
		return
	}
	for _, fi := range fis {
		// ignore files and hidden dirs
		if !fi.IsDir() || strings.HasPrefix(fi.Name(), ".") {
			continue
		}
		// ignore the reserved word "certs"
		if fi.Name() == "certs" {
			continue
		}
		// Runtime, defaulted to values inherited from the repository
		runtime := Runtime{
			Name: fi.Name(),
		}
		// Runtime Manifest
		// Load the file if it exists, which may override values inherited from the
		// repo such as builders, buildpacks and health endpoints.
		var rtConfig runtimeConfig
		rtConfig, err = applyRuntimeManifest(fs, runtime.Name, repoConfig)
		if err != nil {
			return
		}

		// Runtime Templates
		// Load from repo filesystem for runtime. Will inherit values from the
		// runtime such as BuildConfig, HealthEndpoints etc.
		runtime.Templates, err = runtimeTemplates(fs, repoConfig.TemplatesPath, repoName, runtime.Name, rtConfig)
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
// The reserved word "scaffolding" is used for repository-defined scaffolding
// code and is not listed as a template.
func runtimeTemplates(fs filesystem.Filesystem, templatesPath, repoName, runtimeName string, runtimeConfig runtimeConfig) (templates []Template, err error) {
	// Validate runtime directory exists and is a directory
	runtimePath := path.Join(templatesPath, runtimeName)
	if err = checkDir(fs, runtimePath); err != nil {
		err = fmt.Errorf("runtime path '%v' not found. %v", runtimePath, err)
		return
	}

	// Read the directory, loading each template.
	fis, err := fs.ReadDir(runtimePath)
	if err != nil {
		return
	}
	for _, fi := range fis {
		// ignore files and hidden dirs
		if !fi.IsDir() || strings.HasPrefix(fi.Name(), ".") {
			continue
		}
		// ignore the reserved word "scaffolding"
		if fi.Name() == "scaffolding" {
			continue
		}
		// Template, defaulted to values inherited from the runtime
		t := template{
			name:       fi.Name(),
			repository: repoName,
			runtime:    runtimeName,
			config:     runtimeConfig,
			fs:         filesystem.NewSubFS(path.Join(runtimePath, fi.Name()), fs),
		}

		// Template Manifeset
		// Load manifest file if it exists, which may override values inherited from
		// the runtime/repo.
		t, err = applyTemplateManifest(fs, templatesPath, t)
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
	// explicit name takes precedence
	if name != "" {
		return name, nil
	}
	// URI-derived is second precedence
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
func applyRepositoryManifest(fs filesystem.Filesystem, repoConfig repositoryConfig) (repositoryConfig, error) {
	file, err := fs.Open(repositoryManifest)
	if err != nil {
		if os.IsNotExist(err) {
			return repoConfig, nil
		}
		return repoConfig, err
	}
	defer file.Close()
	decoder := yaml.NewDecoder(file)
	return repoConfig, decoder.Decode(&repoConfig)
}

// applyRuntimeManifest from the directory specified (runtime root).  Returned
// is the runtime with values from the manifest populated preferentially.  An
// error is not returned for a missing manifest file (the passed runtime is
// returned), but errors decoding the file are.
func applyRuntimeManifest(fs filesystem.Filesystem, runtimeName string, repoConfig repositoryConfig) (runtimeConfig, error) {
	rtCfg := repoConfig.funcDefaults
	file, err := fs.Open(path.Join(repoConfig.TemplatesPath, runtimeName, runtimeManifest))
	if err != nil {
		if os.IsNotExist(err) {
			return rtCfg, nil
		}
		return rtCfg, err
	}
	defer file.Close()
	decoder := yaml.NewDecoder(file)
	return rtCfg, decoder.Decode(&rtCfg)
}

// applyTemplateManifest from the directory specified (template root).  Returned
// is the template with values from the manifest populated preferentailly.  An
// error is not returned for a missing manifest file (the passed template is
// returned), but errors decoding the file are.
func applyTemplateManifest(fs filesystem.Filesystem, templatesPath string, t template) (template, error) {
	file, err := fs.Open(path.Join(templatesPath, t.runtime, t.Name(), templateManifest))
	if err != nil {
		if os.IsNotExist(err) {
			return t, nil
		}
		return t, err
	}
	defer file.Close()
	decoder := yaml.NewDecoder(file)
	return t, decoder.Decode(&t.config)
}

// check that the given path is an accessible directory or error.
// this checks within the given filesystem, which may have its own root.
func checkDir(fs filesystem.Filesystem, path string) error {
	fi, err := fs.Stat(path)
	if err != nil && os.IsNotExist(err) {
		err = fmt.Errorf("path '%v' not found", path)
	} else if err == nil && !fi.IsDir() {
		err = fmt.Errorf("path '%v' is not a directory", path)
	}
	return err
}

func getGitCloneOptions(uri string) *git.CloneOptions {
	branch := ""
	splitUri := strings.Split(uri, "#")
	if len(splitUri) > 1 {
		uri = splitUri[0]
		branch = splitUri[1]
	}

	opt := &git.CloneOptions{URL: uri, Depth: 1, Tags: git.NoTags,
		RecurseSubmodules: git.NoRecurseSubmodules}
	if branch != "" {
		opt.ReferenceName = plumbing.NewBranchReferenceName(branch)
	}
	return opt
}

// Template from repo for given runtime.
func (r *Repository) Template(runtimeName, name string) (t Template, err error) {
	runtime, err := r.Runtime(runtimeName)
	if err != nil {
		return
	}
	for _, t := range runtime.Templates {
		if t.Name() == name {
			return t, nil
		}
	}
	return nil, ErrTemplateNotFound
}

// Templates returns the set of all templates for a given runtime.
// If runtime not found, an empty list is returned.
func (r *Repository) Templates(runtimeName string) ([]Template, error) {
	for _, runtime := range r.Runtimes {
		if runtime.Name == runtimeName {
			return runtime.Templates, nil
		}
	}
	return nil, nil
}

// Runtime of the given name within the repository.
func (r *Repository) Runtime(name string) (runtime Runtime, err error) {
	if name == "" {
		return Runtime{}, ErrRuntimeRequired
	}
	for _, runtime = range r.Runtimes {
		if runtime.Name == name {
			return runtime, err
		}
	}
	return Runtime{}, ErrRuntimeNotFound
}

// Write all files in the repository to the given path.
func (r *Repository) Write(dest string) (err error) {
	if r.fs == nil {
		return errors.New("the write operation is not supported on this repo")
	}

	fs := r.fs // The FS to copy

	// NOTE
	// We re-load in-memory git repos via a temp directory to avoid what
	// appears to be a missing .git directory in the default worktree FS.
	//
	// This missing .git dir is usually not an issue when utilizing the
	// repository's filesystem (for writing templates, etc), but it does cause
	// problems here where we are writing the entire repository to disk (cloning).
	// We effectively want a full clone with a working tree. So here we do a
	// plain clone first to a temp directory and then copy the files on disk
	// using a regular file copy operation which thus includes the repo metadata.
	if _, ok := r.fs.(filesystem.BillyFilesystem); ok {
		var (
			tempDir string
			clone   *git.Repository
			wt      *git.Worktree
		)
		if tempDir, err = os.MkdirTemp("", "func"); err != nil {
			return
		}
		if clone, err = git.PlainClone(tempDir, false, // not bare
			getGitCloneOptions(r.uri)); err != nil {
			return fmt.Errorf("failed to plain clone repository: %w", err)
		}
		if wt, err = clone.Worktree(); err != nil {
			return fmt.Errorf("failed to get worktree: %w", err)
		}
		fs = filesystem.NewBillyFilesystem(wt.Filesystem)
	}
	return filesystem.CopyFromFS(".", dest, fs)
}

// URL attempts to read the remote git origin URL of the repository.  Best
// effort; returns empty string if the repository is not a git repo or the repo
// has been mutated beyond recognition on disk (ex: removing the origin remote)
func (r *Repository) URL() string {
	uri := r.uri

	// The default builtin repository is indicated by an empty URI.
	// It has no remote URL, and without this check the current working directory
	// would be checked.
	if uri == "" {
		return ""
	}

	// git.PlainOpen does not seem to
	if strings.HasPrefix(uri, "file://") {
		uri = filepath.FromSlash(r.uri[7:])
	}

	repo, err := git.PlainOpen(uri)
	if err != nil {
		return "" // not a git repository
	}

	c, err := repo.Config()
	if err != nil {
		return "" // Has no .git/config or other error.
	}

	ref, _ := repo.Head()
	if _, ok := c.Remotes["origin"]; ok {
		urls := c.Remotes["origin"].URLs
		if len(urls) > 0 {
			return urls[0] + "#" + ref.Name().Short()
		}
	}
	return ""
}
