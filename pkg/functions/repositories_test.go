//go:build !integration
// +build !integration

package functions_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-cmp/cmp"

	fn "knative.dev/func/pkg/functions"
	. "knative.dev/func/pkg/testing"
)

// RepositoriesTestRepo is the general-purpose example repository for most
// test.  Others do eist with specific test requirements that are mutually
// exclusive, such as manifest differences, and are specified inline to their
// requisite test.
const RepositoriesTestRepo = "repository.git"

// TestRepositories_List ensures the base case of listing
// repositories without error in the default scenario of builtin only.
func TestRepositories_List(t *testing.T) {
	root, rm := Mktemp(t)
	defer rm()

	client := fn.New(fn.WithRepositoriesPath(root)) // Explicitly empty

	rr, err := client.Repositories().List()
	if err != nil {
		t.Fatal(err)
	}
	// Assert contains only the default repo
	if len(rr) != 1 && rr[0] != fn.DefaultRepositoryName {
		t.Fatalf("Expected repository list '[%v]', got %v", fn.DefaultRepositoryName, rr)
	}
}

// TestRepositories_GetInvalid ensures that attempting to get an invalid repo
// results in error.
func TestRepositories_GetInvalid(t *testing.T) {
	client := fn.New(fn.WithRepositoriesPath("testdata/repositories"))

	// invalid should error
	_, err := client.Repositories().Get("invalid")
	if err == nil {
		t.Fatal("did not receive expected error getting inavlid repository")
	}
}

// TestRepositories_Get ensures a repository can be accessed by name.
func TestRepositories_Get(t *testing.T) {
	client := fn.New(fn.WithRepositoriesPath("testdata/repositories"))

	// valid should not error
	repo, err := client.Repositories().Get("customTemplateRepo")
	if err != nil {
		t.Fatal(err)
	}

	// valid should have expected name
	if repo.Name != "customTemplateRepo" {
		t.Fatalf("Expected 'customTemplateRepo', got: %v", repo.Name)
	}
}

