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
	manifestFile = "manifest.yaml"

	// DefaultReadinessEndpoint for final deployed function instances
	DefaultReadinessEndpoint = "/health/readiness"

	// DefaultLivenessEndpoint for final deployed function instances
	DefaultLivenessEndpoint = "/health/liveness"

	// DefaultInvocationFormat is a named invocation hint for the convenience
	// helper .Invoke.  It is usually set at the template level.  The default
	// ('http') is a plain HTTP POST.
	DefaultInvocationFormat = "http"

	// Defaults for Builder and Builders not expressly defined as a purposeful
	// delegation of choice.
)

// Repository can be local or remote and contains runtimes
type Repository struct {
	// Runtimes containing Templates loaded from the repo
	Runtimes []Runtime

	repoConfig // values defineable via a manfest.yaml at root level.

	fs  filesystem.Filesystem
	uri string // populated on initial add
}

// Runtime contains templates
type Runtime struct {
	// Name of the runtime
	Name string

	// Templates defined for the runtime
	Templates []Template

	config runtimeConfig // manifest.yaml at root plus runtime level.
}

// see template.go for template definition
// It's config is the root manifest.yaml + runtime manifest.yaml plus
// the template-level.

// repoConfig is the manifest.yaml at the repository level, and
// contains all settings from the runtimeConfig plus a few for the repo.
type repoConfig struct {
	// repository manifest.yaml can define some default values for func.yaml
	runtimeConfig `yaml:",inline"`

	// Name is either directory name on FS or last part of git URL or
	// arbitrary value defined by the Template author or as indicated by the
	// repository author via manifest.yaml.
	Name string `yaml:"name,omitempty"`

	// Version of the repository.
	Version string `yaml:"version,omitempty"`

	// TemplatesPath defines an optional path within the repository at which
	// templates are stored.  By default this is the repository root.
	TemplatesPath string `yaml:"templates,omitempty"`
}

// runtimeConfig is the manifest.yaml at the runtime level, and contains
// all settings from the funcConfig.
type runtimeConfig struct {
	templateConfig `yaml:",inline"`
	// runtime currently contains no level-specific attributes.
}

// templateConfig is the manifest.yaml file in a template directory or the
// runtime parent directory, and is used during template.Write() to set
// values on the newly written Function.
type templateConfig struct {
	// BuildConfig defines builders and buildpacks.  the denormalized view of
	// members which can be defined per repo or per runtime first.
	BuildConfig `yaml:",inline"`

	// HealthEndpoints.  The denormalized view of members which can be defined
	// first per repo or per runtime.
	HealthEndpoints `yaml:"healthEndpoints,omitempty"`

	// BuildEnvs defines environment variables related to the builders,
	// this can be used to parameterize the builders
	BuildEnvs []Env `yaml:"buildEnvs,omitempty"`

	// RunEnvs defines environment variables used in runtime.
	RunEnvs []Env `yaml:"runEnvs,omitempty"`

	// Invoke defines invocation hints for a functions which is created
	// from this template prior to being materially modified.
	Invoke string `yaml:"invoke,omitempty"`
}

