package function

import (
	"io/ioutil"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-git/go-git/v5"
)

// Repositories management.
type Repositories struct {
	// Path to repositories
	Path string
}

// Add a repository of the given name from the URI.  Name, if not provided,
// defaults to the repo name (sans optional .git suffix)
func (r *Repositories) Add(name, uri string) (err error) {
	if name == "" {
		name, err = repoNameFrom(uri)
		if err != nil {
			return err
		}
	}
	path := filepath.Join(r.Path, name)
	bare := false
	_, err = git.PlainClone(path, bare, &git.CloneOptions{URL: uri})
	return err
}

// Rename a repository
func (r *Repositories) Rename(old string, new string) error {
	oldPath := filepath.Join(r.Path, old)
	newPath := filepath.Join(r.Path, new)
	return os.Rename(oldPath, newPath)
}

// Remove a repository of the given name from the repositories.
// (removes its directory in Path)
func (r *Repositories) Remove(name string) error {
	path := filepath.Join(r.Path, name)
	return os.RemoveAll(path)
}

// List repositories installed at the defined root path.
func (r *Repositories) List() (list []string, err error) {
	list = []string{}
	ff, err := ioutil.ReadDir(r.Path)
	if err != nil {
		return
	}
	for _, f := range ff {
		if f.IsDir() && !strings.HasPrefix(f.Name(), ".") {
			list = append(list, f.Name())
		}
	}
	return
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
