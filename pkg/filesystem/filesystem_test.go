package filesystem_test

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/go-git/go-billy/v5/memfs"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/storage/memory"
	"github.com/google/go-cmp/cmp"

	"knative.dev/func/pkg/filesystem"
	fn "knative.dev/func/pkg/functions"

	// Ensure embed directive in templates package is compiled
	_ "knative.dev/func/templates"
)

const templatesPath = "../../templates"

type FileInfo struct {
	Path       string
	Typ        fs.FileMode
	Executable bool
	Content    []byte
}

func TestFileSystems(t *testing.T) {

	tests := []struct {
		name       string
		fileSystem filesystem.Filesystem
	}{
		{
			name:       "embedded",
			fileSystem: fn.EmbeddedTemplatesFS,
		},
		{
			name:       "os",
			fileSystem: initOSFS(t),
		},
		{
			name:       "git",
			fileSystem: initGitFS(t),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			templatesFS := tt.fileSystem

			if templatesFS == nil && runtime.GOOS == "windows" {
				t.Skip("FS == nil")
				// TODO I have no idea why it returns nil on Windows
			}

			embeddedFiles, err := loadFS(templatesFS)
			if err != nil {
				t.Fatal(err)
			}

			localFiles, err := loadLocalFiles(templatesPath)
			if err != nil {
				t.Fatal(err)
			}
			compare := func(fis []FileInfo) func(i, j int) bool {
				return func(i, j int) bool {
					return fis[i].Path < fis[j].Path
				}
			}
			sort.Slice(embeddedFiles, compare(embeddedFiles))
			sort.Slice(localFiles, compare(localFiles))

			if diff := cmp.Diff(localFiles, embeddedFiles); diff != "" {
				t.Error("filesystem content missmatch (-want, +got):", diff)
			}
		})
	}
}

func loadFS(fileSys filesystem.Filesystem) ([]FileInfo, error) {
	var err error
	var files []FileInfo

	permMask := fs.FileMode(0111)
	if runtime.GOOS == "windows" {
		permMask = 0
	}

	err = fs.WalkDir(fileSys, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		// Skip embed.go — infrastructure file, not a template.
		if p == "embed.go" {
			return nil
		}
		fi, err := fileSys.Stat(p)
		if err != nil {
			return err
		}
		var bs []byte
		switch fi.Mode() & fs.ModeType {
		case 0:
			f, err := fileSys.Open(p)
			if err != nil {
				return err
			}
			defer f.Close()
			bs, err = io.ReadAll(f)
			if err != nil {
				return err
			}
		case fs.ModeSymlink:
			t, _ := fileSys.Readlink(p)
			bs = []byte(t)
		}
		// Symlinks are never executable; only regular files can be.
		executable := fi.Mode()&fs.ModeType == 0 &&
			fi.Mode()&permMask == permMask
		files = append(files, FileInfo{
			Path:       p,
			Typ:        fi.Mode().Type(),
			Executable: executable,
			Content:    bs,
		})
		return nil
	})

	return files, err
}

// loadLocalFiles reads the templates directory on disk and reverses on-disk
// name mangling to produce the same view that manglingFS presents at runtime:
//   - foo.embd   → foo  (regular file)
//   - foo.symlink → foo  (symlink, content = target)
//   - embed.go is skipped (infrastructure, not a template)
//   - //go:build embd lines are stripped from .go file content
func loadLocalFiles(root string) ([]FileInfo, error) {
	var files []FileInfo
	var err error

	permMask := fs.FileMode(0111)
	if runtime.GOOS == "windows" {
		permMask = 0
	}

	err = filepath.Walk(root, func(p string, info fs.FileInfo, err error) error {
		if err != nil {
			return err
		}
		fi, err := os.Lstat(p)
		if err != nil {
			return err
		}

		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)

		// Skip embed.go — it is infrastructure, not a template file.
		if rel == "embed.go" {
			return nil
		}

		var (
			bs      []byte
			mode    fs.FileMode
			relPath = rel
		)

		// Reverse on-disk name mangling:
		//   foo.embd    → foo  (regular file, strip suffix)
		//   foo.symlink → foo  (present as symlink, content = target)
		switch {
		case strings.HasSuffix(rel, ".embd"):
			relPath = strings.TrimSuffix(rel, ".embd")
			mode = 0 // regular file
			bs, err = os.ReadFile(p)
			if err != nil {
				return err
			}
		case strings.HasSuffix(rel, ".symlink"):
			relPath = strings.TrimSuffix(rel, ".symlink")
			mode = fs.ModeSymlink
			raw, err2 := os.ReadFile(p)
			if err2 != nil {
				return err2
			}
			// Trim trailing newline (added for EOF-newline linting compliance).
			bs = []byte(strings.TrimRight(string(raw), "\n\r"))
		default:
			mode = fi.Mode().Type()
			switch fi.Mode() & fs.ModeType {
			case 0: // regular file
				bs, err = os.ReadFile(p)
				if err != nil {
					return err
				}
				// Strip //go:build embd from .go template files
				if strings.HasSuffix(p, ".go") {
					bs = stripEmbdBuildTag(bs)
				}
			case fs.ModeSymlink:
				t, _ := os.Readlink(p)
				bs = []byte(filepath.ToSlash(t))
			}
		}

		// Symlinks are never executable; only regular files can be.
		executable := mode == 0 && fi.Mode()&permMask == permMask
		files = append(files, FileInfo{
			Path:       relPath,
			Typ:        mode,
			Executable: executable,
			Content:    bs,
		})
		return nil
	})

	return files, err
}

