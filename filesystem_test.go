package function

import (
	"bytes"
	"crypto/md5"
	"encoding/hex"
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
)

func TestFileSystems(t *testing.T) {
	var err error

	tests := []struct {
		name       string
		fileSystem Filesystem
	}{
		{
			name:       "embedded",
			fileSystem: EmbeddedTemplatesFS,
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

			var embeddedFiles []string
			err = fs.WalkDir(templatesFS, ".", func(path string, d fs.DirEntry, err error) error {
				if err != nil {
					return err
				}
				embeddedFiles = append(embeddedFiles, path)
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}

			var localFiles []string
			err = filepath.Walk("templates", func(path string, info fs.FileInfo, err error) error {
				if err != nil {
					return err
				}
				path, err = filepath.Rel("templates", path)
				if err != nil {
					return err
				}
				localFiles = append(localFiles, filepath.ToSlash(path))
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}

			sort.Strings(embeddedFiles)
			sort.Strings(localFiles)

			if diff := cmp.Diff(localFiles, embeddedFiles); diff != "" {
				t.Error("filesystem content missmatch (-want, +got):", diff)
			}

			err = fs.WalkDir(templatesFS, ".", func(path string, d fs.DirEntry, err error) error {
				if err != nil {
					return err
				}

				if path == "." {
					return nil
				}

				localFilePath := filepath.Join("templates", path)

				localFileStat, err := os.Lstat(localFilePath)
				if err != nil {
					return err
				}

				embeddedFileStats, err := templatesFS.Stat(path)
				if err != nil {
					return err
				}

				if localFileStat.IsDir() && embeddedFileStats.IsDir() {
					return nil
				}

				if localFileStat.IsDir() != embeddedFileStats.IsDir() {
					t.Errorf("directory-file mismatch on %q", path)
					return nil
				}

				if localFileStat.Size() != embeddedFileStats.Size() {
					t.Errorf("size mismatch on %q (expected: %d, actual:%d)",
						path, localFileStat.Size(), embeddedFileStats.Size())
					return nil
				}

				embeddedFile, err := templatesFS.Open(path)
				if err != nil {
					return err
				}

				localFileContent, err := os.ReadFile(localFilePath)
				if err != nil {
					return err
				}

				embeddedFileContent, err := io.ReadAll(embeddedFile)
				if err != nil {
					return err
				}

				if !bytes.Equal(localFileContent, embeddedFileContent) {
					localSum := md5.Sum(localFileContent)
					embeddedSum := md5.Sum(embeddedFileContent)
					t.Errorf("content mismatch on %q (expected hash: %s, actual hash: %s)",
						path, hex.EncodeToString(localSum[:]), hex.EncodeToString(embeddedSum[:]))
					return nil
				}

				if runtime.GOOS != "windows" && (embeddedFileStats.Mode().Perm()&0100) != (localFileStat.Mode().Perm()&0100) {
					t.Errorf("mode mismatch on %q (expected: %o, actual: %o)",
						path, localFileStat.Mode().Perm(), embeddedFileStats.Mode().Perm())
					return nil
				}

				return nil
			})
			if err != nil {
				t.Error(err)
			}

		})
	}
}

func initOSFS(t *testing.T) Filesystem {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	return osFilesystem{root: filepath.Join(wd, "templates")}
}

func initGitFS(t *testing.T) Filesystem {
	repoDir := t.TempDir()

	err := filepath.Walk("templates", func(path string, fi fs.FileInfo, err error) error {
		if err != nil {
			return err
		}

		rel, err := filepath.Rel("templates", path)
		if err != nil {
			return err
		}
		if rel == "" || rel == "." {
			return nil
		}

		if fi.IsDir() {
			err = os.Mkdir(filepath.Join(repoDir, rel), fi.Mode().Perm())
			if err != nil {
				return err
			}
		} else {
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			err = os.WriteFile(filepath.Join(repoDir, rel), data, fi.Mode().Perm())
			if err != nil {
				return err
			}
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

	result, err := filesystemFromRepo(uri, "")
	if err != nil {
		t.Fatal(err)
	}
	return result
}
