package function

import (
	"errors"
	"fmt"
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
	// Optional path to extensible repositories on disk.  Blank indicates not
	// to use extensible
	path string

	// Optional uri of a single repo to use in leau of embedded and extensible.
	single string

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

// SetSingle enables single-reposotory mode.
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

// All repositories under management
// The default repository is always first.
// If a path to custom repositories is defined, these are included next.
// If repositories is in single-repo mode, it will be the only repo returned.
func (r *Repositories) All() (repos []Repository0_18, err error) {
	repo := Repository0_18{}
	repos = []Repository0_18{}

	// if in single-repo mode:
	if r.single != "" {
		if repo, err = NewRepository(r.single); err != nil {
			return
		}
		repos = []Repository0_18{repo}
		return
	}

	// the default repository is always first in the list
	if repo, err = NewRepository(DefaultRepositoryName); err != nil {
		return
	}
	repos = append(repos, repo)

	// Do not continue on to loading extended repositories unless path defined
	// and it exists.
	if r.path == "" {
		return
	}
	if _, err = os.Stat(r.path); err != nil {
		return
	}

	// Load each repo
	ff, err := os.ReadDir(r.path)
	if err != nil {
		return
	}
	for _, f := range ff {
		if !f.IsDir() || strings.HasPrefix(f.Name(), ".") {
			continue
		}
		if repo, err = NewRepository("file://" + r.path + "/" + f.Name()); err != nil {
			return
		}
		repos = append(repos, repo)
	}
	return
}

// Get a repository by name, error if it does not exist.
func (r *Repositories) Get(name string) (repo Repository0_18, err error) {
	all, err := r.All()
	if err != nil {
		return
	}
	if len(all) == 0 { // should not be possible because embedded always exists.
		err = errors.New("internal error: no repositories loaded.")
		return
	}

	if name == DefaultRepositoryName {
		repo = all[0]
		return
	}

	if r.single != "" {
		return repo, fmt.Errorf("repository '%v' will not be loaded because we "+
			"are running in single-repo mode (%v). This is the default (and only) "+
			"repo loaded.  It can be retrived by name '%v'.",
			name, r.single, DefaultRepositoryName)
	}
	for _, v := range all {
		if v.Name == name {
			repo = v
		}
		return
	}
	return repo, ErrRepositoryNotFound
}

// Add a repository of the given name from the URI.  Name, if not provided,
// defaults to the repo name (sans optional .git suffix). Returns the final
// name as added.
func (r *Repositories) Add(name, uri string) (string, error) {
	if r.path == "" {
		return "", fmt.Errorf("repository %v(%v) will not be added because "+
			"no repositories path was specified.", name, uri)
	}

	// if name was not provided, pull the repo into memory which determines the
	// default name by first checking the manifest and falling back to extracting
	// the name from the uri.
	if name == "" {
		repo, err := NewRepository(uri)
		if err != nil {
			return "", err
		}
		name = repo.Name
	}

	// Clone it to disk
	_, err := git.PlainClone(
		filepath.Join(r.path, name), // path
		false,                       // not bare (we want a working branch)
		&git.CloneOptions{URL: uri}) // no other options except uri

	return name, err
}

// Rename a repository
func (r *Repositories) Rename(from, to string) error {
	if r.path == "" {
		return fmt.Errorf("repository %v will not be renamed because "+
			"no repositories path was specified.", from)
	}
	a := filepath.Join(r.path, from)
	b := filepath.Join(r.path, to)
	return os.Rename(a, b)
}

// Remove a repository of the given name from the repositories.
// (removes its directory in Path)
func (r *Repositories) Remove(name string) error {
	if r.path == "" {
		return fmt.Errorf("repository %v will not be removed because "+
			"no repositories path was specified.", name)
	}
	if name == "" {
		return errors.New("name is required")
	}
	path := filepath.Join(r.path, name)
	return os.RemoveAll(path)
}
