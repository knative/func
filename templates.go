package function

// Updating Templates:
// See documentation in ./templates/README.md
// go get github.com/markbates/pkger
//go:generate pkger

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/markbates/pkger"
)

// Path to builtin
// note: this constant must be redefined in each file used due to pkger
// performing static analysis on each source file separately.
const builtinPath = "/templates"

// Templates Manager
type Templates struct {
	client *Client
}

// newTemplates manager
// Includes a back-reference to client (logic tree root) such
// that the templates manager has full access to the API for
// use in its implementations.
func newTemplates(client *Client) *Templates {
	return &Templates{client: client}
}

// List the full name of templates available for the runtime.
// Full name is the optional repository prefix plus the template's repository
// local name.  Default templates grouped first sans prefix.
func (t *Templates) List(runtime string) ([]string, error) {
	// TODO: if repository override was enabled, we should just return those, flat.
	builtin, err := t.listDefault(runtime)
	if err != nil {
		return []string{}, err
	}

	extended, err := t.ListExtended(runtime)
	if err != nil && err != ErrTemplateNotFound {
		return []string{}, err
	}

	// Result is an alphanumerically sorted list first grouped by
	// embedded at head.
	return append(builtin, extended...), nil
}

// listDefault (embedded) templates by runtime
func (t *Templates) listDefault(runtime string) ([]string, error) {
	var (
		names     = newSortedSet()
		repo, err = t.client.Repositories().Get(DefaultRepository)
		templates FunctionTemplates
	)
	if err != nil {
		return []string{}, err
	}

	if templates, err = repo.Templates(runtime); err != nil {
		return []string{}, err
	}
	for _, t := range templates {
		names.Add(t.Name)
	}
	return names.Items(), nil
}

// listExtended templates returns all template full names that
// exist in all extended (config dir) repositories for a runtime.
// Prefixed, sorted.
func (t *Templates) listExtended(runtime string) ([]string, error) {
	var (
		names      = newSortedSet()
		repos, err = t.client.Repositories().All()
		templates  FunctionTemplates
	)
	if err != nil {
		return []string{}, err
	}
	for _, repo := range repos {
		if repo.Name == DefaultRepository {
			continue // already added at head of names
		}
		if templates, err = repo.Templates(runtime); err != nil {
			return []string{}, err
		}
		for _, template := range templates {
			names.Add(Template{
				Name:       template.Path,
				Repository: repo.Name,
				Runtime:    runtime,
			}.Fullname())
		}
	}
	return names.Items(), nil
}

// Template returns the named template in full form '[repo]/[name]' for the
// specified runtime.
// Templates from the default repository do not require the repo name prefix,
// though it can be provided.
func (t *Templates) Get(runtime, fullname string) (Template, error) {
	var (
		template Template
		repoName string
		tplName  string
		repo     Repository
		err      error
	)

	// Split into repo and template names.
	// Defaults when unprefixed to DefaultRepository
	cc := strings.Split(fullname, "/")
	if len(cc) == 1 {
		repoName = DefaultRepository
		tplName = fullname
	} else {
		repoName = cc[0]
		tplName = cc[1]
	}

	// Get specified repository
	repo, err = t.client.Repositories().Get(repoName)
	if err != nil {
		return template, err
	}

	return repo.Template(runtime, tplName)
}

// Write a template to disk at the given location
func (t *Templates) Write(template Template, dest string) error {
	// TODO: These defaults are in the wrong place and will move:
	if template.Name == "" {
		template.Name = DefaultTemplate
	}
	if template.Runtime == "" {
		template.Runtime = DefaultRuntime
	}
	if template.Repository == "" {
		template.Repository = DefaultRepository
	}

	// Write the template from the right location
	// This abstraction will be moved into the object itself such that
	// writing does not depend (at this level) on what _kind_ of template
	// it is (embedded, on disk or remote) and just writes based on its internal
	// filesystem (wherever that FS may have come from)
	if template.Repository == DefaultRepository {
		return writeEmbedded(template, dest)
	}
	return writeCustom(template, t.client.Repositories().Path(), dest)
}

var (
	ErrRepositoryNotFound        = errors.New("repository not found")
	ErrRepositoriesNotDefined    = errors.New("custom template repositories location not specified")
	ErrRuntimeNotFound           = errors.New("runtime not found")
	ErrTemplateNotFound          = errors.New("template not found")
	ErrTemplateMissingRepository = errors.New("template name missing repository prefix")
)

// Writing ------

type filesystem interface {
	Stat(name string) (os.FileInfo, error)
	Open(path string) (file, error)
	ReadDir(path string) ([]os.FileInfo, error)
}

type file interface {
	io.Reader
	io.Closer
}

// Trigger encoding of ./templates as pkged.go
//
// When pkger is run, code analysis detects this pkger.Include statement,
// triggering the serialization of the templates directory and all its contents
// into pkged.go, which is then made available via a pkger filesystem.  Path is
// relative to the go module root.
func init() {
	_ = pkger.Include(builtinPath)
}

