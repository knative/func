// package testing includes minor testing helpers.
//
// These helpers include extensions to the testing nomenclature which exist to
// ease the development of tests for Functions.  It is mostly just syntactic
// sugar and closures for creating an removing test directories etc.
// It was originally included in each of the requisite testing packages, but
// since we use both private-access enabled tests (in the function package),
// as well as closed-box tests (in function_test package), and they are gradually
// increasing in size and complexity, the choice was made to choose a small
// dependency over a small amount of copying.
//
// Another reason for including these in a separate locaiton is that they will
// have no tags such that no combination of tags can cause them to either be
// missing or interfere with eachother (a problem encountered with knative
// tooling which by default runs tests with all tags enabled simultaneously)
package testing

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
)

// USING:  Make specified dir.  Return deferrable cleanup fn.
// Using the given path, create it as a new directory and return a deferrable
// which will remove it.
// usage:
//  defer using(t, "testdata/example.com/someExampleTest")()
func Using(t *testing.T, root string) func() {
	t.Helper()
	mkdir(t, root)
	return func() {
		rm(t, root)
	}
}

// mkdir creates a directory as a test helper, failing the test on error.
func mkdir(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
}

// rm a directory as a test helper, failing the test on error.
func rm(t *testing.T, dir string) {
	t.Helper()
	if err := os.RemoveAll(dir); err != nil {
		t.Fatal(err)
	}
}

// Within the given root creates the directory, CDs to it, and rturns a
// closure that when executed (intended in a defer) removes the given dirctory
// and returns the caller to the initial working directory.
// usage:
//   defer within(t, "somedir")()
func Within(t *testing.T, root string) func() {
	t.Helper()
	cwd := pwd(t)
	mkdir(t, root)
	cd(t, root)
	return func() {
		cd(t, cwd)
		rm(t, root)
	}
}

// Mktemp creates a temporary directory, CDs the current processes (test) to
// said directory, and returns the path to said directory.
// Usage:
//   path, rm := mktemp(t)
//   defer rm()
//   CWD is now 'path'
// errors encountererd fail the current test.
func Mktemp(t *testing.T) (string, func()) {
	t.Helper()
	tmp := tempdir(t)
	owd := pwd(t)
	cd(t, tmp)
	return tmp, func() {
		os.RemoveAll(tmp)
		cd(t, owd)
	}
}

// tempdir creates a new temporary directory and returns its path.
// errors fail the current test.
func tempdir(t *testing.T) string {
	d, err := ioutil.TempDir("", "dir")
	if err != nil {
		t.Fatal(err)
	}
	return d
}

// pwd prints the current working directory.
// errors fail the test.
func pwd(t *testing.T) string {
	t.Helper()
	d, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	return d
}

// cd changes directory to the given directory.
// errors fail the given test.
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
func TestRepoURI(name string, t *testing.T) string {
	t.Helper()
	cwd, _ := os.Getwd()
	repo := filepath.Join(cwd, "testdata", name+".git")
	return fmt.Sprintf(`file://%s`, repo)
}
