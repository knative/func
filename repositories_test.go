// +build !integration

package function_test

import (
	"os"
	"path/filepath"
	"testing"

	fn "knative.dev/kn-plugin-func"
)

const RepositoriesTestRepo = "repository-a"

// TestRepositoriesList ensures the base case of listing
// repositories without error in the default scenario of builtin only.
func TestRepositoriesList(t *testing.T) {
	root, rm := mktemp(t)
	defer rm()

	client := fn.New(fn.WithRepositories(root)) // Explicitly empty

	rr, err := client.Repositories().List()
	if err != nil {
		t.Fatal(err)
	}
	// Assert contains only the default repo
	if len(rr) != 1 && rr[0] != fn.DefaultRepository {
		t.Fatalf("Expected repository list '[%v]', got %v", fn.DefaultRepository, rr)
	}
}

// TestRepositoriesGet ensures a repository can be accessed by name.
func TestRepositoriesGet(t *testing.T) {
	client := fn.New(fn.WithRepositories("testdata/repositories"))

	// invalid should error
	repo, err := client.Repositories().Get("invalid")
	if err == nil {
		t.Fatal("did not receive expected error getting inavlid repository")
	}

	// valid should not error
	repo, err = client.Repositories().Get("customProvider")
	if err != nil {
		t.Fatal(err)
	}

	// valid should have expected name
	if repo.Name != "customProvider" {
		t.Fatalf("expected 'customProvider' as repository name, got: %v", repo.Name)
	}
}

// TestRepositoriesAll ensures builtin and extended repos are returned from
// .All accessor.
func TestRepositoriesAll(t *testing.T) {
	uri := testRepoURI(RepositoriesTestRepo, t)
	root, rm := mktemp(t)
	defer rm()

	client := fn.New(fn.WithRepositories(root))

	// Assert initially only the default is included
	rr, err := client.Repositories().All()
	if err != nil {
		t.Fatal(err)
	}
	if len(rr) != 1 && rr[0].Name != fn.DefaultRepository {
		t.Fatalf("Expected initial repo list to be only the default.  Got %v", rr)
	}

	// Add one
	err = client.Repositories().Add("", uri)
	if err != nil {
		t.Fatal(err)
	}

	// Get full list
	repositories, err := client.Repositories().All()
	if err != nil {
		t.Fatal(err)
	}

	// Assert it now includes both builtin and extended
	if len(repositories) != 2 ||
		repositories[0].Name != fn.DefaultRepository ||
		repositories[1].Name != RepositoriesTestRepo {
		t.Fatal("Repositories list does not pass shallow repository membership check")
	}
}

