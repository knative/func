//go:build !integration
// +build !integration

package function_test

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	fn "knative.dev/kn-plugin-func"
)

// RepositoriesTestRepo is the general-purpose example repository for most
// test.  Others do eist with specific test requirements that are mutually
// exclusive, such as manifest differences, and are specified inline to their
// requisite test.
const RepositoriesTestRepo = "repository"

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

// TestRepositoriesGetInvalid ensures that attempting to get an invalid repo results in error.
func TestRepositoriesGetInvalid(t *testing.T) {
	client := fn.New(fn.WithRepositories("testdata/repositories"))

	// invalid should error
	_, err := client.Repositories().Get("invalid")
	if err == nil {
		t.Fatal("did not receive expected error getting inavlid repository")
	}
}

// TestRepositoriesGet ensures a repository can be accessed by name.
func TestRepositoriesGet(t *testing.T) {
	client := fn.New(fn.WithRepositories("testdata/repositories"))

	// valid should not error
	repo, err := client.Repositories().Get("customProvider")
	if err != nil {
		t.Fatal(err)
	}

	// valid should have expected name
	if repo.Name != "customProvider" {
		t.Fatalf("Expected 'customProvider', got: %v", repo.Name)
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
	_, err = client.Repositories().Add("", uri)
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

// TestRepositoriesAdd checks basic adding of a repository by URI.
func TestRepositoriesAdd(t *testing.T) {
	uri := testRepoURI(RepositoriesTestRepo, t) // ./testdata/$RepositoriesTestRepo.git
	root, rm := mktemp(t)                       // create and cd to a temp dir, returning path.
	defer rm()

	// Instantiate the client using the current temp directory as the
	// repositories' root location.
	client := fn.New(fn.WithRepositories(root))

	// Add the repository, explicitly specifying a name.  See other tests for
	// defaulting from repoistory names and manifest-defined name.
	if _, err := client.Repositories().Add("example", uri); err != nil {
		t.Fatal(err)
	}

	// Confirm the list now contains the name
	rr, err := client.Repositories().List()
	if err != nil {
		t.Fatal(err)
	}
	expected := []string{"default", "example"}
	if !reflect.DeepEqual(rr, expected) {
		t.Fatalf("Expected '%v', got %v", expected, rr)
	}

	// assert a file exists at the location as well indicating it was added to
	// the filesystem, not just the list.
	if _, err := os.Stat(filepath.Join("example", "README.md")); os.IsNotExist(err) {
		t.Fatalf("Repository does not appear on disk as expected: %v", err)
	}
}

// TestRepositoriesAddDefaultName ensures that repository name is optional,
// by default being set to the name of the repoisotory from the URI.
func TestRepositoriesAddDeafultName(t *testing.T) {
	// The test repository is the "base case" repo, which is a manifestless
	// repo meant to exemplify the simplest use case:  a repo with no metadata
	// that simply contains templates, grouped by runtime.  It therefore does
	// not have a manifest and the deafult name will therefore be the repo name
	uri := testRepoURI(RepositoriesTestRepo, t) // ./testdata/$RepositoriesTestRepo.git
	root, rm := mktemp(t)
	defer rm()

	client := fn.New(fn.WithRepositories(root))

	name, err := client.Repositories().Add("", uri)
	if err != nil {
		t.Fatal(err)
	}

	// The name returned should be the repo name
	if name != RepositoriesTestRepo {
		t.Fatalf("expected returned name '%v', got '%v'", RepositoriesTestRepo, name)
	}

	// The list of repositories should contain $RepositoriesTestRepo
	rr, err := client.Repositories().List()
	if err != nil {
		t.Fatal(err)
	}
	expected := []string{"default", RepositoriesTestRepo}
	if !reflect.DeepEqual(rr, expected) {
		t.Fatalf("Expected '%v', got %v", expected, rr)
	}
}

// TestRepositoriesAddDefaultNameFromManifest ensures that a repository with
// a manfest, where a name is specified, is used as the default when one is
// not explicitly specified.
func TestRepositoriesAddDefaultNameFromManifest(t *testing.T) {
	// repository-b is meant to exemplify the use case of a repository which
	// defines a custom language pack and makes full use of the manifest.yaml.
	// The manifest.yaml is included which specifies things like custom templates
	// location and (appropos to this test) a default name/
	uri := testRepoURI("repository-a", t) // ./testdata/repository-b.git
	root, rm := mktemp(t)
	defer rm()

	client := fn.New(fn.WithRepositories(root))

	name, err := client.Repositories().Add("", uri)
	if err != nil {
		t.Fatal(err)
	}

	// The name returned should be that defined in repository-b/manifest.yaml
	expectedName := "defaultName"
	if name != expectedName {
		t.Fatalf("expected returned name '%v', got '%v'", expectedName, name)
	}

	// The list should include the name
	rr, err := client.Repositories().List()
	if err != nil {
		t.Fatal(err)
	}
	expected := []string{"default", expectedName}
	if !reflect.DeepEqual(rr, expected) {
		t.Fatalf("Expected '%v', got %v", expected, rr)
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
	if _, err := client.Repositories().Add(name, uri); err != nil {
		t.Fatal(err)
	}
	if _, err := client.Repositories().Add(name, uri); err == nil {
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
	if _, err := client.Repositories().Add("foo", uri); err != nil {
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
	if _, err := client.Repositories().Add(name, uri); err != nil {
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
	_, err := client.Repositories().Add("newrepo", uri)
	if err != nil {
		t.Fatal(err)
	}

	// Get the newly added repo
	r, err := client.Repositories().Get("newrepo")
	if err != nil {
		t.Fatal(err)
	}

	// Assert it includes the correct URL
	if r.URL() != uri {
		t.Fatalf("expected repository URL '%v', got '%v'", uri, r.URL())
	}
}

// TestRepositoriesMissing ensures that a missing repositores directory
// does not cause an error (is treated as no repositories installed).
// This will change in an upcoming release where the repositories directory
// will be created at the config path if it does not exist, but this requires
// first moving the defaulting path logic from CLI into the client lib.
func TestRepositoriesMissing(t *testing.T) {
	root, rm := mktemp(t)
	defer rm()

	// Client with a repositories path which does not exit.
	repositories := filepath.Join(root, "repositories")
	client := fn.New(fn.WithRepositories(repositories))

	// Get all repositories
	_, err := client.Repositories().All()
	if err != nil {
		t.Fatal(err)
	}
}
