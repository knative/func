// +build integration

package function_test

import (
	"io/ioutil"
	"os"
	"reflect"
	"testing"
	"time"
	"context"

	boson "github.com/boson-project/func"
	"github.com/boson-project/func/buildpacks"
	"github.com/boson-project/func/docker"
	"github.com/boson-project/func/knative"
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
	DefaultRegistry = "localhost:5000/func"

	// DefaultNamespace for the underlying deployments.  Must be the same
	// as is set up and configured (see hack/configure.sh)
	DefaultNamespace = "func"
)

func TestList(t *testing.T) {
	verbose := true

	// Assemble
	lister, err := knative.NewLister(DefaultNamespace)
	if err != nil {
		t.Fatal(err)
	}
	client := boson.New(
		boson.WithLister(lister),
		boson.WithVerbose(verbose))

	// Act
	names, err := client.List()
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
	defer within(t, "testdata/example.com/testnew")()
	verbose := true

	client := newClient(verbose)

	// Act
	if err := client.New(boson.Function{Name: "testnew", Root: ".", Runtime: "go"}); err != nil {
		t.Fatal(err)
	}
	defer del(t, client, "testnew")

	// Assert
	items, err := client.List()
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
	mockIn, err := ioutil.TempFile("", "mockStdin")
	if err != nil {
		t.Fatal(err)
	}
	mockIn.WriteString("\n\n\n")
	mockIn.Close()
	defer os.Remove(mockIn.Name())

	oldStdin := os.Stdin
	os.Stdin, err = os.Open(mockIn.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		os.Stdin.Close()
		os.Stdin = oldStdin
	}()

	defer within(t, "testdata/example.com/deploy")()
	verbose := true

	client := newClient(verbose)

	if err := client.New(boson.Function{Name: "deploy", Root: ".", Runtime: "go"}); err != nil {
		t.Fatal(err)
	}
	defer del(t, client, "deploy")

	if err := client.Deploy(context.TODO(), "."); err != nil {
		t.Fatal(err)
	}
}

// TestRemove deletes
func TestRemove(t *testing.T) {
	defer within(t, "testdata/example.com/remove")()
	verbose := true

	client := newClient(verbose)

	if err := client.New(boson.Function{Name: "remove", Root: ".", Runtime: "go"}); err != nil {
		t.Fatal(err)
	}
	waitFor(t, client, "remove")

	if err := client.Remove(boson.Function{Name: "remove"}); err != nil {
		t.Fatal(err)
	}

	names, err := client.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(names) != 0 {
		t.Fatalf("Expected empty Functions list, got %v", names)
	}
}

// ***********
//   Helpers
// ***********

// newClient creates an instance of the func client whose concrete impls
// match those created by the kn func plugin CLI.
func newClient(verbose bool) *boson.Client {
	builder := buildpacks.NewBuilder()
	builder.Verbose = verbose

	pusher := docker.NewPusher()
	pusher.Verbose = verbose

	deployer, err := knative.NewDeployer(DefaultNamespace)
	if err != nil {
		panic(err) // TODO: remove error from deployer constructor
	}
	deployer.Verbose = verbose

	remover, err := knative.NewRemover(DefaultNamespace)
	if err != nil {
		panic(err) // TODO: remove error from remover constructor
	}
	remover.Verbose = verbose

	lister, err := knative.NewLister(DefaultNamespace)
	if err != nil {
		panic(err) // TODO: remove error from lister constructor
	}
	lister.Verbose = verbose

	return boson.New(
		boson.WithRegistry(DefaultRegistry),
		boson.WithVerbose(verbose),
		boson.WithBuilder(builder),
		boson.WithPusher(pusher),
		boson.WithDeployer(deployer),
		boson.WithRemover(remover),
		boson.WithLister(lister),
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
func del(t *testing.T, c *boson.Client, name string) {
	t.Helper()
	waitFor(t, c, name)
	if err := c.Remove(boson.Function{Name: name}); err != nil {
		t.Fatal(err)
	}
}

// waitFor the named Function to become available in List output.
// TODO: the API should be synchronous, but that depends first on
// Create returning the derived name such that we can bake polling in.
// Ideally the Boson provider's Creaet would be made syncrhonous.
func waitFor(t *testing.T, c *boson.Client, name string) {
	t.Helper()
	var pollInterval = 2 * time.Second

	for { // ever (i.e. defer to global test timeout)
		nn, err := c.List()
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

// Create the given directory, CD to it, and return a function which can be
// run in a defer statement to return to the original directory and cleanup.
// Note must be executed, not deferred itself
// NO:  defer within(t, "somedir")
// YES: defer within(t, "somedir")()
func within(t *testing.T, root string) func() {
	t.Helper()
	cwd := pwd(t)
	mkdir(t, root)
	cd(t, root)
	return func() {
		cd(t, cwd)
		rm(t, root)
	}
}

func pwd(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	return dir
}

func mkdir(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
}

func cd(t *testing.T, dir string) {
	t.Helper()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
}

func rm(t *testing.T, dir string) {
	t.Helper()
	if err := os.RemoveAll(dir); err != nil {
		t.Fatal(err)
	}
}

func touch(file string) {
	_, err := os.Stat(file)
	if os.IsNotExist(err) {
		f, err := os.Create(file)
		if err != nil {
			panic(err)
		}
		defer f.Close()
	}
	t := time.Now().Local()
	if err := os.Chtimes(file, t, t); err != nil {
		panic(err)
	}
}
