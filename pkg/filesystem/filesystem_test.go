package filesystem_test

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"testing"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/google/go-cmp/cmp"

	"knative.dev/func/pkg/filesystem"
	fn "knative.dev/func/pkg/functions"
)

const templatesPath = "../../templates"

func TestFileSystems(t *testing.T) {
	var err error

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

	type FileInfo struct {
		Path       string
		Type       fs.FileMode
		Executable bool
		Content    []byte
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			templatesFS := tt.fileSystem

			if templatesFS == nil && runtime.GOOS == "windows" {
				t.Skip("FS == nil")
				// TODO I have no idea why it returns nil on Windows
			}

			permMask := fs.FileMode(0111)
			if runtime.GOOS == "windows" {
				permMask = 0
			}

			var embeddedFiles []FileInfo
			err = fs.WalkDir(templatesFS, ".", func(path string, d fs.DirEntry, err error) error {
				if err != nil {
					return err
				}
				fi, err := templatesFS.Stat(path)
				if err != nil {
					return err
				}
				var bs []byte
				switch fi.Mode() & fs.ModeType {
				case 0:
					f, err := templatesFS.Open(path)
					if err != nil {
						return err
					}
					defer f.Close()
					bs, err = io.ReadAll(f)
					if err != nil {
						return err
					}
				case fs.ModeSymlink:
					t, _ := templatesFS.Readlink(path)
					bs = []byte(t)
				}
				embeddedFiles = append(embeddedFiles, FileInfo{
					Path:       path,
					Type:       fi.Mode().Type(),
					Executable: fi.Mode()&permMask == permMask && !fi.IsDir(),
					Content:    bs,
				})
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}

			var localFiles []FileInfo
			err = filepath.Walk(templatesPath, func(path string, info fs.FileInfo, err error) error {
				if err != nil {
					return err
				}
				fi, err := os.Lstat(path)
				if err != nil {
					return err
				}
				var bs []byte
				switch fi.Mode() & fs.ModeType {
				case 0:
					bs, err = os.ReadFile(path)
					if err != nil {
						return err
					}
				case fs.ModeSymlink:
					t, _ := os.Readlink(path)
					bs = []byte(t)
				}
				path, err = filepath.Rel(templatesPath, path)
				if err != nil {
					return err
				}
				localFiles = append(localFiles, FileInfo{
					Path:       filepath.ToSlash(path),
					Type:       fi.Mode().Type(),
					Executable: fi.Mode()&permMask == permMask && !fi.IsDir(),
					Content:    bs,
				})
				return nil
			})
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

func initOSFS(t *testing.T) filesystem.Filesystem {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	return filesystem.NewOsFilesystem(filepath.Join(wd, templatesPath))
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
			symlinkTarget, err := os.Readlink(path)
			if err != nil {
				t.Fatal(err)
			}
			err = os.Symlink(symlinkTarget, filepath.Join(repoDir, rel))
			if err != nil {
				t.Fatal(err)
			}
		case fi.Mode()&fs.ModeType == 0: // regular file
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
	return result
}
