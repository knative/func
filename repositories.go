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

// Repositories manager
type Repositories struct {
	path   string // Path to repositories
	client *Client
}

// newRepositories manager
// contains a backreference to the client (type tree root) for access to the
// full client API during implementations.
func newRepositories(client *Client, path string) *Repositories {
	return &Repositories{client: client, path: path}
}

// SetPath to repositories under management.
func (r *Repositories) SetPath(path string) {
	r.path = path
}

// Path returns the currently active repositories path under management.
func (r *Repositories) Path() string {
	return r.path
}

// List all repositories installed at the defined root path plus builtin.
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

	// Single repo override
	// TODO: Create single remote repository override for WithRepository option.

	// Default (embedded) repo always first
	builtin, err := newEmbeddedRepository()
	if err != nil {
		return
	}
	repos = append(repos, builtin)

	// Return if not using on-disk repos
	// If r.path not populated, this indicates the client should
	// not read repositories from disk, using only builtin.
	if r.path == "" {
		return
	}

	// Return empty if path does not exit
	// This will change to an error when the logic to determine config path,
	// and create its initial structure, is moved into the client library.
	// For now a missing repositores directory is considered equivalent to having
	// none installed.
	if _, err := os.Stat(r.path); os.IsNotExist(err) {
		return repos, nil
	}

	// read repos from filesystem (sorted by name)
	// TODO: when manifests are introduced, the final name may be different
	// than the name on the filesystem, and as such we can not rely on the
	// alphanumeric ordering of underlying list, and will instead have to sort
	// by configured name.
	ff, err := ioutil.ReadDir(r.path)
	if err != nil {
		return
	}
	for _, f := range ff {
		if !f.IsDir() || strings.HasPrefix(f.Name(), ".") {
			continue
		}
		var repo Repository0_18
		repo, err = newRepository(filepath.Join(r.path, f.Name()))
		if err != nil {
			return
		}
		repos = append(repos, repo)
	}
	return repos, nil
}

// Get a repository by name, error if it does not exist.
func (r *Repositories) Get(name string) (repo Repository0_18, err error) {
	if name == DefaultRepository {
		return newEmbeddedRepository()
	}
	// TODO: when WithRepository defined, only it can be defined
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