// TestRepositories_All ensures repos are returned from
// .All accessor.  Tests both builtin and buitlin+extensible cases.
func TestRepositories_All(t *testing.T) {
	uri := ServeRepo(RepositoriesTestRepo, t)
	root, rm := Mktemp(t)
	defer rm()

	client := fn.New(fn.WithRepositoriesPath(root))

	// Assert initially only the default is included
	rr, err := client.Repositories().All()
	if err != nil {
		t.Fatal(err)
	}
	if len(rr) != 1 && rr[0].Name != fn.DefaultRepositoryName {
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
		repositories[0].Name != fn.DefaultRepositoryName ||
		repositories[1].Name != "repository" {
		t.Fatal("Repositories list does not pass shallow repository membership check")
	}
}

// TestRepositories_Add checks basic adding of a repository by URI.
func TestRepositories_Add(t *testing.T) {
	uri := ServeRepo(RepositoriesTestRepo, t) // ./testdata/$RepositoriesTestRepo.git
	root, rm := Mktemp(t)                     // create and cd to a temp dir, returning path.
	defer rm()

	// Instantiate the client using the current temp directory as the
	// repositories' root location.
	client := fn.New(fn.WithRepositoriesPath(root))

	// Add the repository, explicitly specifying a name.  See other tests for
	// defaulting from repository names and manifest-defined name.
	if _, err := client.Repositories().Add("example", uri); err != nil {
		t.Fatal(err)
	}

	// Confirm the list now contains the name
	rr, err := client.Repositories().List()
	if err != nil {
		t.Fatal(err)
	}
	expected := []string{"default", "example"}
	if diff := cmp.Diff(expected, rr); diff != "" {
		t.Error("Repositories differed (-want, +got):", diff)
	}

	// assert a file exists at the location as well indicating it was added to
	// the filesystem, not just the list.
	if _, err := os.Stat(filepath.Join("example", "README.md")); os.IsNotExist(err) {
		t.Fatalf("Repository does not appear on disk as expected: %v", err)
	}
}

// TestRepositories_AddDefaultName ensures that repository name is optional,
// by default being set to the name of the repoisotory from the URI.
func TestRepositories_AddDeafultName(t *testing.T) {
	// The test repository is the "base case" repo, which is a manifestless
	// repo meant to exemplify the simplest use case:  a repo with no metadata
	// that simply contains templates, grouped by runtime.  It therefore does
	// not have a manifest and the default name will therefore be the repo name
	uri := ServeRepo(RepositoriesTestRepo, t) // ./testdata/$RepositoriesTestRepo.git
	root, rm := Mktemp(t)
	defer rm()

	client := fn.New(fn.WithRepositoriesPath(root))

	name, err := client.Repositories().Add("", uri)
	if err != nil {
		t.Fatal(err)
	}

	// The name returned should be the repo name
	if name != "repository" {
		t.Fatalf("expected returned name '%v', got '%v'", RepositoriesTestRepo, name)
	}

	// The list of repositories should contain $RepositoriesTestRepo
	rr, err := client.Repositories().List()
	if err != nil {
		t.Fatal(err)
	}
	expected := []string{"default", "repository"}
	if diff := cmp.Diff(expected, rr); diff != "" {
		t.Error("Repositories differed (-want, +got):", diff)
	}
}

// TestRepositories_AddWithManifest ensures that a repository with
// a manfest wherein a default name is specified, is used as the name for the
// added repository when a name is not explicitly specified.
func TestRepositories_AddWithManifest(t *testing.T) {
	// repository-b is meant to exemplify the use case of a repository which
	// defines a custom language pack and makes full use of the manifest.yaml.
	// The manifest.yaml is included which specifies things like custom templates
	// location and (appropos to this test) a default name/
	uri := ServeRepo("repository-a.git", t) // ./testdata/repository-a.git
	root, rm := Mktemp(t)
	defer rm()

	client := fn.New(fn.WithRepositoriesPath(root))

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
	if diff := cmp.Diff(expected, rr); diff != "" {
		t.Error("Repositories differed (-want, +got):", diff)
	}
}

// TestRepositories_AddExistingErrors ensures that adding a repository that
// already exists yields an error.
func TestRepositories_AddExistingErrors(t *testing.T) {
	uri := ServeRepo(RepositoriesTestRepo, t)
	root, rm := Mktemp(t) // create and cd to a temp dir, returning path.
	defer rm()

	// Instantiate the client using the current temp directory as the
	// repositories' root location.
	client := fn.New(fn.WithRepositoriesPath(root))

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

// TestRepositories_Rename ensures renaming a repository succeeds.
func TestRepositories_Rename(t *testing.T) {
	uri := ServeRepo(RepositoriesTestRepo, t)
	root, rm := Mktemp(t) // create and cd to a temp dir, returning path.
	defer rm()

	// Instantiate the client using the current temp directory as the
	// repositories' root location.
	client := fn.New(fn.WithRepositoriesPath(root))

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

// TestRepositories_Remove ensures that removing a repository by name
// removes it from the list and FS.
func TestRepositories_Remove(t *testing.T) {
	uri := ServeRepo(RepositoriesTestRepo, t) // ./testdata/repository.git
	root, rm := Mktemp(t)                     // create and cd to a temp dir
	defer rm()

	// Instantiate the client using the current temp directory as the
	// repositories' root location.
	client := fn.New(fn.WithRepositoriesPath(root))

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

// TestRepositories_URL ensures that a repository populates its URL member
// from the git repository's origin url (if it is a git repo and exists)
func TestRepositories_URL(t *testing.T) {
	uri := ServeRepo(RepositoriesTestRepo, t)
	root, rm := Mktemp(t)
	defer rm()

	client := fn.New(fn.WithRepositoriesPath(root))

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

	// Assert it includes the correct URL, including the refspec fragment
	if r.URL() != uri+"#master" {
		t.Fatalf("expected repository URL '%v#master', got '%v'", uri, r.URL())
	}
}

// TestRepositories_Missing ensures that a missing repositores directory
// does not cause an error unless it was explicitly set (zero value indicates
// no repos should be loaded from os).
// This may change in an upcoming release where the repositories directory
// will be created at the config path if it does not exist, but this requires
// first moving the defaulting path logic from CLI into the client lib.
func TestRepositories_Missing(t *testing.T) {
	// Client with no repositories path defined.
	client := fn.New()

	// Get all repositories
	_, err := client.Repositories().All()
	if err != nil {
		t.Fatal(err)
	}
}
