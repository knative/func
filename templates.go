package function

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

// Updating Templates:
// See documentation in ./templates/README.md

// DefautlTemplate is the default Function signature / environmental context
// of the resultant template.  All runtimes are expected to have at least
// an HTTP Handler ("http") and Cloud Events ("events")
const DefaultTemplate = "http"

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
// triggering the serializaation of the templates directory and all
// its contents into pkged.go, which is then made available via
// a pkger fileAccessor.
// Path is relative to the go module root.
func init() {
	_ = pkger.Include("/templates")
}

type templateWriter struct {
	verbose   bool
	templates string
}

func (n templateWriter) Write(runtime, template string, dest string) error {
	if template == "" {
		template = DefaultTemplate
	}

	// TODO: Confirm the dest path is empty?  This is currently in an earlier
	// step of the create process but future calls directly to initialize would
	// be better off being made safe.

	if isEmbedded(runtime, template) {
		return copyEmbedded(runtime, template, dest)
	}
	if n.templates != "" {
		return copyFilesystem(n.templates, runtime, template, dest)
	}
	return fmt.Errorf("A template for runtime '%v' template '%v' was not found internally and no custom template path was defined.", runtime, template)
}

func copyEmbedded(runtime, template, dest string) error {
	// Copy files to the destination
	// Example embedded path:
	//   /templates/go/http
	src := filepath.Join("/templates", runtime, template)
	return copy(src, dest, embeddedAccessor{})
}

func copyFilesystem(templatesPath, runtime, templateFullName, dest string) error {
	// ensure that the templateFullName is of the format "repoName/templateName"
	cc := strings.Split(templateFullName, "/")
	if len(cc) != 2 {
		return errors.New("Template name must be in the format 'REPO/NAME'")
	}
	repo := cc[0]
	template := cc[1]

	// Example FileSystem path:
	//   /home/alice/.config/func/templates/boson-experimental/go/json
	src := filepath.Join(templatesPath, repo, runtime, template)
	return copy(src, dest, filesystemAccessor{})
}

func isEmbedded(runtime, template string) bool {
	_, err := pkger.Stat(filepath.Join("/templates", runtime, template))
	return err == nil
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
