package function

import (
	"bytes"
	"compress/gzip"
	_ "embed"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/boson-project/func/tarfs"
)

// Generate templates.tgz
//go:generate go run generate.go

// Embed the templates tarball
//go:embed templates.tgz
var embedded []byte

// DefaultTemplate is the default Function signature / environmental context
// of the resultant template.  All runtimes are expected to have at least
// an HTTP Handler ("http") and Cloud Events ("events")
const DefaultTemplate = "http"

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
	templates    fs.FS
	verbose      bool
}

var (
	ErrRepositoryNotFound        = errors.New("repository not found")
	ErrRepositoriesNotDefined    = errors.New("custom template location not specified")
	ErrRuntimeNotFound           = errors.New("runtime not found")
	ErrTemplateNotFound          = errors.New("template not found")
	ErrTemplateMissingRepository = errors.New("template name missing repository prefix")
)

// Decompress into an in-memory FS which implements fs.ReadDirFS
func (t *templateWriter) load() (err error) {

	// Templates are stored as an embedded byte slice gzip encoded.
	zr, err := gzip.NewReader(bytes.NewReader(embedded))
	if err != nil {
		return
	}

	t.templates, err = tarfs.New(zr)
	return
}

// Write the template for the given runtime to the destination specified.
// Template may be prefixed with a custom repo name.
func (t *templateWriter) Write(runtime, template, dest string) (err error) {
	if t.templates == nil {
		// load static embedded templates into t.templates as an fs.FS
		if err = t.load(); err != nil {
			return
		}
	}
	if template == "" {
		template = DefaultTemplate
	}
	if isCustom(template) {
		return t.writeCustom(t.repositories, runtime, template, dest)
	}
	return t.writeEmbedded(runtime, template, dest)
}

func isCustom(template string) bool {
	return len(strings.Split(template, "/")) > 1
}

func (t *templateWriter) writeCustom(repositoriesPath, runtime, template, dest string) (err error) {
	if repositoriesPath == "" {
		return ErrRepositoriesNotDefined
	}
	if !repositoryExists(repositoriesPath, template) {
		return ErrRepositoryNotFound
	}
	cc := strings.Split(template, "/")
	if len(cc) < 2 {
		return ErrTemplateMissingRepository
	}
	repositoriesFS := os.DirFS(repositoriesPath)

	runtimePath := cc[0] + "/" + runtime
	_, err = fs.Stat(repositoriesFS, runtimePath)
	if errors.Is(err, fs.ErrNotExist) {
		return ErrRuntimeNotFound
	}

	templatePath := runtimePath + "/" + cc[1]
	_, err = fs.Stat(repositoriesFS, templatePath)
	if errors.Is(err, fs.ErrNotExist) {
		return ErrTemplateNotFound
	}

	// ex: /home/alice/.config/func/repositories/boson/go/http
	// Note that the FS instance returned by os.DirFS uses forward slashes
	// internally, so source paths do not use the os path separator due to
	// that breaking Windows.
	src := cc[0] + "/" + runtime + "/" + cc[1]
	return t.cp(src, dest, repositoriesFS)
}

func (t *templateWriter) writeEmbedded(runtime, template, dest string) error {
	runtimePath := "templates/" + runtime // embedded FS alwas uses '/'
	_, err := fs.Stat(t.templates, runtimePath)
	if errors.Is(err, fs.ErrNotExist) {
		return ErrRuntimeNotFound
	}

	templatePath := "templates/" + runtime + "/" + template // always '/' in embedded fs
	_, err = fs.Stat(t.templates, templatePath)
	if errors.Is(err, fs.ErrNotExist) {
		return ErrTemplateNotFound
	}

	return t.cp(templatePath, dest, t.templates)
}

func repositoryExists(repositories, template string) bool {
	cc := strings.Split(template, "/")
	_, err := fs.Stat(os.DirFS(repositories), cc[0])
	return err == nil
}

func (t *templateWriter) cp(src, dest string, files fs.FS) error {
	node, err := fs.Stat(files, src)
	if err != nil {
		return err
	}
	if node.IsDir() {
		return t.copyNode(src, dest, files)
	} else {
		return t.copyLeaf(src, dest, files)
	}
}

func (t *templateWriter) copyNode(src, dest string, files fs.FS) error {
	node, err := fs.Stat(files, src)
	if err != nil {
		return err
	}

	mode := node.Mode()

	err = os.MkdirAll(dest, mode)
	if err != nil {
		return err
	}

	children, err := readDir(src, files)
	if err != nil {
		return err
	}
	for _, child := range children {
		// NOTE: instances of fs.FS use forward slashes,
		// even on Windows.
		childSrc := src + "/" + child.Name()
		childDest := filepath.Join(dest, child.Name())
		if err = t.cp(childSrc, childDest, files); err != nil {
			return err
		}
	}
	return nil
}

func readDir(src string, files fs.FS) ([]fs.DirEntry, error) {
	f, err := files.Open(src)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	fi, err := f.Stat()
	if err != nil {
		return nil, err
	}

	if !fi.IsDir() {
		return nil, fmt.Errorf("%v must be a directory", fi.Name())
	}
	list, err := f.(fs.ReadDirFile).ReadDir(-1)
	if err != nil {
		return nil, err
	}

	sort.Slice(list, func(i, j int) bool { return list[i].Name() < list[j].Name() })
	return list, nil
}

func (t *templateWriter) copyLeaf(src, dest string, files fs.FS) (err error) {
	srcFile, err := files.Open(src)
	if err != nil {
		return
	}
	defer srcFile.Close()

	srcFileInfo, err := fs.Stat(files, src)
	if err != nil {
		return
	}

	// Use the original's mode unless a nonzero mode was explicitly provided.
	mode := srcFileInfo.Mode()

	destFile, err := os.OpenFile(dest, os.O_RDWR|os.O_CREATE|os.O_TRUNC, mode)
	if err != nil {
		return
	}
	defer destFile.Close()

	_, err = io.Copy(destFile, srcFile)
	return
}
