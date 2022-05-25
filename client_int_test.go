//go:build integration
// +build integration

package function_test

import (
	"context"
	"os"
	"reflect"
	"testing"
	"time"

	fn "knative.dev/kn-plugin-func"
	"knative.dev/kn-plugin-func/buildpacks"
	"knative.dev/kn-plugin-func/docker"
	"knative.dev/kn-plugin-func/knative"
	. "knative.dev/kn-plugin-func/testing"
)

/*
 NOTE:  Running integration tests locally requires a configured test cluster.
        Test failures may require manual removal of dangling resources.

 ## Integration Cluster

 These integration tests require a properly configured cluster,
 such as that which is setup and configured in CI (see .github/workflows).
 A local KinD cluster can be started via:
   ./hack/allocate.sh && ./hack/configure.sh

 ## Integration Testing

 These tests can be run via the make target:
   make test-integration
  or manually by specifying the tag
   go test -v -tags integration ./...

 ## Teardown and Cleanup

 Tests should clean up after themselves.  In the event of failures, one may
 need to manually remove files:
   rm -rf ./testdata/example.com
 The test cluster is not automatically removed, as it can be reused.  To remove:
   ./hack/delete.sh
*/

const (
	// DefaultRegistry must contain both the registry host and
	// registry namespace at this time.  This will likely be
	// split and defaulted to the forthcoming in-cluster registry.
	DefaultRegistry = "localhost:50000/func"

	// DefaultNamespace for the underlying deployments.  Must be the same
	// as is set up and configured (see hack/configure.sh)
	DefaultNamespace = "func"
)

func TestList(t *testing.T) {
	verbose := true

	// Assemble
	lister := knative.NewLister(DefaultNamespace, verbose)

	client := fn.New(
		fn.WithLister(lister),
		fn.WithVerbose(verbose))

	// Act
	names, err := client.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	// Assert
	if len(names) != 0 {
		t.Fatalf("Expected no Functions, got %v", names)
	}
}

// TestNew creates
func TestNew(t *testing.T) {
	defer Within(t, "testdata/example.com/testnew")()
	verbose := true

	client := newClient(verbose)

	// Act
	if err := client.New(context.Background(), fn.Function{Name: "testnew", Root: ".", Runtime: "go"}); err != nil {
		t.Fatal(err)
	}
	defer del(t, client, "testnew")

	// Assert
	items, err := client.List(context.Background())
	names := []string{}
	for _, item := range items {
		names = append(names, item.Name)
	}
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(names, []string{"testnew"}) {
		t.Fatalf("Expected function list ['testnew'], got %v", names)
	}
}

// TestDeploy updates
func TestDeploy(t *testing.T) {
	defer Within(t, "testdata/example.com/deploy")()
	verbose := true

	client := newClient(verbose)

	if err := client.New(context.Background(), fn.Function{Name: "deploy", Root: ".", Runtime: "go"}); err != nil {
		t.Fatal(err)
	}
	defer del(t, client, "deploy")

	if err := client.Deploy(context.Background(), "."); err != nil {
		t.Fatal(err)
	}
}

// TestRemove deletes
func TestRemove(t *testing.T) {
	defer Within(t, "testdata/example.com/remove")()
	verbose := true

	client := newClient(verbose)

	if err := client.New(context.Background(), fn.Function{Name: "remove", Root: ".", Runtime: "go"}); err != nil {
		t.Fatal(err)
	}
	waitFor(t, client, "remove")

	if err := client.Remove(context.Background(), fn.Function{Name: "remove"}, false); err != nil {
		t.Fatal(err)
	}

	names, err := client.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(names) != 0 {
		t.Fatalf("Expected empty Functions list, got %v", names)
	}
}

// TestRemoteRepositories ensures that initializing a Function
// defined in a remote repository finds the template, writes
// the expected files, and retains the expected modes.
// NOTE: this test only succeeds due to an override in
// templates' copyNode which forces mode 755 for directories.
// See https://github.com/go-git/go-git/issues/364
func TestRemoteRepositories(t *testing.T) {
	defer Within(t, "testdata/example.com/remote")()

	// Write the test template from the remote onto root
	client := fn.New(
		fn.WithRegistry(DefaultRegistry),
		fn.WithRepository("https://github.com/boson-project/test-templates"),
	)
	err := client.Create(fn.Function{
		Root:     ".",
		Runtime:  "runtime",
		Template: "template",
	})
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		Path string
		Perm uint32
		Dir  bool
	}{
		{Path: "file", Perm: 0644},
		{Path: "dir-a/file", Perm: 0644},
		{Path: "dir-b/file", Perm: 0644},
		{Path: "dir-b/executable", Perm: 0755},
		{Path: "dir-b", Perm: 0755},
		{Path: "dir-a", Perm: 0755},
	}

	// Note that .Perm() are used to only consider the least-signifigant 9 and
	// thus not have to consider the directory bit.
	for _, test := range tests {
		file, err := os.Stat(test.Path)
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("%04o repository/%v", file.Mode().Perm(), test.Path)
		if file.Mode().Perm() != os.FileMode(test.Perm) {
			t.Fatalf("expected 'repository/%v' to have mode %04o, got %04o", test.Path, test.Perm, file.Mode().Perm())
		}
	}
}

// ***********
//   Helpers
// ***********

// newClient creates an instance of the func client whose concrete impls
// match those created by the kn func plugin CLI.
func newClient(verbose bool) *fn.Client {
	builder := buildpacks.NewBuilder(buildpacks.WithVerbose(verbose))
	pusher := docker.NewPusher(docker.WithVerbose(verbose))
	deployer := knative.NewDeployer(DefaultNamespace, verbose)
	remover := knative.NewRemover(DefaultNamespace, verbose)
	lister := knative.NewLister(DefaultNamespace, verbose)

	return fn.New(
		fn.WithRegistry(DefaultRegistry),
		fn.WithVerbose(verbose),
		fn.WithBuilder(builder),
		fn.WithPusher(pusher),
		fn.WithDeployer(deployer),
		fn.WithRemover(remover),
		fn.WithLister(lister),
	)
}

// Del cleans up after a test by removing a function by name.
// (test fails if the named function does not exist)
//
// Intended to be run in a defer statement immediately after creation, del
// works around the asynchronicity of the underlying platform's creation
// step by polling the provider until the names function becomes available
// (or the test times out), before firing off a deletion request.
// Of course, ideally this would be replaced by the use of a synchronous
// method, or at a minimum a way to register a callback/listener for the
// creation event.  This is what we have for now, and the show must go on.
func del(t *testing.T, c *fn.Client, name string) {
	t.Helper()
	waitFor(t, c, name)
	if err := c.Remove(context.Background(), fn.Function{Name: name}, false); err != nil {
		t.Fatal(err)
	}
}

// waitFor the named Function to become available in List output.
// TODO: the API should be synchronous, but that depends first on
// Create returning the derived name such that we can bake polling in.
// Ideally the Boson provider's Creaet would be made syncrhonous.
func waitFor(t *testing.T, c *fn.Client, name string) {
	t.Helper()
	var pollInterval = 2 * time.Second

	for { // ever (i.e. defer to global test timeout)
		nn, err := c.List(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		for _, n := range nn {
			if n.Name == name {
				return
			}
		}
		time.Sleep(pollInterval)
	}
}