type templateWriter struct {
	// Extensible Template Repositories
	// templates on disk (extensible templates)
	// Stored on disk at path:
	//   [customTemplatesPath]/[repository]/[runtime]/[template]
	// For example
	//   ~/.config/func/boson/go/http"
	// Specified when writing templates as simply:
	//   Write([runtime], [repository], [path])
	// For example
	// w := templateWriter{templates:"/home/username/.config/func/templates")
	//   w.Write("go", "boson/http")
	// Ie. "Using the custom templates in the func configuration directory,
	//    write the Boson HTTP template for the Go runtime."
	repositories string
	// enable verbose logging
	verbose bool
}

// write from a custom repository.  The temlate full name is prefixed
func writeCustom(t Template, from, to string) error {
	// assert path to template repos provided
	if from == "" {
		return ErrRepositoriesNotDefined
	}

	var (
		repoPath     = filepath.Join(from, t.Repository)
		runtimePath  = filepath.Join(from, t.Repository, t.Runtime)
		templatePath = filepath.Join(from, t.Repository, t.Runtime, t.Name)
		accessor     = osFilesystem{} // in instanced provider of Stat and Open
	)
	if _, err := accessor.Stat(repoPath); err != nil {
		return ErrRepositoryNotFound
	}
	if _, err := accessor.Stat(runtimePath); err != nil {
		return ErrRuntimeNotFound
	}
	if _, err := accessor.Stat(templatePath); err != nil {
		return ErrTemplateNotFound
	}
	return copy(templatePath, to, accessor)
}

func writeEmbedded(t Template, dest string) error {
	var (
		repoPath     = "/templates"
		runtimePath  = filepath.Join(repoPath, t.Runtime)
		templatePath = filepath.Join(repoPath, t.Runtime, t.Name)
		accessor     = pkgerFilesystem{} // instanced provder of Stat and Open
	)
	if _, err := accessor.Stat(runtimePath); err != nil {
		return ErrRuntimeNotFound
	}
	if _, err := accessor.Stat(templatePath); err != nil {
		return ErrTemplateNotFound
	}
	return copy(templatePath, dest, accessor)
}

func copy(src, dest string, accessor filesystem) (err error) {
	node, err := accessor.Stat(src)
	if err != nil {
		return
	}
	if node.IsDir() {
		return copyNode(src, dest, accessor)
	} else {
		return copyLeaf(src, dest, accessor)
	}
}

func copyNode(src, dest string, accessor filesystem) (err error) {
	// Ideally we should use the file mode of the src node
	// but it seems the git module is reporting directories
	// as 0644 instead of 0755. For now, just do it this way.
	// See https://github.com/go-git/go-git/issues/364
	// Upon resolution, return accessor.Stat(src).Mode()
	err = os.MkdirAll(dest, 0755)
	if err != nil {
		return
	}

	children, err := readDir(src, accessor)
	if err != nil {
		return
	}
	for _, child := range children {
		if err = copy(filepath.Join(src, child.Name()), filepath.Join(dest, child.Name()), accessor); err != nil {
			return
		}
	}
	return
}

func readDir(src string, accessor filesystem) ([]os.FileInfo, error) {
	list, err := accessor.ReadDir(src)
	if err != nil {
		return nil, err
	}
	sort.Slice(list, func(i, j int) bool { return list[i].Name() < list[j].Name() })
	return list, nil
}

func copyLeaf(src, dest string, accessor filesystem) (err error) {
	srcFile, err := accessor.Open(src)
	if err != nil {
		return
	}
	defer srcFile.Close()

	srcFileInfo, err := accessor.Stat(src)
	if err != nil {
		return
	}

	destFile, err := os.OpenFile(dest, os.O_RDWR|os.O_CREATE|os.O_TRUNC, srcFileInfo.Mode())
	if err != nil {
		return
	}
	defer destFile.Close()

	_, err = io.Copy(destFile, srcFile)
	return
}

// Filesystems
// Wrap the implementations of FS with their subtle differences into the
// common interface for accessing template files defined herein.
// os:    standard for on-disk extensible template repositories.
// pker:  embedded filesystem backed by the generated pkged.go.
// billy: go-git library's filesystem used for remote git template repos.

// osFilesystem is a template file accessor backed by an os
// filesystem.
type osFilesystem struct{}

func (a osFilesystem) Stat(path string) (os.FileInfo, error) {
	return os.Stat(path)
}

func (a osFilesystem) Open(path string) (file, error) {
	return os.Open(path)
}

func (a osFilesystem) ReadDir(path string) ([]os.FileInfo, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return f.Readdir(-1)
}

// pkgerFilesystem is template file accessor backed by the pkger-provided
// embedded filesystem.
type pkgerFilesystem struct{}

func (a pkgerFilesystem) Stat(path string) (os.FileInfo, error) {
	return pkger.Stat(path)
}

func (a pkgerFilesystem) Open(path string) (file, error) {
	return pkger.Open(path)
}

func (a pkgerFilesystem) ReadDir(path string) ([]os.FileInfo, error) {
	f, err := pkger.Open(path)
	if err != nil {
		return nil, err
	}
	return f.Readdir(-1)
}