// TestRepositoriesAdd ensures that adding a repository adds it to the FS
// and List output.  Uses default name (repo name).
func TestRepositoriesAdd(t *testing.T) {
	uri := testRepoURI(RepositoriesTestRepo, t) // ./testdata/$RepositoriesTestRepo.git
	root, rm := mktemp(t)                       // create and cd to a temp dir
	defer rm()

	client := fn.New(fn.WithRepositories(root))

	// Add repo at uri
	if err := client.Repositories().Add("", uri); err != nil {
		t.Fatal(err)
	}

	// Assert list now includes the test repo
	rr, err := client.Repositories().List()
	if err != nil {
		t.Fatal(err)
	}
	if len(rr) != 2 || rr[1] != RepositoriesTestRepo {
		t.Fatalf("Expected '%v', got %v", RepositoriesTestRepo, rr)
	}

	// assert expected name
	if rr[1] != RepositoriesTestRepo {
		t.Fatalf("Expected name '%v', got %v", RepositoriesTestRepo, rr[1])
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
	uri := testRepoURI(RepositoriesTestRepo, t) // ./testdata/$RepositoriesTestRepo.git
	root, rm := mktemp(t)                       // create and cd to a temp dir, returning path.
	defer rm()

	// Instantiate the client using the current temp directory as the
	// repositories' root location.
	client := fn.New(fn.WithRepositories(root))

	name := "example"                                            // the custom name for the new repo
	if err := client.Repositories().Add(name, uri); err != nil { // add with name
		t.Fatal(err)
	}

	rr, err := client.Repositories().List()
	if err != nil {
		t.Fatal(err)
	}
	if len(rr) != 2 || rr[1] != name {
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
	uri := testRepoURI(RepositoriesTestRepo, t)
	root, rm := mktemp(t) // create and cd to a temp dir, returning path.
	defer rm()

	// Instantiate the client using the current temp directory as the
	// repositories' root location.
	client := fn.New(fn.WithRepositories(root))

	// Add twice.
	name := "example"
	if err := client.Repositories().Add(name, uri); err != nil {
		t.Fatal(err)
	}
	if err := client.Repositories().Add(name, uri); err == nil {
		t.Fatalf("did not receive expected error adding an existing repository")
	}

	// assert repo named correctly
	rr, err := client.Repositories().List()
	if err != nil {
		t.Fatal(err)
	}
	if len(rr) != 2 || rr[1] != name {
		t.Fatalf("Expected '[%v]', got %v", name, rr)
	}

	// assert repo files exist
	if _, err := os.Stat(filepath.Join(name, "README.md")); os.IsNotExist(err) {
		t.Fatalf("Repository does not appear on disk as expected: %v", err)
	}
}

// TestRepositoriesRename ensures renaming a repository.
func TestRepositoriesRename(t *testing.T) {
	uri := testRepoURI(RepositoriesTestRepo, t)
	root, rm := mktemp(t) // create and cd to a temp dir, returning path.
	defer rm()

	// Instantiate the client using the current temp directory as the
	// repositories' root location.
	client := fn.New(fn.WithRepositories(root))

	// Add and Rename
	if err := client.Repositories().Add("foo", uri); err != nil {
		t.Fatal(err)
	}
	if err := client.Repositories().Rename("foo", "bar"); err != nil {
		t.Fatal(err)
	}

	// assert repo named correctly
	rr, err := client.Repositories().List()
	if err != nil {
		t.Fatal(err)
	}
	if len(rr) != 2 || rr[1] != "bar" {
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
	uri := testRepoURI(RepositoriesTestRepo, t) // ./testdata/repository.git
	root, rm := mktemp(t)                       // create and cd to a temp dir, returning path.
	defer rm()

	// Instantiate the client using the current temp directory as the
	// repositories' root location.
	client := fn.New(fn.WithRepositories(root))

	// Add and Remove
	name := "example"
	if err := client.Repositories().Add(name, uri); err != nil {
		t.Fatal(err)
	}
	if err := client.Repositories().Remove(name); err != nil {
		t.Fatal(err)
	}

	// assert repo not in list
	rr, err := client.Repositories().List()
	if err != nil {
		t.Fatal(err)
	}
	if len(rr) != 1 {
		t.Fatalf("Expected repo list of len 1.  Got %v", rr)
	}

	// assert repo not on filesystem
	if _, err := os.Stat(name); !os.IsNotExist(err) {
		t.Fatalf("Repo %v still exists on filesystem.", name)
	}
}

// TestRepositoriesURL ensures that a repository populates its URL member
// from the git repository's origin url (if it is a git repo and exists)
func TestRepositoriesURL(t *testing.T) {
	uri := testRepoURI(RepositoriesTestRepo, t)
	root, rm := mktemp(t)
	defer rm()

	client := fn.New(fn.WithRepositories(root))

	// Add the test repo
	err := client.Repositories().Add("newrepo", uri)
	if err != nil {
		t.Fatal(err)
	}

	// Get the newly added repo
	r, err := client.Repositories().Get("newrepo")
	if err != nil {
		t.Fatal(err)
	}

	// Assert it includes the correct URL
	if r.URL != uri {
		t.Fatalf("expected repository URL '%v', got '%v'", uri, r.URL)
	}
}
