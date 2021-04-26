package tarfs

import (
	"archive/tar"
	"bytes"
	"io"
	"io/fs"
	"path"
	"sort"
	"strings"
	"sync"
	"time"
)

// FS is a tar-backed fs.FS
// adapted from testing/fstest.MapFS
type FS map[string]*file

// file can be any file within the FS
type file struct {
	Data    []byte
	Mode    fs.FileMode
	ModTime time.Time
	Sys     interface{}
}

var _ fs.FS = FS(nil)
var _ fs.File = (*openFile)(nil)

// New tar FS from a reader attached to a tarball.
func New(r io.Reader) (FS, error) {
	mapfs := make(map[string]*file)

	tr := tar.NewReader(r)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			return mapfs, nil
		}
		if err != nil {
			return mapfs, err
		}

		// Create the file entry in the memory FS
		mapfs[header.Name] = &file{
			Mode:    header.FileInfo().Mode(),
			ModTime: header.FileInfo().ModTime(),
			Sys:     header.FileInfo().Sys,
		}

		// Done if directory
		if header.FileInfo().IsDir() {
			continue
		}

		// Copy over the data as well
		buf := bytes.Buffer{}
		if _, err = buf.ReadFrom(tr); err != nil {
			return mapfs, err
		}
		mapfs[header.Name].Data = buf.Bytes()
	}
}

func (fsys FS) Open(name string) (fs.File, error) {
	if !fs.ValidPath(name) {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
	}
	f := fsys[name]
	if f != nil && f.Mode&fs.ModeDir == 0 {
		// Ordinary file
		return &openFile{fileInfo{path.Base(name), f}, name, 0}, nil
	}

	// Directory, possibly synthesized.
	// Note that file can be nil here: the map need not contain explicit parent directories for all its files.
	// But file can also be non-nil, in case the user wants to set metadata for the directory explicitly.
	// Either way, we need to construct the list of children of this directory.
	list := []fileInfo{}
	elem := ""
	need := make(map[string]bool)
	if name == "." {
		elem = "."
		for fname, f := range fsys {
			i := strings.Index(fname, "/")
			if i < 0 {
				list = append(list, fileInfo{fname, f})
			} else {
				need[fname[:i]] = true
			}
		}
	} else {
		elem = name[strings.LastIndex(name, "/")+1:]
		prefix := name + "/"
		for fname, f := range fsys {
			if strings.HasPrefix(fname, prefix) {
				felem := fname[len(prefix):]
				i := strings.Index(felem, "/")
				if i < 0 {
					list = append(list, fileInfo{felem, f})
				} else {
					need[fname[len(prefix):len(prefix)+i]] = true
				}
			}
		}
		// If the directory name is not in the map,
		// and there are no children of the name in the map,
		// then the directory is treated as not existing.
		if f == nil && len(list) == 0 && len(need) == 0 {
			return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
		}
	}
	for _, fi := range list {
		delete(need, fi.name)
	}
	for name := range need {
		list = append(list, fileInfo{name, &file{Mode: fs.ModeDir}})
	}
	sort.Slice(list, func(i, j int) bool {
		return list[i].name < list[j].name
	})

	if f == nil {
		f = &file{Mode: fs.ModeDir}
	}
	return &dir{fileInfo{elem, f}, name, list, 0, sync.Mutex{}}, nil
}

// dir represents a directory in the FS
type dir struct {
	fileInfo
	path    string
	entries []fileInfo
	offset  int
	sync.Mutex
}

func (d *dir) Stat() (fs.FileInfo, error) { return &d.fileInfo, nil }
func (d *dir) Close() error               { return nil }
func (d *dir) Read(b []byte) (int, error) {
	return 0, &fs.PathError{Op: "read", Path: d.path, Err: fs.ErrInvalid}
}
func (d *dir) ReadDir(count int) (entries []fs.DirEntry, err error) {
	d.Lock()
	defer d.Unlock()
	n := len(d.entries) - d.offset
	if count > 0 && n > count {
		n = count
	}
	if n == 0 && count > 0 {
		return nil, io.EOF
	}
	list := make([]fs.DirEntry, n)
	for i := range list {
		list[i] = &d.entries[d.offset+i]
	}
	d.offset += n
	return list, nil
}

// fileInfo wraps files with metadata
type fileInfo struct {
	name string
	f    *file
}

func (i *fileInfo) Name() string               { return i.name }
func (i *fileInfo) Size() int64                { return int64(len(i.f.Data)) }
func (i *fileInfo) Mode() fs.FileMode          { return i.f.Mode }
func (i *fileInfo) Type() fs.FileMode          { return i.f.Mode.Type() }
func (i *fileInfo) ModTime() time.Time         { return i.f.ModTime }
func (i *fileInfo) IsDir() bool                { return i.f.Mode&fs.ModeDir != 0 }
func (i *fileInfo) Sys() interface{}           { return i.f.Sys }
func (i *fileInfo) Info() (fs.FileInfo, error) { return i, nil }

// openFile decorates a fileInfo with accessors to the underlying data for use
// by Open
type openFile struct {
	fileInfo
	path   string
	offset int64
}

func (f *openFile) Stat() (fs.FileInfo, error) { return &f.fileInfo, nil }
func (f *openFile) Close() error               { return nil }

func (f *openFile) Read(b []byte) (int, error) {
	if f.offset >= int64(len(f.f.Data)) {
		return 0, io.EOF
	}
	if f.offset < 0 {
		return 0, &fs.PathError{Op: "read", Path: f.path, Err: fs.ErrInvalid}
	}
	n := copy(b, f.f.Data[f.offset:])
	f.offset += int64(n)
	return n, nil
}

func (f *openFile) Seek(offset int64, whence int) (int64, error) {
	switch whence {
	case 0:
		// offset += 0
	case 1:
		offset += f.offset
	case 2:
		offset += int64(len(f.f.Data))
	}
	if offset < 0 || offset > int64(len(f.f.Data)) {
		return 0, &fs.PathError{Op: "seek", Path: f.path, Err: fs.ErrInvalid}
	}
	f.offset = offset
	return offset, nil
}

func (f *openFile) ReadAt(b []byte, offset int64) (int, error) {
	if offset < 0 || offset > int64(len(f.f.Data)) {
		return 0, &fs.PathError{Op: "read", Path: f.path, Err: fs.ErrInvalid}
	}
	n := copy(b, f.f.Data[offset:])
	if n < len(b) {
		return n, io.EOF
	}
	return n, nil
}
