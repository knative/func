// +build !integration

package function_test

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	fn "knative.dev/kn-plugin-func"
)

const RepositoriesTestRepo = "repository-a"

// TestRepositoriesList ensures the base case of an empty list
// when no repositories are installed.
func TestRepositoriesList(t *testing.T) {
	root, rm := mktemp(t)
	defer rm()

	client := fn.New(fn.WithRepositories(root))

	rr, err := client.Repositories.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(rr) != 0 {
		t.Fatalf("Expected an empty repositories list, got %v", len(rr))
	}
}

// TestRepositoriesAdd ensures that adding a repository adds it to the FS
// and List output.  Uses default name (repo name).
func TestRepositoriesAdd(t *testing.T) {
	uri := testRepoURI(t) // ./testdata/$RepositoriesTestRepo.git
	root, rm := mktemp(t) // create and cd to a temp dir
	defer rm()

	// Instantiate the client using the current temp directory as the
	// repositories' root location.
	client := fn.New(fn.WithRepositories(root))

	// Create a new repository without a name (use default of repo name)
	if err := client.Repositories.Add("", uri); err != nil {
		t.Fatal(err)
	}

	// assert list len 1
	rr, err := client.Repositories.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(rr) != 1 || rr[0] != RepositoriesTestRepo {
		t.Fatalf("Expected '%v', got %v", RepositoriesTestRepo, rr)
	}

	// assert expected name
	if rr[0] != RepositoriesTestRepo {
		t.Fatalf("Expected name '%v', got %v", RepositoriesTestRepo, rr[0])
	}

	// assert repo was checked out
	if _, err := os.Stat(filepath.Join(RepositoriesTestRepo, "README.md")); os.IsNotExist(err) {
		t.Fatalf("Repository does not appear on disk as expected: %v", err)
	}
	if err != nil {
		t.Fatal(err) // other unexpected error.
	}
}

// TestRepositoriesAddNamed ensures that adding a repository with a specified
// name takes precidence over the default of repo name.
func TestRepositoriesAddNamed(t *testing.T) {
	uri := testRepoURI(t) // ./testdata/$RepositoriesTestRepo.git
	root, rm := mktemp(t) // create and cd to a temp dir, returning path.
	defer rm()

	// Instantiate the client using the current temp directory as the
	// repositories' root location.
	client := fn.New(fn.WithRepositories(root))

	name := "example"                                          // the custom name for the new repo
	if err := client.Repositories.Add(name, uri); err != nil { // add with name
		t.Fatal(err)
	}

	rr, err := client.Repositories.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(rr) != 1 || rr[0] != name {
		t.Fatalf("Expected '%v', got %v", name, rr)
	}

	// assert repo files exist
	if _, err := os.Stat(filepath.Join(name, "README.md")); os.IsNotExist(err) {
		t.Fatalf("Repository does not appear on disk as expected: %v", err)
	}
}

// TestRepositoriesAddExistingErrors ensures that adding a repository that
// already exists yields an error.
func TestRepositoriesAddExistingErrors(t *testing.T) {
	uri := testRepoURI(t)
	root, rm := mktemp(t) // create and cd to a temp dir, returning path.
	defer rm()

	// Instantiate the client using the current temp directory as the
	// repositories' root location.
	client := fn.New(fn.WithRepositories(root))

	// Add twice.
	name := "example"
	if err := client.Repositories.Add(name, uri); err != nil {
		t.Fatal(err)
	}
	if err := client.Repositories.Add(name, uri); err == nil {
		t.Fatalf("did not receive expected error adding an existing repository")
	}

	// assert repo named correctly
	rr, err := client.Repositories.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(rr) != 1 || rr[0] != name {
		t.Fatalf("Expected '[%v]', got %v", name, rr)
	}

	// assert repo files exist
	if _, err := os.Stat(filepath.Join(name, "README.md")); os.IsNotExist(err) {
		t.Fatalf("Repository does not appear on disk as expected: %v", err)
	}
}

// TestRepositoriesRename ensures renaming a repository.
func TestRepositoriesRename(t *testing.T) {
	uri := testRepoURI(t)
	root, rm := mktemp(t) // create and cd to a temp dir, returning path.
	defer rm()

	// Instantiate the client using the current temp directory as the
	// repositories' root location.
	client := fn.New(fn.WithRepositories(root))

	// Add and Rename
	if err := client.Repositories.Add("foo", uri); err != nil {
		t.Fatal(err)
	}
	if err := client.Repositories.Rename("foo", "bar"); err != nil {
		t.Fatal(err)
	}

	// assert repo named correctly
	rr, err := client.Repositories.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(rr) != 1 || rr[0] != "bar" {
		t.Fatalf("Expected '[bar]', got %v", rr)
	}

	// assert repo files exist
	if _, err := os.Stat(filepath.Join("bar", "README.md")); os.IsNotExist(err) {
		t.Fatalf("Repository does not appear on disk as expected: %v", err)
	}
}

// TestRepositoriesRemove ensures that removing a repository by name
// removes it from the list and FS.
func TestRepositoriesRemove(t *testing.T) {
	uri := testRepoURI(t) // ./testdata/repository.git
	root, rm := mktemp(t) // create and cd to a temp dir, returning path.
	defer rm()

	// Instantiate the client using the current temp directory as the
	// repositories' root location.
	client := fn.New(fn.WithRepositories(root))

	// Add and Remove
	name := "example"
	if err := client.Repositories.Add(name, uri); err != nil {
		t.Fatal(err)
	}
	if err := client.Repositories.Remove(name); err != nil {
		t.Fatal(err)
	}

	// assert repo not in list
	rr, err := client.Repositories.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(rr) != 0 {
		t.Fatalf("Expected empty repo list upon remove.  Got %v", rr)
	}

	// assert repo not on filesystem
	if _, err := os.Stat(name); !os.IsNotExist(err) {
		t.Fatalf("Repo %v still exists on filesystem.", name)
	}
}

// Helpers ---

// testRepoURI returns a file:// URI to a test repository in
// testdata.  Must be called prior to mktemp in tests which changes current
// working directory.
// Repo uri:  file://$(pwd)/testdata/repository.git (unix-like)
//            file: //$(pwd)\testdata\repository.git (windows)
func testRepoURI(t *testing.T) string {
	t.Helper()
	cwd, _ := os.Getwd()
	repo := filepath.Join(cwd, "testdata", RepositoriesTestRepo+".git")
	return "file://" + filepath.ToSlash(repo)
}

// mktemp creates a temp dir, returning its path
// and a function which will remove it.
func mktemp(t *testing.T) (string, func()) {
	t.Helper()
	tmp := mktmp(t)
	owd := pwd(t)
	cd(t, tmp)
	return tmp, func() {
		os.RemoveAll(tmp)
		cd(t, owd)
	}
}

func mktmp(t *testing.T) string {
	d, err := ioutil.TempDir("", "dir")
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func pwd(t *testing.T) string {
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
