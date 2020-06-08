package embedded

import (
	"io"
	"os"
	"path/filepath"
	"sort"

	"github.com/markbates/pkger"
)

// DefautlContext is the default function signature / environmental context
// of the resultant template.  All languages are expected to have at least
// an HTTP Handler ("http") and Cloud Events ("events")
const DefaultContext = "events"

// Until such time as embedding static assets in binaries is included in the
// base go build funcitonality (see https://github.com/golang/go/issues/35950)
// a third-party tool is used and invoked via go-generate such that this package
// can still be installed via go install.
// 1) install pkger
//    go install github.com/markbates/pkger/cmd/pkger
// 2) Generate pkged.go (from module root):
//    pkger -o embedded
// 3)

var defaultContexts = map[string]string{
	"go": "events",
}

func init() {
	pkger.Include("/templates")
}

type Initializer struct {
	Verbose bool
}

func NewInitializer() *Initializer {
	return &Initializer{}
}

func (n *Initializer) Initialize(language, context string, path string) error {
	if context == "" {
		context = DefaultContext
	}

	// TODO: Confirm the path is empty?  This is currently handled by an earlier
	// step of the create process but future calls directly to initialize would
	// be better off being made safe.

	// TODO: since these are included in the binary statically, should we use os.PathSeparator?
	return copy("/templates/"+language+"/"+context, path)
}

func copy(src, dest string) (err error) {
	node, err := pkger.Stat(src)
	if err != nil {
		return
	}
	if node.IsDir() {
		return copyNode(src, dest)
	} else {
		return copyLeaf(src, dest)
	}
}

func copyNode(src, dest string) (err error) {
	node, err := pkger.Stat(src)
	if err != nil {
		return
	}

	err = os.MkdirAll(dest, node.Mode())
	if err != nil {
		return
	}

	children, err := ReadDir(src)
	if err != nil {
		return
	}
	for _, child := range children {
		if err = copy(filepath.Join(src, child.Name()), filepath.Join(dest, child.Name())); err != nil {
			return
		}
	}
	return
}

func ReadDir(src string) ([]os.FileInfo, error) {
	f, err := pkger.Open(src)
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

func copyLeaf(src, dest string) (err error) {
	srcFile, err := pkger.Open(src)
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
