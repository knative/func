package function

import (
	"errors"
	"io/ioutil"
	"path/filepath"
	"strings"

	"github.com/markbates/pkger"
)

// Path to builtin repositories.
// note: this constant must be defined in the same file in which it is used due
// to pkger performing static analysis on source files separately.
const builtinRepositories = "/templates"

// Repository
type Repository struct {
	Name      string
	Templates []Template
	Runtimes  []string
}

// NewRepository from path.
// Represents the file structure of 'path' at time of construction as
// a Repository with Templates, each of which has a Name and its Runtime.
// a convenience member of Runtimes is the unique, sorted list of all
// runtimes
func NewRepositoryFromPath(path string) (Repository, error) {
	// TODO: read and use manifest if it exists

	r := Repository{
		Name:      filepath.Base(path),
		Templates: []Template{},
		Runtimes:  []string{}}

	// Each subdirectory is a Runtime
	runtimes, err := ioutil.ReadDir(path)
	if err != nil {
		return r, err
	}
	for _, runtime := range runtimes {
		if !runtime.IsDir() || strings.HasPrefix(runtime.Name(), ".") {
			continue // ignore files and hidden
		}
		r.Runtimes = append(r.Runtimes, runtime.Name())

		// Each subdirectory is a Template
		templates, err := ioutil.ReadDir(filepath.Join(path, runtime.Name()))
		if err != nil {
			return r, err
		}
		for _, template := range templates {
			if !template.IsDir() || strings.HasPrefix(template.Name(), ".") {
				continue // ignore files and hidden
			}
			r.Templates = append(r.Templates, Template{
				Runtime:    runtime.Name(),
				Repository: r.Name,
				Name:       template.Name()})
		}
	}
	return r, nil
}

// NewRepository from builtin (encoded ./templates)
func NewRepositoryFromBuiltin() (Repository, error) {
	r := Repository{
		Name:      DefaultRepository,
		Templates: []Template{},
		Runtimes:  []string{}}

	// Read in runtimes
	dir, err := pkger.Open(builtinRepositories)
	if err != nil {
		return r, err
	}
	runtimes, err := dir.Readdir(-1)
	if err != nil {
		return r, err
	}
	for _, runtime := range runtimes {
		if !runtime.IsDir() || strings.HasPrefix(runtime.Name(), ".") {
			continue // ignore from runtimes non-directory or hidden items
		}
		r.Runtimes = append(r.Runtimes, runtime.Name())

		// Each subdirectory is a Template
		templateDir, err := pkger.Open(filepath.Join(builtinRepositories, runtime.Name()))
		if err != nil {
			return r, err
		}
		templates, err := templateDir.Readdir(-1)
		if err != nil {
			return r, err
		}
		for _, template := range templates {
			if !template.IsDir() || strings.HasPrefix(template.Name(), ".") {
				continue // ignore from templates non-directory or hidden items
			}
			r.Templates = append(r.Templates, Template{
				Runtime:    runtime.Name(),
				Repository: r.Name,
				Name:       template.Name(),
			})

		}
	}
	return r, nil
}

// GetTemplate from repo with given runtime
func (r *Repository) GetTemplate(runtime, name string) (Template, error) {
	// TODO: return a typed RuntimeNotFound in repo X
	// rather than the generic Template Not Found
	for _, t := range r.Templates {
		if t.Runtime == runtime && t.Name == name {
			return t, nil
		}
	}
	// TODO: Typed TemplateNotFound in repo X
	return Template{}, errors.New("template not found")
}
