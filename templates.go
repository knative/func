package function

// Updating Templates:
// See documentation in ./templates/README.md
// go get github.com/markbates/pkger
//go:generate pkger

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/markbates/pkger"
)

// fileAccessor encapsulates methods for accessing template files.
type fileAccessor interface {
	Stat(name string) (os.FileInfo, error)
	Open(p string) (file, error)
}

type file interface {
	Readdir(int) ([]os.FileInfo, error)
	Read([]byte) (int, error)
	Close() error
}

// When pkger is run, code analysis detects this Include statement,
// triggering the serialization of the templates directory and all
// its contents into pkged.go, which is then made available via
// a pkger fileAccessor.
// Path is relative to the go module root.
func init() {
	_ = pkger.Include("/templates")
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
	templates string
	verbose   bool
}

var (
	ErrRepositoryNotFound        = errors.New("repository not found")
	ErrRepositoriesNotDefined    = errors.New("custom template repositories location not specified")
	ErrRuntimeNotFound           = errors.New("runtime not found")
	ErrTemplateNotFound          = errors.New("template not found")
	ErrTemplateMissingRepository = errors.New("template name missing repository prefix")
)

func (t templateWriter) Write(runtime, template, dest string) error {
	if template == "" {
		template = DefaultTemplate
	}

	if isCustom(template) {
		return writeCustom(t.templates, runtime, template, dest)
	}

	return writeEmbedded(runtime, template, dest)
}

func isCustom(template string) bool {
	return len(strings.Split(template, "/")) > 1
}

func writeCustom(templatesPath, runtime, templateFullName, dest string) error {
	if templatesPath == "" {
		return ErrRepositoriesNotDefined
	}

	if !repositoryExists(templatesPath, templateFullName) {
		return ErrRepositoryNotFound
	}

	// ensure that the templateFullName is of the format "repoName/templateName"
	cc := strings.Split(templateFullName, "/")
	if len(cc) != 2 {
		return ErrTemplateMissingRepository
	}
	repo := cc[0]
	template := cc[1]

	runtimePath := filepath.Join(templatesPath, repo, runtime)
	_, err := os.Stat(runtimePath)
	if err != nil {
		return ErrRuntimeNotFound
	}

	// Example FileSystem path:
	//   /home/alice/.config/func/templates/boson-experimental/go/json
	templatePath := filepath.Join(templatesPath, repo, runtime, template)
	_, err = os.Stat(templatePath)
	if err != nil {
		return ErrTemplateNotFound
	}
	return copy(templatePath, dest, filesystemAccessor{})
}

func writeEmbedded(runtime, template, dest string) (err error) {
	fmt.Println("copyEmbedded")
	// Copy files to the destination
	// Example embedded path:
	//   /templates/go/http
	runtimePath := filepath.Join("/templates", runtime)
	_, err = pkger.Stat(runtimePath)
	if err != nil {
		return ErrRuntimeNotFound
	}

	templatePath := filepath.Join("/templates", runtime, template)
	_, err = pkger.Stat(templatePath)
	if err != nil {
		return ErrTemplateNotFound
	}

	return copy(templatePath, dest, embeddedAccessor{})
}

type embeddedAccessor struct{}

func (a embeddedAccessor) Stat(path string) (os.FileInfo, error) {
	return pkger.Stat(path)
}

func (a embeddedAccessor) Open(path string) (file, error) {
	return pkger.Open(path)
}

type filesystemAccessor struct{}

func (a filesystemAccessor) Stat(path string) (os.FileInfo, error) {
	return os.Stat(path)
}

func (a filesystemAccessor) Open(path string) (file, error) {
	return os.Open(path)
}

func repositoryExists(repositories, template string) bool {
	cc := strings.Split(template, "/")
	_, err := os.Stat(filepath.Join(repositories, cc[0]))
	return err == nil
}

func copy(src, dest string, accessor fileAccessor) (err error) {
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

func copyNode(src, dest string, accessor fileAccessor) (err error) {
	node, err := accessor.Stat(src)
	if err != nil {
		return
	}

	err = os.MkdirAll(dest, node.Mode())
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

func readDir(src string, accessor fileAccessor) ([]os.FileInfo, error) {
	f, err := accessor.Open(src)
	if err != nil {
		return nil, err
	}
	list, err := f.Readdir(-1)
	f.Close()
	if err != nil {
		return nil, err
	}
	sort.Slice(list, func(i, j int) bool { return list[i].Name() < list[j].Name() })
	return list, nil
}

func copyLeaf(src, dest string, accessor fileAccessor) (err error) {
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
