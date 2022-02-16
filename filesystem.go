package function

import (
	"archive/zip"
	"bytes"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	billy "github.com/go-git/go-billy/v5"
)

// Filesystems
// Wrap the implementations of FS with their subtle differences into the
// common interface for accessing template files defined herein.
// os:    standard for on-disk extensible template repositories.
// zip:   embedded filesystem backed by the byte array representing zipfile.
// billy: go-git library's filesystem used for remote git template repos.

type Filesystem interface {
	fs.ReadDirFS
	fs.StatFS
}

type zipFS struct {
	archive *zip.Reader
}

func (z zipFS) Open(name string) (fs.File, error) {
	return z.archive.Open(name)
}

func (z zipFS) ReadDir(name string) ([]fs.DirEntry, error) {
	var dirEntries []fs.DirEntry
	for _, file := range z.archive.File {
		cleanName := strings.TrimRight(file.Name, "/")
		if path.Dir(cleanName) == name {
			f, err := z.archive.Open(cleanName)
			if err != nil {
				return nil, err
			}
			fi, err := f.Stat()
			if err != nil {
				return nil, err
			}
			dirEntries = append(dirEntries, dirEntry{FileInfo: fi})
		}
	}
	return dirEntries, nil
}

func (z zipFS) Stat(name string) (fs.FileInfo, error) {
	f, err := z.archive.Open(name)
	if err != nil {
		return nil, err
	}
	return f.Stat()
}

//go:generate go run ./generate/templates/main.go

func newEmbeddedTemplatesFS() Filesystem {
	archive, err := zip.NewReader(bytes.NewReader(templatesZip), int64(len(templatesZip)))
	if err != nil {
		panic(err)
	}
	return zipFS{
		archive: archive,
	}
}

var EmbeddedTemplatesFS Filesystem = newEmbeddedTemplatesFS()

// billyFilesystem is a template file accessor backed by a billy FS
type billyFilesystem struct{ fs billy.Filesystem }

type bfsFile struct {
	billy.File
	stats func() (fs.FileInfo, error)
}

func (b bfsFile) Stat() (fs.FileInfo, error) {
	return b.stats()
}

func (b billyFilesystem) Open(name string) (fs.File, error) {
	f, err := b.fs.Open(name)
	if err != nil {
		return nil, err
	}
	return bfsFile{
		File: f,
		stats: func() (fs.FileInfo, error) {
			return b.fs.Stat(name)
		}}, nil
}

type dirEntry struct {
	fs.FileInfo
}

func (d dirEntry) Type() fs.FileMode {
	return d.Mode().Type()
}

func (d dirEntry) Info() (fs.FileInfo, error) {
	return d, nil
}

func (b billyFilesystem) ReadDir(name string) ([]fs.DirEntry, error) {
	fis, err := b.fs.ReadDir(name)
	if err != nil {
		return nil, err
	}
	var des = make([]fs.DirEntry, len(fis))
	for i, fi := range fis {
		des[i] = dirEntry{fi}
	}
	return des, nil
}

func (b billyFilesystem) Stat(name string) (fs.FileInfo, error) {
	return b.fs.Stat(name)
}

// osFilesystem is a template file accessor backed by the os.
type osFilesystem struct{ root string }

func (o osFilesystem) Open(name string) (fs.File, error) {
	name = filepath.FromSlash(name)
	return os.Open(filepath.Join(o.root, name))
}

func (o osFilesystem) ReadDir(name string) ([]fs.DirEntry, error) {
	name = filepath.FromSlash(name)
	return os.ReadDir(filepath.Join(o.root, name))
}

func (o osFilesystem) Stat(name string) (fs.FileInfo, error) {
	name = filepath.FromSlash(name)
	return os.Stat(filepath.Join(o.root, name))
}

// copy

func copy(src, dest string, accessor Filesystem) (err error) {
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

func copyNode(src, dest string, accessor Filesystem) (err error) {
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
		if err = copy(path.Join(src, child.Name()), filepath.Join(dest, child.Name()), accessor); err != nil {
			return
		}
	}
	return
}

func readDir(src string, accessor Filesystem) ([]fs.DirEntry, error) {
	list, err := accessor.ReadDir(src)
	if err != nil {
		return nil, err
	}
	sort.Slice(list, func(i, j int) bool { return list[i].Name() < list[j].Name() })
	return list, nil
}

func copyLeaf(src, dest string, accessor Filesystem) (err error) {
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
