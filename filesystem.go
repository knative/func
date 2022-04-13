package function

import (
	"archive/zip"
	"bytes"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
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

// subFS exposes subdirectory of underlying FS, this is similar to `chroot`.
type subFS struct {
	root string
	fs   Filesystem
}

func (o subFS) Open(name string) (fs.File, error) {
	return o.fs.Open(path.Join(o.root, name))
}

func (o subFS) ReadDir(name string) ([]fs.DirEntry, error) {
	return o.fs.ReadDir(path.Join(o.root, name))
}

func (o subFS) Stat(name string) (fs.FileInfo, error) {
	return o.fs.Stat(path.Join(o.root, name))
}

type maskingFS struct {
	masked func(path string) bool
	fs     Filesystem
}

func (m maskingFS) Open(name string) (fs.File, error) {
	if m.masked(name) {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
	}
	return m.fs.Open(name)
}

func (m maskingFS) ReadDir(name string) ([]fs.DirEntry, error) {
	if m.masked(name) {
		return nil, &fs.PathError{Op: "readdir", Path: name, Err: fs.ErrNotExist}
	}
	des, err := m.fs.ReadDir(name)
	if err != nil {
		return nil, err
	}
	result := make([]fs.DirEntry, 0, len(des))
	for _, de := range des {
		if !m.masked(path.Join(name, de.Name())) {
			result = append(result, de)
		}
	}
	return result, nil
}

func (m maskingFS) Stat(name string) (fs.FileInfo, error) {
	if m.masked(name) {
		return nil, &fs.PathError{Op: "stat", Path: name, Err: fs.ErrNotExist}
	}
	return m.fs.Stat(name)
}

// copyFromFS copies files from the `src` dir on the accessor Filesystem to local filesystem into `dest` dir.
// The src path uses slashes as their separator.
// The dest path uses OS specific separator.
func copyFromFS(src, dest string, accessor Filesystem) (err error) {

	return fs.WalkDir(accessor, src, func(path string, de fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if path == src {
			return nil
		}

		p, err := filepath.Rel(filepath.FromSlash(src), filepath.FromSlash(path))
		if err != nil {
			return err
		}

		dest := filepath.Join(dest, p)
		if de.IsDir() {
			// Ideally we should use the file mode of the src node
			// but it seems the git module is reporting directories
			// as 0644 instead of 0755. For now, just do it this way.
			// See https://github.com/go-git/go-git/issues/364
			// Upon resolution, return accessor.Stat(src).Mode()
			return os.MkdirAll(dest, 0755)
		}
		fi, err := de.Info()
		if err != nil {
			return err
		}

		destFile, err := os.OpenFile(dest, os.O_RDWR|os.O_CREATE|os.O_TRUNC, fi.Mode())
		if err != nil {
			return err
		}
		defer destFile.Close()

		srcFile, err := accessor.Open(path)
		if err != nil {
			return err
		}
		defer srcFile.Close()

		_, err = io.Copy(destFile, srcFile)
		return err
	})

}
