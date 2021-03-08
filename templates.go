package function

import (
	"embed"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Embed all files in ./templates.
//go:embed templates
var embedded embed.FS

// DefautlTemplate is the default Function signature / environmental context
// of the resultant template.  All runtimes are expected to have at least
// an HTTP Handler ("http") and Cloud Events ("events")
const DefaultTemplate = "http"

// DefaultTemplateFileMode for embedded files which have lost their mode due to being
// retained in a read-only filesystem (forced 0444 on embed).
const DefaultTemplateFileMode = 0644

// DefaultTemplateDirMode for embedded files which have lost their mode due to being
// retained in a read-only filesystem (forced 0444 on embed).
const DefaultTemplateDirMode = 0744

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
	defaultModes bool
	verbose      bool
}

var (
	ErrRepositoryNotFound        = errors.New("repository not found")
	ErrRepositoriesNotDefined    = errors.New("custom template repositories location not specified")
	ErrTemplateMissingRepository = errors.New("template name missing repository prefix")
)

// Write the template for the given runtime to the destination specified.
// Template may be prefixed with a custom repo name.
func (t *templateWriter) Write(runtime, template, dest string) error {
	if template == "" {
		template = DefaultTemplate
	}

	if isCustom(template) {
		return t.writeCustom(t.repositories, runtime, template, dest)
	}

	t.defaultModes = true // overwrite read-only mode on write to defaults.
	return t.writeEmbedded(runtime, template, dest)
}

func isCustom(template string) bool {
	return len(strings.Split(template, "/")) > 1
}

func (t *templateWriter) writeCustom(repositories, runtime, template, dest string) error {
	if repositories == "" {
		return ErrRepositoriesNotDefined
	}
	if !repositoryExists(repositories, template) {
		return ErrRepositoryNotFound
	}
	cc := strings.Split(template, "/")
	if len(cc) < 2 {
		return ErrTemplateMissingRepository
	}
	// ex: /home/alice/.config/func/repositories/boson/go/http
	src := filepath.Join(cc[0], runtime, cc[1])
	return t.cp(src, dest, os.DirFS(repositories))
}

func (t *templateWriter) writeEmbedded(runtime, template, dest string) error {
	_, err := fs.Stat(embedded, filepath.Join("templates", runtime, template))
	if err != nil {
		return err
	}
	src := filepath.Join("templates", runtime, template)
	return t.cp(src, dest, embedded)
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
	if t.defaultModes {
		mode = DefaultTemplateDirMode
	}

	err = os.MkdirAll(dest, mode)
	if err != nil {
		return err
	}

	children, err := readDir(src, files)
	if err != nil {
		return err
	}
	for _, child := range children {
		if err = t.cp(filepath.Join(src, child.Name()), filepath.Join(dest, child.Name()), files); err != nil {
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
		return nil, errors.New(fmt.Sprintf("%v must be a directory", fi.Name()))
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
	if t.defaultModes {
		mode = DefaultTemplateFileMode
	}

	destFile, err := os.OpenFile(dest, os.O_RDWR|os.O_CREATE|os.O_TRUNC, mode)
	if err != nil {
		return
	}
	defer destFile.Close()

	_, err = io.Copy(destFile, srcFile)
	return
}
