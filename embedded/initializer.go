package embedded

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
// See documentation in faas/templates for instructions on including template
// updates in the binary for access by pkger.

// DefautlContext is the default function signature / environmental context
// of the resultant template.  All languages are expected to have at least
// an HTTP Handler ("http") and Cloud Events ("events")
const DefaultContext = "events"

// FileAccessor encapsulates methods for accessing template files.
type FileAccessor interface {
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
// a pkger FileAccessor.
// Path is relative to the go module root.
func init() {
	pkger.Include("/templates")
}

type Initializer struct {
	Verbose   bool
	templates string
}

// NewInitializer with an optional path to extended repositories directory
// (by default only internally embedded templates are used)
func NewInitializer(templates string) *Initializer {
	return &Initializer{templates: templates}
}

func (n *Initializer) Initialize(language, context string, dest string) error {
	if context == "" {
		context = DefaultContext
	}

	// TODO: Confirm the dest path is empty?  This is currently in an earlier
	// step of the create process but future calls directly to initialize would
	// be better off being made safe.

	if isEmbedded(language, context) {
		return copyEmbedded(language, context, dest)
	}
	if n.templates != "" {
		return copyFilesystem(n.templates, language, context, dest)
	}
	return errors.New(fmt.Sprintf("A template for language '%v' context '%v' was not found internally and no extended repository path was defined.", language, context))
}

func copyEmbedded(language, context, dest string) error {
	// Copy files to the destination
	// Example embedded path:
	//   /templates/go/http
	src := filepath.Join("/templates", language, context)
	return copy(src, dest, embeddedAccessor{})
}

func copyFilesystem(templatesPath, language, contextFullName, dest string) error {
	// ensure that the contextFullName is of the format "repoName/contextName"
	cc := strings.Split(contextFullName, "/")
	if len(cc) != 2 {
		return errors.New("Context name must be in the format 'REPO/NAME'")
	}
	repo := cc[0]
	context := cc[1]

	// Example FileSystem path:
	//   /home/alice/.config/faas/templates/boson-experimental/go/json
	src := filepath.Join(templatesPath, repo, language, context)
	return copy(src, dest, filesystemAccessor{})
}

func isEmbedded(language, context string) bool {
	_, err := pkger.Stat(filepath.Join("/templates", language, context))
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

func copy(src, dest string, accessor FileAccessor) (err error) {
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

func copyNode(src, dest string, accessor FileAccessor) (err error) {
	node, err := accessor.Stat(src)
	if err != nil {
		return
	}

	err = os.MkdirAll(dest, node.Mode())
	if err != nil {
		return
	}

	children, err := ReadDir(src, accessor)
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

func ReadDir(src string, accessor FileAccessor) ([]os.FileInfo, error) {
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

func copyLeaf(src, dest string, accessor FileAccessor) (err error) {
	srcFile, err := accessor.Open(src)
	if err != nil {
		return
	}
	defer srcFile.Close()

	destFile, err := os.Create(dest)
	if err != nil {
		return
	}
	defer destFile.Close()

	_, err = io.Copy(destFile, srcFile)
	return
}
