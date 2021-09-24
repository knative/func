package function

import (
	"errors"
	"io/ioutil"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-git/go-git/v5"
	"gopkg.in/yaml.v2"
)

// Repositories manager
type Repositories struct {
	Path string // Path to repositories
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
func (r *Repositories) All() (repos []Repository, err error) {
	repos = []Repository{}

	// Single repo override
	// TODO: Create single remote repository override for WithRepository option.

	// Default (builtin) repo always first
	builtin, err := NewRepositoryFromBuiltin()
	if err != nil {
		return
	}
	repos = append(repos, builtin)

	// Return if not using on-disk repos
	// If r.Path not populated, this indicates the client should
	// not read repositories from disk, using only builtin.
	if r.Path == "" {
		return
	}

	// Return empty if path does not exit
	// This will change to an error when the logic to determine config path,
	// and create its initial structure, is moved into the client library.
	// For now a missing repositores directory is considered equivalent to having
	// none installed.
	if _, err := os.Stat(r.Path); os.IsNotExist(err) {
		return repos, nil
	}

	// read repos from filesystem (sorted by name)
	// TODO: when manifests are introduced, the final name may be different
	// than the name on the filesystem, and as such we can not rely on the
	// alphanumeric ordering of underlying list, and will instead have to sort
	// by configured name.
	ff, err := ioutil.ReadDir(r.Path)
	if err != nil {
		return
	}
	for _, f := range ff {
		if !f.IsDir() || strings.HasPrefix(f.Name(), ".") {
			continue
		}
		var repo Repository
		repo, err = NewRepositoryFromPath(filepath.Join(r.Path, f.Name()))
		if err != nil {
			return
		}
		repos = append(repos, repo)
	}
	return repos, nil
}

// Get a repository by name, error if it does not exist.
func (r *Repositories) Get(name string) (repo Repository, err error) {
	if name == DefaultRepository {
		return NewRepositoryFromBuiltin()
	}
	// TODO: when WithRepository defined, only it can be defined
	return NewRepositoryFromPath(filepath.Join(r.Path, name))
}

// Add a repository of the given name from the URI.  Name, if not provided,
// defaults to the repo name (sans optional .git suffix)
func (r *Repositories) Add(name, uri string) (err error) {
	derivedName := name
	if derivedName == "" {
		derivedName, err = repoNameFrom(uri)
		if err != nil {
			return err
		}
	}
	path := filepath.Join(r.Path, derivedName)
	bare := false
	_, err = git.PlainClone(path, bare, &git.CloneOptions{URL: uri})
	if err != nil {
		return
	}
	// If the caller provided a name, set it
	if name != "" {
		err = writeRepositoryName(name, path)
	}
	return
}

// Rename a repository
func (r *Repositories) Rename(from, to string) error {
	a := filepath.Join(r.Path, from)
	b := filepath.Join(r.Path, to)
	if err := os.Rename(a, b); err != nil {
		return err
	}
	return writeRepositoryName(to, b)
}

// Remove a repository of the given name from the repositories.
// (removes its directory in Path)
func (r *Repositories) Remove(name string) error {
	if name == "" {
		return errors.New("name is required")
	}
	path := filepath.Join(r.Path, name)
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

// writeRepositoryName writes the given name to the repository's
// Name attribute in the manifest.yaml file
func writeRepositoryName(name, path string) (err error) {
	var r Repository
	if r, err = NewRepositoryFromPath(path); err != nil {
		return
	}
	r.Name = name

	// Now write the manifest file back to disk
	p := filepath.Join(path, ManifestYaml)
	var bb []byte
	if bb, err = yaml.Marshal(&r); err != nil {
		return
	}
	return ioutil.WriteFile(p, bb, 0644)
}
