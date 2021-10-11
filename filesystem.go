package function

import (
	"io"
	"os"
	"path/filepath"
	"sort"

	billy "github.com/go-git/go-billy/v5"
	"github.com/markbates/pkger"
)

// Filesystems
// Wrap the implementations of FS with their subtle differences into the
// common interface for accessing template files defined herein.
// os:    standard for on-disk extensible template repositories.
// pker:  embedded filesystem backed by the generated pkged.go.
// billy: go-git library's filesystem used for remote git template repos.

type filesystem interface {
	Stat(name string) (os.FileInfo, error)
	Open(path string) (file, error)
	ReadDir(path string) ([]os.FileInfo, error)
}

type file interface {
	io.Reader
	io.Closer
}

// pkgerFilesystem is template file accessor backed by the pkger-provided
// embedded filesystem.o
type pkgerFilesystem struct {
}

// the root of the repository is actually ./templates, which is proffered
// in the pkger filesystem as /templates, so all path requests will be
// prefixed with this path to emulate having the pkger fs root the same
// as the logical root.
const pkgerRoot = "/templates"

func (a pkgerFilesystem) Stat(path string) (os.FileInfo, error) {
	return pkger.Stat(filepath.Join(pkgerRoot, path))
}

func (a pkgerFilesystem) Open(path string) (file, error) {
	return pkger.Open(filepath.Join(pkgerRoot, path))
}

func (a pkgerFilesystem) ReadDir(path string) ([]os.FileInfo, error) {
	f, err := pkger.Open(filepath.Join(pkgerRoot, path))
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

// copy

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
