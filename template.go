package function

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"sort"

	"github.com/markbates/pkger"
)

// Template
type Template struct {
	// Name (short name) of this template within the repository.
	// See .Fullname for the calculated field wich is the unique primary id.
	Name string `yaml:"-"`
	// Runtime for which this template applies.
	Runtime string
	// Repository within which this template is contained.
	Repository string
	// BuildConfig defines builders and buildpacks.  the denormalized view of
	// members which can be defined per repo or per runtime first.
	BuildConfig `yaml:",inline"`
	// HealthEndpoints.  The denormalized view of members which can be defined
	// first per repo or per runtime.
	HealthEndpoints `yaml:"healthEndpoints,omitempty"`
}

// Fullname is a caluclated field of [repo]/[name] used
// to uniquely reference a template which may share a name
// with one in another repository.
func (t Template) Fullname() string {
	return t.Repository + "/" + t.Name
}

// write the given template to path using data from given repos.
func writeTemplate(t Template, repos, dest string) error {
	// Write the template from the right location
	// TODO The filesystem abstraction will be moved into the object itself such
	// that writing does not depend (at this level) on what _kind_ of template
	// it is (embedded, on disk or remote) and just writes based on its internal
	// filesystem (wherever that FS may have come from)
	if t.Repository == DefaultRepositoryName {
		return writeEmbedded(t, dest)
	} else {
		return writeCustom(t, repos, dest)
	}
}

var (
	ErrRepositoryNotFound        = errors.New("repository not found")
	ErrRepositoriesNotDefined    = errors.New("custom template repositories location not specified")
	ErrRuntimeNotFound           = errors.New("runtime not found")
	ErrTemplateNotFound          = errors.New("template not found")
	ErrTemplateMissingRepository = errors.New("template name missing repository prefix")
)

type filesystem interface {
	Stat(name string) (os.FileInfo, error)
	Open(path string) (file, error)
	ReadDir(path string) ([]os.FileInfo, error)
}

type file interface {
	io.Reader
	io.Closer
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

// Embedding Directives
// Trigger encoding of ./templates as pkged.go

// Path to embedded
// note: this constant must be defined in the file in which pkger is called,
// as it performs static analysis on each source file separately to trigger
// encoding of referenced paths.
const embeddedPath = "/templates"

// When pkger is run, code analysis detects this pkger.Include statement,
// triggering the serialization of the templates directory and all its contents
// into pkged.go, which is then made available via a pkger filesystem.  Path is
// relative to the go module root.
func init() {
	_ = pkger.Include(embeddedPath)
}