// NewRepository creates a repository instance from any of: a path on disk, a
// remote or local URI, or from the embedded default repo if uri not provided.
// Name (optional), if provided takes precedence over name derived from repo at
// the given URI.
//
// uri (optional), the path either locally or remote from which to load
// the repository files.  If not provided, the internal default is assumed.
func NewRepository(name, uri string) (repo Repository, err error) {
	repo = Repository{uri: uri}

	repo.Name, err = repositoryName(name, uri)
	if err != nil {
		return
	}

	repo.fs, err = filesystemFromURI(uri)
	if err != nil {
		return Repository{}, fmt.Errorf("failed to get repository from URI (%q): %w", uri, err)
	}

	repo.repoConfig, err = loadRepoConfig(repo.fs, repo.repoConfig)
	if err != nil {
		return
	}

	repo.Runtimes, err = runtimes(repo.fs, repo.repoConfig)
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

// runtimes returns runtimes defined in this repository's filesystem.
// The views are denormalized, using the parent repository's values
// for inherited fields BuildConfig and HealthEndpoints as the default values
// for the runtimes and templates.  The runtimes and templates themselves can
// override these values by specifying new values in thir config files.
func runtimes(fs filesystem.Filesystem, repoCfg repoConfig) (runtimes []Runtime, err error) {
	// Validate templates path
	if err = checkDir(fs, repoCfg.TemplatesPath); err != nil {
		err = fmt.Errorf("templates path '%v' does not exist in repository. %v",
			repoCfg.TemplatesPath, err)
		return
	}

	// For each directory at the path, load it as a runtime
	fis, err := fs.ReadDir(repoCfg.TemplatesPath)
	if err != nil {
		return
	}
	for _, fi := range fis {
		if !fi.IsDir() || strings.HasPrefix(fi.Name(), ".") {
			continue // ignore files and hidden directories
		}

		if fi.Name() == "certs" {
			continue // ignore reserved word "certs"
		}

		runtime := Runtime{
			Name: fi.Name(),
		}

		// Load the runtimeConfig (manifest.yaml) with values from the
		// shared repoCfg as defaults.
		runtime.config, err = loadRuntimeConfig(fs, repoCfg, runtime.Name)
		if err != nil {
			return
		}

		runtime.Templates, err = templates(fs, repoCfg, runtime.config, runtime.Name)
		if err != nil {
			return
		}
		runtimes = append(runtimes, runtime)
	}
	return
}

// templates returns templates currently defined in the given runtime's
// filesystem.  The view is denormalized, using the inherited fields from the
// runtime for defaults of BuildConfig andHealthEndpoints.  The template itself
// can override these by including a manifest.
// The reserved word "scaffolding" is used for repository-defined scaffolding
// code and is not listed as a template.
func templates(fs filesystem.Filesystem, repoCfg repoConfig, runtimeCfg runtimeConfig, runtimeName string) (templates []Template, err error) {
	// Validate runtime path
	runtimePath := path.Join(repoCfg.TemplatesPath, runtimeName)
	if err = checkDir(fs, runtimePath); err != nil {
		err = fmt.Errorf("runtime path '%v' not found. %v", runtimePath, err)
		return
	}

	// Read the directory at the path, load it as a template
	fis, err := fs.ReadDir(runtimePath)
	if err != nil {
		return
	}
	for _, fi := range fis {
		if !fi.IsDir() || strings.HasPrefix(fi.Name(), ".") {
			continue // ignore files and hidden dirs
		}

		if fi.Name() == "scaffolding" {
			continue // ignore the reserved word "scaffolding"
		}

		t := template{
			name:       fi.Name(),
			repository: repoCfg.Name,
			runtime:    runtimeName,
			fs:         filesystem.NewSubFS(path.Join(runtimePath, fi.Name()), fs),
		}

		// update repoCfg with template's manifest.yaml valuse
		t.config, err = loadTemplateConfig(fs, repoCfg, runtimeCfg, runtimeName, t.name)
		if err != nil {
			return
		}

		templates = append(templates, t)
	}
	return
}

// repositoryName returns the given name, which if empty falls back to
// deriving a name from the URI.
func repositoryName(name, uri string) (string, error) {
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

// loadRepoConfig from the root of the repository's filesystem if it
// exists.  Returned is the repository with any values from the manifest
// set to those of the manifest.
func loadRepoConfig(fs filesystem.Filesystem, repoCfg repoConfig) (repoConfig, error) {
	file, err := fs.Open(manifestFile)
	if err != nil {
		if os.IsNotExist(err) {
			err = nil
		}
		return repoCfg, err
	}
	defer file.Close()

	if err = yaml.NewDecoder(file).Decode(&repoCfg); err != nil {
		return repoCfg, err
	}

	// Default TemplatesPath to CWD
	if repoCfg.TemplatesPath == "" {
		repoCfg.TemplatesPath = "."
	}

	return repoCfg, nil
}

// loadRuntimeConfig from the directory specified (runtime root).  Returned
// is the runtime with values from the manifest populated preferentially.  An
// error is not returned for a missing manifest file (the passed runtime is
// returned), but errors decoding the file are.
func loadRuntimeConfig(fs filesystem.Filesystem, repoCfg repoConfig, runtime string) (runtimeCfg runtimeConfig, err error) {
	// The runtimeConfig is defaulted to the values from the parent (repo)
	runtimeCfg = repoCfg.runtimeConfig // Defaults from the repoCfg

	// If there is a manifest.yaml at the repo level, it can overwrite
	file, err := fs.Open(path.Join(repoCfg.TemplatesPath, runtime, manifestFile))
	if err != nil {
		if os.IsNotExist(err) {
			err = nil
		}
		return // errors other than "Not found" are legitimate
	}
	defer file.Close()
	err = yaml.NewDecoder(file).Decode(&runtimeCfg)
	return
}

// loadTemplateConfig from the directory specified (template root).  Returned
// is the template with values from the manifest populated preferentailly.  An
// error is not returned for a missing manifest file (the passed template is
// returned), but errors decoding the file are.
func loadTemplateConfig(fs filesystem.Filesystem, repoCfg repoConfig, runtimeCfg runtimeConfig, runtimeName, templateName string) (tplCfg templateConfig, err error) {
	// The templateConfig is defaulted to the values from the parent (repo)
	tplCfg = runtimeCfg.templateConfig

	// If there is a manifest.yaml at the template level, it can overwrite
	file, err := fs.Open(path.Join(repoCfg.TemplatesPath, runtimeName, templateName, manifestFile))
	if err != nil {
		if os.IsNotExist(err) {
			err = nil
		}
		return
	}
	defer file.Close()
	err = yaml.NewDecoder(file).Decode(&tplCfg)
	return
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