// stripEmbdBuildTag removes "//go:build embd" and its trailing blank line.
func stripEmbdBuildTag(src []byte) []byte {
	const tag = "//go:build embd"
	lines := bytes.SplitAfter(src, []byte("\n"))
	out := make([]byte, 0, len(src))
	skipNext := false
	for _, line := range lines {
		trimmed := strings.TrimRight(string(line), "\r\n")
		if skipNext && trimmed == "" {
			skipNext = false
			continue
		}
		skipNext = false
		if trimmed == tag {
			skipNext = true
			continue
		}
		out = append(out, line...)
	}
	return out
}

func initOSFS(t *testing.T) filesystem.Filesystem {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	return filesystem.NewManglingFilesystem(filesystem.NewOsFilesystem(filepath.Join(wd, templatesPath)))
}

func initGitFS(t *testing.T) filesystem.Filesystem {
	repoDir := t.TempDir()

	err := filepath.Walk(templatesPath, func(path string, fi fs.FileInfo, err error) error {
		if err != nil {
			return err
		}

		rel, err := filepath.Rel(templatesPath, path)
		if err != nil {
			return err
		}
		if rel == "" || rel == "." {
			return nil
		}

		switch {
		case fi.IsDir():
			err = os.Mkdir(filepath.Join(repoDir, rel), fi.Mode().Perm())
			if err != nil {
				return err
			}
		case fi.Mode()&fs.ModeSymlink != 0:
			// Real symlinks (should not exist in the mangled tree, but handle gracefully)
			symlinkTarget, err := os.Readlink(path)
			if err != nil {
				t.Fatal(err)
			}
			err = os.Symlink(symlinkTarget, filepath.Join(repoDir, rel))
			if err != nil {
				t.Fatal(err)
			}
		case fi.Mode()&fs.ModeType == 0: // regular file (includes .symlink marker files)
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			err = os.WriteFile(filepath.Join(repoDir, rel), data, fi.Mode().Perm())
			if err != nil {
				return err
			}
		default:
			return fmt.Errorf("unsupported file type: %s", fi.Mode().String())
		}
		return nil
	})

	if err != nil {
		t.Fatal(err)
	}

	r, err := git.PlainInit(repoDir, false)
	if err != nil {
		t.Fatal(err)
	}
	w, err := r.Worktree()
	if err != nil {
		t.Fatal(err)
	}
	err = w.AddGlob(".")
	if err != nil {
		t.Fatal(err)
	}
	author := &object.Signature{Name: "johndoe"}
	_, err = w.Commit("init", &git.CommitOptions{
		Author:    author,
		Committer: author,
		All:       true,
	})
	if err != nil {
		t.Fatal(err)
	}

	uri := fmt.Sprintf(`file://%s`, filepath.ToSlash(repoDir))

	result, err := fn.FilesystemFromRepo(uri)
	if err != nil {
		t.Fatal(err)
	}
	return filesystem.NewManglingFilesystem(result)
}

