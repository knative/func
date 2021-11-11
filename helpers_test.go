package function_test

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
)

// Helpers for all tests (outside core package in function_test)
// Held in a separate file without any build tags such that no combination
// of tags can cause them to either be missing or interfere.

// USING:  Make specified dir.  Return deferrable cleanup fn.
func using(t *testing.T, root string) func() {
	t.Helper()
	mkdir(t, root)
	return func() {
		rm(t, root)
	}
}

func mkdir(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
}

func rm(t *testing.T, dir string) {
	t.Helper()
	if err := os.RemoveAll(dir); err != nil {
		t.Fatal(err)
	}
}

// MKTEMP:  Create and CD to a temp dir.
//          Returns a deferrable cleanup fn.
func mktemp(t *testing.T) (string, func()) {
	t.Helper()
	tmp := tempdir(t)
	owd := pwd(t)
	cd(t, tmp)
	return tmp, func() {
		os.RemoveAll(tmp)
		cd(t, owd)
	}
}

func tempdir(t *testing.T) string {
	d, err := ioutil.TempDir("", "dir")
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func pwd(t *testing.T) string {
	t.Helper()
	d, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func cd(t *testing.T, dir string) {
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
}

// TEST REPO URI:  Return URI to repo in ./testdata of matching name.
// Suitable as URI for repository override. returns in form file://
// Must be called prior to mktemp in tests which changes current
// working directory as it depends on a relative path.
// Repo uri:  file://$(pwd)/testdata/repository.git (unix-like)
//            file: //$(pwd)\testdata\repository.git (windows)
func testRepoURI(name string, t *testing.T) string {
	t.Helper()
	cwd, _ := os.Getwd()
	repo := filepath.Join(cwd, "testdata", name+".git")
	return fmt.Sprintf(`file://%s`, repo)
}
