package function

import (
	"crypto/rand"
	"errors"
	"fmt"
	"io/ioutil"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-git/go-git/v5"
)

const (
	// DefaultRepositoryName is the name by which the currently default repo can
	// be referred.  This name is assumed when no template prefix is provided
	// when determining a template canonical (full) name.
	// Unless a single-repo override is defined, this is usally referring to the
	// builtin (embedded) repository.
	DefaultRepositoryName = "default"

	// DefaultRepositoriesPath is the default location for repositories under
	// management on local disk.
	// TODO: the logic which defaults this to ~/.config/func/repositories will
	// be moved from the CLI to the core in the near future.  For now use the
	// current working directory.
	DefaultRepositoriesPath = ""
)

// Repositories manager
type Repositories struct {
	// Path to repositories
	// (optional) is the path to extensible repositories on disk.
	path string

	// Single Repo Mode
	// (optional) the URI to a single repository for single-repo mode
	single string // URI of a single-repo override mode

	// backreference to the client enabling full api access for the repo manager
	client *Client
}

// newRepositories manager
// contains a backreference to the client (type tree root) for access to the
// full client API during implementations.
func newRepositories(client *Client) *Repositories {
	return &Repositories{
		path:   DefaultRepositoriesPath,
		client: client,
	}
}

// SetPath to repositories under management.
func (r *Repositories) SetPath(path string) {
	r.path = path
}

// Path returns the currently active repositories path under management.
func (r *Repositories) Path() string {
	return r.path
}

// SetURI enables single-reposotory mode.
// Enables single-repository mode.  This replaces the default embedded repo
// and extended repositories.  This is an important mode for both diskless
// (config-less) operation, such as security-restrited environments, and for
// running as a library in which case environmental settings should be
// ignored in favor of a more funcitonal approach in which only inputs affect
// outputs.
func (r *Repositories) SetSingle(uri string) {
	r.single = uri
}

// List all repositories the current configuration of the repo manager has
// defined.
func (r *Repositories) List() ([]string, error) {
	repositories, err := r.All()
	if err != nil {
		return []string{}, err
	}

	names := []string{}
	for _, repo := range repositories {
		names = append(names, repo.Name)
	}
	return names, nil
}

// All repositories under management (at configured Path)
func (r *Repositories) All() (repos []Repository0_18, err error) {
	repos = []Repository0_18{}

	// The default repository is always first in the list
	d, err := r.newDefault()
	if err != nil {
		return
	}
	repos = append(repos, d)

	// If running in "single repo" mode, or there is no path defined where to
	// find additional repositories, our job is done.
	if r.singleMode() || r.path == "" {
		return
	}

	// Loads the extended repositories from the defined path.
	// Note that an empty path results in an empty set.
	// (empty path indicates to not use extended repos)
	extended, err := newExtendedRepositories(r.path)
	if err != nil {
		return
	}
	repos = append(repos, extended...)
	return
}

// Return all repositories defined at path
// An empty path always returns an empty list.
func newExtendedRepositories(path string) (repos []Repository0_18, err error) {
	repos = []Repository0_18{}

	// TODO This will change to an _error_ when the logic to determine config path,
	// and create its initial structure, is moved into the client library.
	// For now a missing repositores directory is considered equivalent to having
	// none installed to minimize the blast-radious of that forthcoming change.
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return repos, nil
	}

	// Read repos from filesystem (sorted by name)
	ff, err := ioutil.ReadDir(path)
	if err != nil {
		return
	}
	for _, f := range ff {
		if !f.IsDir() || strings.HasPrefix(f.Name(), ".") {
			continue
		}
		var repo Repository0_18
		repo, err = newRepository(filepath.Join(path, f.Name()))
		if err != nil {
			return
		}
		repos = append(repos, repo)
	}
	return
}

// newDefault returns the default repository which is the embedded (builtin)
// repo or (if defined) the single remote repository specified for single-repo
// mode.
func (r *Repositories) newDefault() (Repository0_18, error) {
	if r.single != "" {
		return newRemoteRepository(r.single)
	}
	return newEmbeddedRepository()
}

// singleMode returns whether or not we are running in single-repo mode.
func (r *Repositories) singleMode() bool {
	return r.single != ""
}

// Get a repository by name, error if it does not exist.
func (r *Repositories) Get(name string) (repo Repository0_18, err error) {
	if name == DefaultRepositoryName {
		return r.newDefault()
	}

	if r.singleMode() {
		return repo, fmt.Errorf("repository '%v' will not be loaded because we "+
			"are running in single-repo mode (%v). This is the default (and only) "+
			"repo loaded.  It can be retrived by name '%v'.",
			name, r.single, DefaultRepositoryName)
	}

	return newRepository(filepath.Join(r.path, name))
}

// Add a repository of the given name from the URI.  Name, if not provided,
// defaults to the repo name (sans optional .git suffix). Returns the final
// name as added.
func (r *Repositories) Add(name, uri string) (string, error) {
	// TODO: this function will fail if there already exists a repository of the
	// _repo_ name due to a filesystem collision.  We use a temporary GUID here,
	// but this is messy as it can 1) leave files on the system in the event
	// of a proces interruption and 2) muddies up any other instance of the
	// library in another process as they will contain a temporary guid in their
	// api usage and 3) requires an explicit rename after the final is deployed.
	// etc.  These could be eliminated with an in-memory initial clone.

	// Clone the remote to local disk
	id, err := uuid()
	if err != nil {
		return name, fmt.Errorf("error generating local id for new repo. %v", err)
	}
	_, err = git.PlainClone(
		filepath.Join(r.path, id),   // path
		false,                       // bare (we want a working branch)
		&git.CloneOptions{URL: uri}) // no other options except URL

	// If the user specified a name, use that preferentially
	if name != "" {
		if err := r.Rename(id, name); err != nil {
			_ = r.Remove(id)
			return name, err
		}
		return name, nil
	}

	// Use the default name defined in the manifest, if provided
	repo, err := r.Get(id)
	if err != nil {
		return name, err
	}
	if repo.DefaultName != "" {
		if err := r.Rename(id, repo.DefaultName); err != nil {
			_ = r.Remove(id)
			return repo.DefaultName, err
		}
		return repo.DefaultName, nil
	}

	// Use the repo name from the URI as the base default case.
	repoName, err := repoNameFrom(uri)
	if err != nil {
		return name, err
	}
	if err := r.Rename(id, repoName); err != nil {
		_ = r.Remove(id)
		return repoName, err
	}
	return repoName, nil
}

// Rename a repository
func (r *Repositories) Rename(from, to string) error {
	a := filepath.Join(r.path, from)
	b := filepath.Join(r.path, to)
	return os.Rename(a, b)
}

// Remove a repository of the given name from the repositories.
// (removes its directory in Path)
func (r *Repositories) Remove(name string) error {
	if name == "" {
		return errors.New("name is required")
	}
	path := filepath.Join(r.path, name)
	return os.RemoveAll(path)
}

// repoNameFrom uri returns the last token with any .git suffix trimmed.
// uri must be parseable as a net/URL
func repoNameFrom(uri string) (name string, err error) {
	url, err := url.Parse(uri)
	if err != nil {
		return "", err
	}

	ss := strings.Split(url.Path, "/")
	if len(ss) == 0 {
		return
	}
	return strings.TrimSuffix(ss[len(ss)-1], ".git"), nil
}

func uuid() (string, error) {
	b := make([]byte, 6)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return fmt.Sprintf("%x-%x-%x", b[0:2], b[2:4], b[4:6]), nil
}