func TestCopy(t *testing.T) {
	var err error

	expectedFiles, err := loadLocalFiles(filepath.Join("testdata", "root"))
	if err != nil {
		t.Fatal(err)
	}

	zr, err := zip.OpenReader(filepath.Join("testdata", "fs.zip"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = zr.Close() })

	clone, err := git.Clone(
		memory.NewStorage(),
		memfs.New(),
		&git.CloneOptions{URL: filepath.Join("testdata", "repo.git")},
	)
	if err != nil {
		t.Fatal(err)
	}
	wt, err := clone.Worktree()
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name       string
		fileSystem filesystem.Filesystem
	}{
		{
			name:       "os",
			fileSystem: filesystem.NewOsFilesystem(filepath.Join("testdata", "root")),
		},
		{
			name:       "zip",
			fileSystem: filesystem.NewZipFS(&zr.Reader),
		},
		{
			name:       "git",
			fileSystem: filesystem.NewBillyFilesystem(wt.Filesystem),
		},
		{
			name: "sub",
			fileSystem: filesystem.NewSubFS("a", mockFS{
				files: []FileInfo{
					{
						Path: "a",
						Typ:  fs.ModeDir,
					},
					{
						Path: "a/a",
						Typ:  fs.ModeDir,
					},
					{
						Path:    "a/a/hello.lnk",
						Typ:     fs.ModeSymlink,
						Content: []byte("hello.txt"),
					},
					{
						Path:    "a/a/hello.txt",
						Content: []byte("Hello World!\n"),
					},
				},
			}),
		},
		{
			name: "masking",
			fileSystem: filesystem.NewMaskingFS(func(p string) bool {
				return p == "ignored"
			},
				mockFS{
					files: []FileInfo{
						{
							Path: "a",
							Typ:  fs.ModeDir,
						},
						{
							Path:    "a/hello.lnk",
							Typ:     fs.ModeSymlink,
							Content: []byte("hello.txt"),
						},
						{
							Path:    "a/hello.txt",
							Content: []byte("Hello World!\n"),
						},
						{
							Path:    "ignored",
							Content: []byte("ignored"),
						},
					},
				}),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dest := t.TempDir()
			err = filesystem.CopyFromFS(".", dest, tt.fileSystem)
			if err != nil {
				t.Errorf("cannot copy: %v", err)
			}
			var actualFiles []FileInfo
			actualFiles, err = loadLocalFiles(dest)
			if err != nil {
				t.Errorf("cannot load local files: %v", err)
			}

			if diff := cmp.Diff(expectedFiles, actualFiles); diff != "" {
				t.Error("filesystem content missmatch (-want, +got):", diff)
			}
			t.Log(actualFiles)
		})
	}
}

// mock for testing symlink functionality
type mockFS struct {
	files []FileInfo
}

func (m mockFS) lookupFile(name string) (FileInfo, bool) {
	if name == "." {
		return FileInfo{Path: ".", Typ: fs.ModeDir}, true
	}
	for _, file := range m.files {
		if file.Path == name {
			return file, true
		}
	}
	return FileInfo{}, false
}

type mockFile struct {
	FileInfo
	io.ReadCloser
}

func (m mockFile) Stat() (fs.FileInfo, error) {
	return m.FileInfo, nil
}

func (m mockFS) Open(name string) (fs.File, error) {

	file, ok := m.lookupFile(name)
	if !ok {
		return nil, fs.ErrNotExist
	}
	return mockFile{FileInfo: file, ReadCloser: io.NopCloser(bytes.NewReader(file.Content))}, nil
}

func (m mockFS) ReadDir(name string) ([]fs.DirEntry, error) {
	_, ok := m.lookupFile(name)
	if !ok {
		return nil, fs.ErrNotExist
	}

	var dirEntries []fs.DirEntry
	for _, file := range m.files {
		cleanName := strings.TrimRight(file.Path, "/")
		if path.Dir(cleanName) == name {
			dirEntries = append(dirEntries, file)
		}
	}
	return dirEntries, nil
}

func (m mockFS) Stat(name string) (fs.FileInfo, error) {
	file, ok := m.lookupFile(name)
	if !ok {
		return nil, fs.ErrNotExist
	}
	return file, nil
}

func (m mockFS) Readlink(link string) (string, error) {
	file, ok := m.lookupFile(link)
	if !ok {
		return "", fs.ErrNotExist
	}
	if file.Typ != fs.ModeSymlink {
		return "", fs.ErrInvalid
	}
	return string(file.Content), nil
}

func (f FileInfo) Name() string {
	return path.Base(f.Path)
}

func (f FileInfo) Size() int64 {
	return int64(len(f.Content))
}

func (f FileInfo) Mode() fs.FileMode {
	if f.Typ == fs.ModeSymlink {
		return f.Typ | 0777
	}
	if f.Executable || f.Typ == fs.ModeDir {
		return f.Typ | 0755
	}
	return f.Typ | 0644
}

func (f FileInfo) ModTime() time.Time {
	return time.Time{}
}

func (f FileInfo) IsDir() bool {
	return f.Typ.IsDir()
}

func (f FileInfo) Sys() any {
	return nil
}

func (f FileInfo) Type() fs.FileMode {
	return f.Typ
}

func (f FileInfo) Info() (fs.FileInfo, error) {
	return f, nil
}
