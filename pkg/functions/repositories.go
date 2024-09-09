package functions

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	// DefaultRepositoryName is the name by which the currently default repo can
	// be referred.  This name is assumed when no template prefix is provided
	// when determining a template canonical (full) name.
	// Unless a single-repo override is defined, this is usually referring to the
	// builtin (embedded) repository.
	DefaultRepositoryName = "default"
)

// Repositories manager
type Repositories struct {
	// Optional path to extensible repositories on disk.  Blank indicates not
	// to use extensible
	path string

	// Optional uri of a single repo to use in leau of embedded and extensible.
	// Enables single-repository mode.  This replaces the default embedded repo
	// and extended repositories.  This is an important mode for both diskless
	// (config-less) operation, such as security-restrited environments, and for
	// running as a library in which case environmental settings should be
	// ignored in favor of a more functional approach in which only inputs affect
	// outputs.
	remote string

	// backreference to the client enabling this repositorires manager to
	// have full API access.
	client *Client
}

// newRepositories manager
// contains a backreference to the client (type tree root) for access to the
// full client API during implementations.
func newRepositories(client *Client) *Repositories {
	return &Repositories{
		client: client,
		path:   client.repositoriesPath,
		remote: client.repositoriesURI,
	}
}

// Path returns the currently active repositories path under management.
// Blank indicates that the system was not instantiated to use any
// repositories on disk.
func (r *Repositories) Path() string {
	return r.path
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

// All repositories under management
// The default repository is always first.
// If a path to custom repositories is defined, these are included next.
// If repositories is in single-repo mode, it will be the only repo returned.
func (r *Repositories) All() (repos []Repository, err error) {
	var repo Repository

	// if in single-repo mode:
	// Create a new repository from the remote URI, and set its name to
	// the default so that it is treated as the default in place of the embedded.
	if r.remote != "" {
		if repo, err = NewRepository(DefaultRepositoryName, r.remote); err != nil {
			return
		}
		repos = []Repository{repo}
		return
	}

	// When not in single-repo mode (above), the default repository is always
	// first in the list
	if repo, err = NewRepository("", ""); err != nil {
		return
	}
	repos = append(repos, repo)

	// Do not continue on to loading extended repositories unless path defined
	// and it exists.
	if r.path == "" {
		return
	}
	// Return empty if path does not exist or insufficient permissions
	if _, err = os.Stat(r.path); os.IsNotExist(err) || os.IsPermission(err) {
		return repos, nil
	}

	// Load each repo from disk.
	// All settings, including name, are derived from its structure on disk
	// plus manifest.
	ff, err := os.ReadDir(r.path)
	if err != nil {
		return
	}
	for _, f := range ff {
		if !f.IsDir() || strings.HasPrefix(f.Name(), ".") {
			continue
		}
		var abspath string
		abspath, err = filepath.Abs(r.path)
		if err != nil {
			return
		}
		if repo, err = NewRepository("", "file://"+filepath.ToSlash(abspath)+"/"+f.Name()); err != nil {
			return
		}
		repos = append(repos, repo)
	}
	return
}

// Get a repository by name, error if it does not exist.
func (r *Repositories) Get(name string) (repo Repository, err error) {
	all, err := r.All()
	if err != nil {
		return
	}
	if len(all) == 0 { // should not be possible because embedded always exists.
		err = errors.New("internal error: no repositories loaded")
		return
	}

	if name == DefaultRepositoryName {
		repo = all[0]
		return
	}

	if r.remote != "" {
		return repo, fmt.Errorf("in single-repo mode (%v). Repository '%v' not loaded", r.remote, name)
	}
	for _, v := range all {
		if v.Name == name {
			repo = v
			return
		}
	}
	return repo, ErrRepositoryNotFound
}

// Add a repository of the given name from the URI.  Name, if not provided,
// defaults to the repo name (sans optional .git suffix). Returns the final
// name as added.
func (r *Repositories) Add(name, uri string) (string, error) {
	if r.path == "" {
		return "", fmt.Errorf("repository %v(%v) not added. "+
			"No repositories path provided", name, uri)
	}

	// Create a repo (in-memory FS) from the URI
	repo, err := NewRepository(name, uri)
	if err != nil {
		return "", fmt.Errorf("failed to create new repository: %w", err)
	}

	// Error if the repository already exists on disk
	dest := filepath.Join(r.path, repo.Name)
	if _, err := os.Stat(dest); !os.IsNotExist(err) {
		return "", fmt.Errorf("repository '%v' already exists", repo.Name)
	}

	// Instruct the repository to write itself to disk at the given path.
	// Fails if path exists.
	err = repo.Write(dest)
	if err != nil {
		return "", fmt.Errorf("failed to write repository: %w", err)
	}
	return repo.Name, nil
}

// Rename a repository
func (r *Repositories) Rename(from, to string) error {
	if r.path == "" {
		return fmt.Errorf("repository %v not renamed. "+
			"No repositories path provided", from)
	}
	a := filepath.Join(r.path, from)
	b := filepath.Join(r.path, to)
	return os.Rename(a, b)
}

// Remove a repository of the given name from the repositories.
// (removes its directory in Path)
func (r *Repositories) Remove(name string) error {
	if r.path == "" {
		return fmt.Errorf("repository %v not removed. "+
			"No repositories path provided", name)
	}
	if name == "" {
		return errors.New("name is required")
	}
	path := filepath.Join(r.path, name)
	return os.RemoveAll(path)
}
