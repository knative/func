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

	"github.com/go-git/go-billy/v5"
	"github.com/go-git/go-billy/v5/memfs"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/storage/memory"
	"github.com/markbates/pkger"
)

// Path to builtin
// note: this constant must be redefined in each file used due to pkger
// performing static analysis on each source file separately.
const builtinPath = "/templates"

// Templates Manager
type Templates struct {
	Repositories *Repositories // Repository Manager
}

// Template metadata
type Template struct {
	Runtime    string
	Repository string
	Name       string
}

// Fullname is a caluclate field of [repo]/[name] used
// to uniquely reference a template which may share a name
// with one in another repository.
func (t Template) Fullname() string {
	return t.Repository + "/" + t.Name
}

// List the full name of templates available for the runtime.
// Full name is the optional repository prefix plus the template's repository
// local name.  Default templates grouped first sans prefix.
func (t *Templates) List(runtime string) ([]string, error) {
	// TODO: if repository override was enabled, we should just return those, flat.
	builtin, err := t.ListDefault(runtime)
	if err != nil {
		return []string{}, err
	}

	extended, err := t.ListExtended(runtime)
	if err != nil {
		return []string{}, err
	}

	// Result is an alphanumerically sorted list first grouped by
	// embedded at head.
	return append(builtin, extended...), nil
}

// ListDefault (embedded) templates by runtime
func (t *Templates) ListDefault(runtime string) ([]string, error) {
	var (
		names     = newSortedSet()
		repo, err = t.Repositories.Get(DefaultRepository)
	)
	if err != nil {
		return []string{}, err
	}
	for _, template := range repo.Templates {
		if template.Runtime != runtime {
			continue
		}
		names.Add(template.Name)
	}
	return names.Items(), nil
}

// ListExtended templates returns all template full names that
// exist in all extended (config dir) repositories for a runtime.
// Prefixed, sorted.
func (t *Templates) ListExtended(runtime string) ([]string, error) {
	var (
		names      = newSortedSet()
		repos, err = t.Repositories.All()
	)
	if err != nil {
		return []string{}, err
	}
	for _, repo := range repos {
		if repo.Name == DefaultRepository {
			continue // already added at head of names
		}
		for _, template := range repo.Templates {
			if template.Runtime != runtime {
				continue
			}
			names.Add(template.Fullname())
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
	repo, err = t.Repositories.Get(repoName)
	if err != nil {
		return template, err
	}

	return repo.GetTemplate(runtime, tplName)
}

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

	// URL of a a specific network-available Git repository to use for
	// templates.  Takes precidence over both builtin and extensible
	// if defined.
	url string

	// enable verbose logging
	verbose bool
}

var (
	ErrRepositoryNotFound        = errors.New("repository not found")
	ErrRepositoriesNotDefined    = errors.New("custom template repositories location not specified")
	ErrRuntimeNotFound           = errors.New("runtime not found")
	ErrTemplateNotFound          = errors.New("template not found")
	ErrTemplateMissingRepository = errors.New("template name missing repository prefix")
)

func (t templateWriter) Write(runtime, template, dest string) error {
	if runtime == "" {
		runtime = DefaultRuntime
	}

	if template == "" {
		template = DefaultTemplate
	}

	// remote URLs, when provided, take precidence
	if t.url != "" {
		return writeRemote(t.url, runtime, template, dest)
	}

	// templates with repo prefix are on-disk "custom" (not embedded)
	if len(strings.Split(template, "/")) > 1 {
		return writeCustom(t.repositories, runtime, template, dest)
	}

	// default case is to write from the embedded set of core templates.
	return writeEmbedded(runtime, template, dest)
}

func writeRemote(url, runtime, template, dest string) error {
	// Clone a minimal copy of the remote repository in-memory.
	r, err := git.Clone(
		memory.NewStorage(),
		memfs.New(),
		&git.CloneOptions{
			URL:               url,
			Depth:             1,
			Tags:              git.NoTags,
			RecurseSubmodules: git.NoRecurseSubmodules,
		})
	if err != nil {
		return err
	}
	wt, err := r.Worktree()
	if err != nil {
		return err
	}
	fs := wt.Filesystem

	if _, err := fs.Stat(runtime); err != nil {
		return ErrRuntimeNotFound
	}

	templatePath := filepath.Join(runtime, template)

	if _, err := fs.Stat(templatePath); err != nil {
		return ErrTemplateNotFound
	}

	accessor := billyFilesystem{fs: fs}

	return copy(templatePath, dest, accessor)
}

// write from a custom repository.  The temlate full name is prefixed
func writeCustom(repositoriesPath, runtime, templateFullName, dest string) error {
	// assert path to template repos provided
	if repositoriesPath == "" {
		return ErrRepositoriesNotDefined
	}

	// assert template in form "repoName/templateName"
	cc := strings.Split(templateFullName, "/")
	if len(cc) != 2 {
		return ErrTemplateMissingRepository
	}

	var (
		repo         = cc[0]
		template     = cc[1]
		repoPath     = filepath.Join(repositoriesPath, repo)
		runtimePath  = filepath.Join(repositoriesPath, repo, runtime)
		templatePath = filepath.Join(repositoriesPath, repo, runtime, template)
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
	return copy(templatePath, dest, accessor)
}

func writeEmbedded(runtime, template, dest string) error {
	var (
		repoPath     = "/templates"
		runtimePath  = filepath.Join(repoPath, runtime)
		templatePath = filepath.Join(repoPath, runtime, template)
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

// billyFilesystem is a template file accessor backed by a billy FS
type billyFilesystem struct {
	fs billy.Filesystem
}

func (a billyFilesystem) Stat(path string) (os.FileInfo, error) {
	return a.fs.Stat(path)
}

func (a billyFilesystem) Open(path string) (file, error) {
	return a.fs.Open(path)
}

func (a billyFilesystem) ReadDir(path string) ([]os.FileInfo, error) {
	return a.fs.ReadDir(path)
}
