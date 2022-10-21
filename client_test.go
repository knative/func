//go:build !integration
// +build !integration

package function_test

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	fn "knative.dev/func"
	"knative.dev/func/builders"
	"knative.dev/func/mock"
	. "knative.dev/func/testing"
)

const (
	// TestRegistry for calculating destination image during tests.
	// Will be optional once we support in-cluster container registries
	// by default.  See TestRegistryRequired for details.
	TestRegistry = "example.com/alice"

	// TestRuntime is currently Go, the "reference implementation" and is
	// used for verifying functionality that should be runtime agnostic.
	TestRuntime = "go"
)

// TestClient_New function completes without error using defaults and zero values.
// New is the superset of creating a new fully deployed function, and
// thus implicitly tests Create, Build and Deploy, which are exposed
// by the client API for those who prefer manual transmissions.
func TestClient_New(t *testing.T) {
	root := "testdata/example.com/testNew"
	defer Using(t, root)()

	client := fn.New(fn.WithRegistry(TestRegistry), fn.WithVerbose(true))

	if err := client.New(context.Background(), fn.Function{Root: root, Runtime: TestRuntime}); err != nil {
		t.Fatal(err)
	}
}

// TestClient_New_RuntimeRequired ensures that the the runtime is an expected value.
func TestClient_New_RuntimeRequired(t *testing.T) {
	// Create a root for the new function
	root := "testdata/example.com/testRuntimeRequired"
	defer Using(t, root)()

	client := fn.New(fn.WithRegistry(TestRegistry))

	// Create a new function at root with all defaults.
	err := client.New(context.Background(), fn.Function{Root: root})
	if err == nil {
		t.Fatalf("did not receive error creating a function without specifying runtime")
	}
}

// TestClient_New_NameDefaults ensures that a newly created function has its name defaulted
// to a name which can be derived from the last part of the given root path.
func TestClient_New_NameDefaults(t *testing.T) {
	root := "testdata/example.com/testNameDefaults"
	defer Using(t, root)()

	client := fn.New(fn.WithRegistry(TestRegistry))

	f := fn.Function{
		Runtime: TestRuntime,
		// NO NAME
		Root: root,
	}

	if err := client.New(context.Background(), f); err != nil {
		t.Fatal(err)
	}

	f, err := fn.NewFunction(root)
	if err != nil {
		t.Fatal(err)
	}

	expected := "testNameDefaults"
	if f.Name != expected {
		t.Fatalf("name was not defaulted. expected '%v' got '%v'", expected, f.Name)
	}
}

// TestClient_New_WritesTemplate ensures the config file and files from the template
// are written on new.
func TestClient_New_WritesTemplate(t *testing.T) {
	root := "testdata/example.com/testWritesTemplate"
	defer Using(t, root)()

	client := fn.New(fn.WithRegistry(TestRegistry))

	if err := client.New(context.Background(), fn.Function{Runtime: TestRuntime, Root: root}); err != nil {
		t.Fatal(err)
	}

	// Assert the standard config file was written
	if _, err := os.Stat(filepath.Join(root, fn.FunctionFile)); os.IsNotExist(err) {
		t.Fatalf("Initialize did not result in '%v' being written to '%v'", fn.FunctionFile, root)
	}

	// Assert a file from the template was written
	if _, err := os.Stat(filepath.Join(root, "README.md")); os.IsNotExist(err) {
		t.Fatalf("Initialize did not result in '%v' being written to '%v'", fn.FunctionFile, root)
	}
}

// TestClient_New_ExtantAborts ensures that a directory which contains an extant
// function does not reinitialize.
func TestClient_New_ExtantAborts(t *testing.T) {
	root := "testdata/example.com/testExtantAborts"
	defer Using(t, root)()

	client := fn.New(fn.WithRegistry(TestRegistry))

	// First .New should succeed...
	if err := client.New(context.Background(), fn.Function{Runtime: TestRuntime, Root: root}); err != nil {
		t.Fatal(err)
	}

	// Calling again should abort...
	if err := client.New(context.Background(), fn.Function{Root: root}); err == nil {
		t.Fatal("error expected initilizing a path already containing an initialized function")
	}
}

// TestClient_New_NonemptyAborts ensures that a directory which contains any
// (visible) files aborts.
func TestClient_New_NonemptyAborts(t *testing.T) {
	root := "testdata/example.com/testNonemptyAborts"
	defer Using(t, root)()

	client := fn.New(fn.WithRegistry(TestRegistry))

	// Write a visible file which should cause an abort
	visibleFile := filepath.Join(root, "file.txt")
	if err := os.WriteFile(visibleFile, []byte{}, 0644); err != nil {
		t.Fatal(err)
	}

	// Creation should abort due to the visible file
	if err := client.New(context.Background(), fn.Function{Root: root}); err == nil {
		t.Fatal("error expected initilizing a function in a nonempty directory")
	}
}

// TestClient_New_HiddenFilesIgnored ensures that initializing in a directory that
// only contains hidden files does not error, protecting against the naive
// implementation of aborting initialization if any files exist, which would
// break functions tracked in source control (.git), or when used in
// conjunction with other tools (.envrc, etc)
func TestClient_New_HiddenFilesIgnored(t *testing.T) {
	// Create a directory for the function
	root := "testdata/example.com/testHiddenFilesIgnored"
	defer Using(t, root)()

	client := fn.New(fn.WithRegistry(TestRegistry))

	// Create a hidden file that should be ignored.
	hiddenFile := filepath.Join(root, ".envrc")
	if err := os.WriteFile(hiddenFile, []byte{}, 0644); err != nil {
		t.Fatal(err)
	}

	// Should succeed without error, ignoring the hidden file.
	if err := client.New(context.Background(), fn.Function{Runtime: TestRuntime, Root: root}); err != nil {
		t.Fatal(err)
	}
}

// TestClient_New_RepositoriesExtensible ensures that templates are extensible
// using a custom path to template repositories on disk. The custom repositories
// location is not defined herein but expected to be provided because, for
// example, a CLI may want to use XDG_CONFIG_HOME.  Assuming a repository path
// $FUNC_REPOSITORIES_PATH, a Go template named 'json' which is provided in the
// repository 'boson', would be expected to be in the location:
// $FUNC_REPOSITORIES_PATH/boson/go/json
// See the CLI for full details, but a standard default location is
// $HOME/.config/func/repositories/boson/go/json
func TestClient_New_RepositoriesExtensible(t *testing.T) {
	root := "testdata/example.com/testRepositoriesExtensible"
	defer Using(t, root)()

	client := fn.New(
		fn.WithRepositoriesPath("testdata/repositories"),
		fn.WithRegistry(TestRegistry))

	// Create a function specifying a template which only exists in the extensible set
	if err := client.New(context.Background(), fn.Function{Root: root, Runtime: "test", Template: "customTemplateRepo/tplc"}); err != nil {
		t.Fatal(err)
	}

	// Ensure that a file from that only exists in that template set was actually written 'json.js'
	if _, err := os.Stat(filepath.Join(root, "customtpl.txt")); os.IsNotExist(err) {
		t.Fatalf("Initializing a custom did not result in customtpl.txt being written to '%v'", root)
	} else if err != nil {
		t.Fatal(err)
	}
}

// TestRuntime_New_RuntimeNotFoundError generates an error when the provided
// runtime is not fo0und (embedded default repository).
func TestClient_New_RuntimeNotFoundError(t *testing.T) {
	root := "testdata/example.com/testRuntimeNotFound"
	defer Using(t, root)()

	client := fn.New(fn.WithRegistry(TestRegistry))

	// creating a function with an unsupported runtime should bubble
	// the error generated by the underlying template initializer.
	err := client.New(context.Background(), fn.Function{Root: root, Runtime: "invalid"})
	if !errors.Is(err, fn.ErrRuntimeNotFound) {
		t.Fatalf("Expected ErrRuntimeNotFound, got %T", err)
	}
}

// TestClient_New_RuntimeNotFoundCustom ensures that the correct error is returned
// when the requested runtime is not found in a given custom repository
func TestClient_New_RuntimeNotFoundCustom(t *testing.T) {
	root := "testdata/example.com/testRuntimeNotFoundCustom"
	defer Using(t, root)()

	// Create a new client with path to the extensible templates
	client := fn.New(
		fn.WithRepositoriesPath("testdata/repositories"),
		fn.WithRegistry(TestRegistry))

	// Create a function specifying a runtime, 'python' that does not exist
	// in the custom (testdata) repository but does in the embedded.
	f := fn.Function{Root: root, Runtime: "python", Template: "customTemplateRepo/event"}

	// creating should error as runtime not found
	err := client.New(context.Background(), f)
	if !errors.Is(err, fn.ErrRuntimeNotFound) {
		t.Fatalf("Expected ErrRuntimeNotFound, got %v", err)
	}
}

// TestClient_New_TemplateNotFoundError generates an error (embedded default repository).
func TestClient_New_TemplateNotFoundError(t *testing.T) {
	root := "testdata/example.com/testTemplateNotFound"
	defer Using(t, root)()

	client := fn.New(fn.WithRegistry(TestRegistry))

	// creating a function with an unsupported runtime should bubble
	// the error generated by the unsderlying template initializer.
	f := fn.Function{Root: root, Runtime: "go", Template: "invalid"}
	err := client.New(context.Background(), f)
	if !errors.Is(err, fn.ErrTemplateNotFound) {
		t.Fatalf("Expected ErrTemplateNotFound, got %v", err)
	}
}

// TestClient_New_TemplateNotFoundCustom ensures that the correct error is returned
// when the requested template is not found in the given custom repository.
func TestClient_New_TemplateNotFoundCustom(t *testing.T) {
	root := "testdata/example.com/testTemplateNotFoundCustom"
	defer Using(t, root)()

	// Create a new client with path to extensible templates
	client := fn.New(
		fn.WithRepositoriesPath("testdata/repositories"),
		fn.WithRegistry(TestRegistry))

	// An invalid template, but a valid custom provider
	f := fn.Function{Root: root, Runtime: "test", Template: "customTemplateRepo/invalid"}

	// Creation should generate the correct error of template not being found.
	err := client.New(context.Background(), f)
	if !errors.Is(err, fn.ErrTemplateNotFound) {
		t.Fatalf("Expected ErrTemplateNotFound, got %v", err)
	}
}

// TestClient_New_Named ensures that an explicitly passed name is used in leau of the
// path derived name when provided, and persists through instantiations.
func TestClient_New_Named(t *testing.T) {
	// Explicit name to use
	name := "service.example.com"

	// Path which would derive to testWithHame.example.com were it not for the
	// explicitly provided name.
	root := "testdata/example.com/testNamed"
	defer Using(t, root)()

	client := fn.New(fn.WithRegistry(TestRegistry))

	if err := client.New(context.Background(), fn.Function{Runtime: TestRuntime, Root: root, Name: name}); err != nil {
		t.Fatal(err)
	}

	f, err := fn.NewFunction(root)
	if err != nil {
		t.Fatal(err)
	}

	if f.Name != name {
		t.Fatalf("expected name '%v' got '%v", name, f.Name)
	}
}

// TestClient_New_RegistryRequired ensures that a registry is required, and is
// prepended with the DefaultRegistry if a single token.
// Registry is the namespace at the container image registry.
// If not prepended with the registry, it will be defaulted:
// Examples:  "docker.io/alice"
//
//	"quay.io/bob"
//	"charlie" (becomes [DefaultRegistry]/charlie
//
// At this time a registry namespace is required as we rely on a third-party
// registry in all cases.  When we support in-cluster container registries,
// this configuration parameter will become optional.
func TestClient_New_RegistryRequired(t *testing.T) {
	// Create a root for the function
	root := "testdata/example.com/testRegistryRequired"
	defer Using(t, root)()

	client := fn.New()
	var err error
	if err = client.New(context.Background(), fn.Function{Root: root}); err == nil {
		t.Fatal("did not receive expected error creating a function without specifying Registry")
	}
}

// TestClient_New_ImageNamePopulated ensures that the full image (tag) of the
// resultant OCI container is populated.
func TestClient_New_ImageNamePopulated(t *testing.T) {
	// Create the root function directory
	root := "testdata/example.com/testDeriveImage"
	defer Using(t, root)()

	// Create the function which calculates fields such as name and image.
	client := fn.New(fn.WithRegistry(TestRegistry))
	if err := client.New(context.Background(), fn.Function{Runtime: TestRuntime, Root: root}); err != nil {
		t.Fatal(err)
	}

	// Load the function with the now-populated fields.
	f, err := fn.NewFunction(root)
	if err != nil {
		t.Fatal(err)
	}

	// This opaque-box unit test ensures NewImageTag is invoked and applied.
	// See the Test_NewImageTag clear-box unit test for an in-depth exploration of
	// how the values of image and registry are treated to create, by default:
	//   [Default Registry]/[Registry Namespace]/[Service Name]:latest
	imageTag, err := f.ImageName()
	if err != nil {
		t.Fatal(err)
	}
	if f.Image != imageTag {
		t.Fatalf("expected image '%v' got '%v'", imageTag, f.Image)
	}
}

// TestCleint_New_ImageRegistryDefaults ensures that a Registry which does not have
// a registry prefix has the DefaultRegistry prepended.
// For example "alice" becomes "docker.io/alice"
func TestClient_New_ImageRegistryDefaults(t *testing.T) {
	// Create the root function directory
	root := "testdata/example.com/testDeriveImageDefaultRegistry"
	defer Using(t, root)()

	// Create the function which calculates fields such as name and image.
	// Rather than use TestRegistry, use a single-token name and expect
	// the DefaultRegistry to be prepended.
	client := fn.New(fn.WithRegistry("alice"))
	if err := client.New(context.Background(), fn.Function{Runtime: TestRuntime, Root: root}); err != nil {
		t.Fatal(err)
	}

	// Load the function with the now-populated fields.
	f, err := fn.NewFunction(root)
	if err != nil {
		t.Fatal(err)
	}

	// Expected image is [DefaultRegistry]/[namespace]/[servicename]:latest
	expected := fn.DefaultRegistry + "/alice/" + f.Name + ":latest"
	if f.Image != expected {
		t.Fatalf("expected image '%v' got '%v'", expected, f.Image)
	}
}

// TestClient_New_Delegation ensures that Create invokes each of the individual
// subcomponents via delegation through Build, Push and
// Deploy (and confirms expected fields calculated).
func TestClient_New_Delegation(t *testing.T) {
	var (
		root          = "testdata/example.com/testNewDelegates" // .. in which to initialize
		expectedName  = "testNewDelegates"                      // expected to be derived
		expectedImage = "example.com/alice/testNewDelegates:latest"
		builder       = mock.NewBuilder()
		pusher        = mock.NewPusher()
		deployer      = mock.NewDeployer()
	)

	// Create a directory for the test
	defer Using(t, root)()

	// Create a client with mocks for each of the subcomponents.
	client := fn.New(
		fn.WithRegistry(TestRegistry),
		fn.WithBuilder(builder),   // builds an image
		fn.WithPusher(pusher),     // pushes images to a registry
		fn.WithDeployer(deployer), // deploys images as a running service
	)

	// Register function delegates on the mocks which validate assertions
	// -------------

	// The builder should be invoked with a path to a function project's source
	// An example image name is returned.
	builder.BuildFn = func(f fn.Function) error {
		if root != f.Root {
			t.Fatalf("builder expected path %v, got '%v'", root, f.Root)
		}
		return nil
	}

	pusher.PushFn = func(f fn.Function) (string, error) {
		if f.Image != expectedImage {
			t.Fatalf("pusher expected image '%v', got '%v'", expectedImage, f.Image)
		}
		return "", nil
	}

	deployer.DeployFn = func(f fn.Function) error {
		if f.Name != expectedName {
			t.Fatalf("deployer expected name '%v', got '%v'", expectedName, f.Name)
		}
		if f.Image != expectedImage {
			t.Fatalf("deployer expected image '%v', got '%v'", expectedImage, f.Image)
		}
		return nil
	}

	// Invocation
	// -------------

	// Invoke the creation, triggering the function delegates, and
	// perform follow-up assertions that the functions were indeed invoked.
	if err := client.New(context.Background(), fn.Function{Runtime: TestRuntime, Root: root}); err != nil {
		t.Fatal(err)
	}

	// Confirm that each delegate was invoked.
	if !builder.BuildInvoked {
		t.Fatal("builder was not invoked")
	}
	if !pusher.PushInvoked {
		t.Fatal("pusher was not invoked")
	}
	if !deployer.DeployInvoked {
		t.Fatal("deployer was not invoked")
	}
}

// TestClient_Run ensures that the runner is invoked with the absolute path requested.
// Implicitly checks that the stop fn returned also is respected.
func TestClient_Run(t *testing.T) {
	// Create the root function directory
	root := "testdata/example.com/testRun"
	defer Using(t, root)()

	// client with the mock runner and the new test function
	runner := mock.NewRunner()
	client := fn.New(fn.WithRegistry(TestRegistry), fn.WithRunner(runner))
	if err := client.New(context.Background(), fn.Function{Runtime: TestRuntime, Root: root}); err != nil {
		t.Fatal(err)
	}

	// Run the newly created function
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	job, err := client.Run(ctx, root)
	if err != nil {
		t.Fatal(err)
	}
	defer job.Stop()

	// Assert the runner was invoked, and with the expected root.
	if !runner.RunInvoked {
		t.Fatal("run did not invoke the runner")
	}
	if runner.RootRequested != root {
		t.Fatalf("expected path '%v', got '%v'", root, runner.RootRequested)
	}
}

// TestClient_Run_DataDir ensures that when a function is created, it also
// includes a .func (runtime data) directory which is registered as ignored for
// functions which will be tracked in git source control.
// Note that this test is somewhat testing an implementation detail of `.Run(`
// (it writes runtime data to files in .func) but since the feature of adding
// .func to .gitignore is an important externally visible "feature", an explicit
// test is warranted.
func TestClient_Run_DataDir(t *testing.T) {
	root := "testdata/example.com/testRunDataDir"
	defer Using(t, root)()

	// Create a function at root
	client := fn.New(fn.WithRegistry(TestRegistry))
	if err := client.New(context.Background(), fn.Function{Root: root, Runtime: TestRuntime}); err != nil {
		t.Fatal(err)
	}

	// Assert the directory exists
	if _, err := os.Stat(filepath.Join(root, fn.RunDataDir)); os.IsNotExist(err) {
		t.Fatal(err)
	}

	// Assert that .gitignore was also created and includes an ignore directove
	// for the .func directory
	if _, err := os.Stat(filepath.Join(root, ".gitignore")); os.IsNotExist(err) {
		t.Fatal(err)
	}

	// Assert that .func is ignored
	file, err := os.Open(filepath.Join(root, ".gitignore"))
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()

	// Assert the directive exists
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		if scanner.Text() == "/"+fn.RunDataDir {
			return // success
		}
	}
	t.Errorf(".gitignore does not include '/%v' ignore directive", fn.RunDataDir)
}

// TestClient_Update ensures that updating invokes the build/push/deploy
// process, erroring if run on a directory uncreated.
func TestClient_Update(t *testing.T) {
	var (
		root          = "testdata/example.com/testUpdate"
		expectedName  = "testUpdate"
		expectedImage = "example.com/alice/testUpdate:latest"
		builder       = mock.NewBuilder()
		pusher        = mock.NewPusher()
		deployer      = mock.NewDeployerWithResult(&fn.DeploymentResult{
			Status:    fn.Deployed,
			URL:       "example.com",
			Namespace: "test-ns",
		})
		deployerUpdated = mock.NewDeployerWithResult(&fn.DeploymentResult{
			Status:    fn.Updated,
			URL:       "example.com",
			Namespace: "test-ns",
		})
	)

	// Create the root function directory
	defer Using(t, root)()

	// A client with mocks whose implementaton will validate input.
	client := fn.New(
		fn.WithRegistry(TestRegistry),
		fn.WithBuilder(builder),
		fn.WithPusher(pusher),
		fn.WithDeployer(deployer))

	// create the new function which will be updated
	if err := client.New(context.Background(), fn.Function{Runtime: TestRuntime, Root: root}); err != nil {
		t.Fatal(err)
	}

	// Builder whose implementation verifies the expected root
	builder.BuildFn = func(f fn.Function) error {
		rootPath, err := filepath.Abs(root)
		if err != nil {
			t.Fatal(err)
		}
		if f.Root != rootPath {
			t.Fatalf("builder expected path %v, got '%v'", rootPath, f.Root)
		}
		return nil
	}

	// Pusher whose implementaiton verifies the expected image
	pusher.PushFn = func(f fn.Function) (string, error) {
		if f.Image != expectedImage {
			t.Fatalf("pusher expected image '%v', got '%v'", expectedImage, f.Image)
		}
		// image of given name wouold be pushed to the configured registry.
		return "", nil
	}

	// Update whose implementaiton verifed the expected name and image
	deployer.DeployFn = func(f fn.Function) error {
		if f.Name != expectedName {
			t.Fatalf("updater expected name '%v', got '%v'", expectedName, f.Name)
		}
		if f.Image != expectedImage {
			t.Fatalf("updater expected image '%v', got '%v'", expectedImage, f.Image)
		}
		return nil
	}

	// Invoke the creation, triggering the function delegates, and
	// perform follow-up assertions that the functions were indeed invoked.
	if err := client.Deploy(context.Background(), root); err != nil {
		t.Fatal(err)
	}

	if !builder.BuildInvoked {
		t.Fatal("builder was not invoked")
	}
	if !pusher.PushInvoked {
		t.Fatal("pusher was not invoked")
	}
	if !deployer.DeployInvoked {
		t.Fatal("deployer was not invoked")
	}

	client = fn.New(
		fn.WithRegistry(TestRegistry),
		fn.WithBuilder(builder),
		fn.WithPusher(pusher),
		fn.WithDeployer(deployerUpdated))

	// Invoke the update, triggering the function delegates, and
	// perform follow-up assertions that the functions were indeed invoked during the update.
	if err := client.Deploy(context.Background(), root); err != nil {
		t.Fatal(err)
	}

	if !builder.BuildInvoked {
		t.Fatal("builder was not invoked")
	}
	if !pusher.PushInvoked {
		t.Fatal("pusher was not invoked")
	}
	if !deployerUpdated.DeployInvoked {
		t.Fatal("deployer was not invoked")
	}
}

// TestClient_Deploy_RegistryUpdate ensures that deploying a Function updates
// its image member on initial deploy, and on subsequent deploys only
// if reset to it zero value.
func TestClient_Deploy_RegistryUpdate(t *testing.T) {
	root, rm := Mktemp(t)
	defer rm()
	client := fn.New(fn.WithRegistry("example.com/alice"))

	// New runs build and deploy, thus the initial instantiation should result in
	// the member being populated from the client's registry and function name.
	if err := client.New(context.Background(), fn.Function{Runtime: "go", Name: "f", Root: root}); err != nil {
		t.Fatal(err)
	}
	f, err := fn.NewFunction(root)
	if err != nil {
		t.Fatal(err)
	}
	if f.Image != "example.com/alice/f:latest" {
		t.Error("image name was not initially set")
	}

	// Updating the registry and performing a subsequent update should not result
	// in the image member being updated to the new value: registry is only used
	// when calculating a nonexistent value
	f.Registry = "example.com/bob"
	if err := f.Write(); err != nil {
		t.Fatal(err)
	}
	if err := client.Build(context.Background(), root); err != nil {
		t.Fatal(err)
	}
	if err := client.Deploy(context.Background(), root); err != nil {
		t.Fatal(err)
	}
	expected := "example.com/alice/f:latest"
	f, err = fn.NewFunction(root) // reload and check
	if err != nil {
		t.Fatal(err)
	}
	if f.Image != expected { // NOT changed to bob
		t.Errorf("expected image name to stay '%v' and not be updated, but got '%v'", expected, f.Image)
	}

	// Reset the value of .Image to default "" and ensure this triggers recalc.
	f.Image = ""
	f.Registry = "example.com/bob"
	if err := f.Write(); err != nil {
		t.Fatal(err)
	}
	if err := client.Build(context.Background(), root); err != nil {
		t.Fatal(err)
	}
	if err := client.Deploy(context.Background(), root); err != nil {
		t.Fatal(err)
	}
	expected = "example.com/bob/f:latest"
	f, err = fn.NewFunction(root)
	if err != nil {
		t.Fatal(err)
	}
	if f.Image != expected { // DOES change to bob
		t.Errorf("expected image name to stay '%v' and not be updated, but got '%v'", expected, f.Image)

	}
}

// TestClient_Remove_ByPath ensures that the remover is invoked to remove
// the function with the name of the function at the provided root.
func TestClient_Remove_ByPath(t *testing.T) {
	var (
		root         = "testdata/example.com/testRemoveByPath"
		expectedName = "testRemoveByPath"
		remover      = mock.NewRemover()
	)

	defer Using(t, root)()

	client := fn.New(
		fn.WithRegistry(TestRegistry),
		fn.WithRemover(remover))

	if err := client.New(context.Background(), fn.Function{Runtime: TestRuntime, Root: root}); err != nil {
		t.Fatal(err)
	}

	remover.RemoveFn = func(name string) error {
		if name != expectedName {
			t.Fatalf("Expected to remove '%v', got '%v'", expectedName, name)
		}
		return nil
	}

	if err := client.Remove(context.Background(), fn.Function{Root: root}, false); err != nil {
		t.Fatal(err)
	}

	if !remover.RemoveInvoked {
		t.Fatal("remover was not invoked")
	}

}

// TestClient_Remove_DeleteAll ensures that the remover is invoked to remove
// and that dependent resources are removed as well -> pipeline provider is invoked
// the function with the name of the function at the provided root.
func TestClient_Remove_DeleteAll(t *testing.T) {
	var (
		root              = "testdata/example.com/testRemoveDeleteAll"
		expectedName      = "testRemoveDeleteAll"
		remover           = mock.NewRemover()
		pipelinesProvider = mock.NewPipelinesProvider()
		deleteAll         = true
	)

	defer Using(t, root)()

	client := fn.New(
		fn.WithRegistry(TestRegistry),
		fn.WithRemover(remover),
		fn.WithPipelinesProvider(pipelinesProvider))

	if err := client.New(context.Background(), fn.Function{Runtime: TestRuntime, Root: root}); err != nil {
		t.Fatal(err)
	}

	remover.RemoveFn = func(name string) error {
		if name != expectedName {
			t.Fatalf("Expected to remove '%v', got '%v'", expectedName, name)
		}
		return nil
	}

	if err := client.Remove(context.Background(), fn.Function{Root: root}, deleteAll); err != nil {
		t.Fatal(err)
	}

	if !remover.RemoveInvoked {
		t.Fatal("remover was not invoked")
	}

	if !pipelinesProvider.RemoveInvoked {
		t.Fatal("pipelinesprovider was not invoked")
	}

}

// TestClient_Remove_Dont_DeleteAll ensures that the remover is invoked to remove
// and that dependent resources are not removed as well -> pipeline provider not is invoked
// the function with the name of the function at the provided root.
func TestClient_Remove_Dont_DeleteAll(t *testing.T) {
	var (
		root              = "testdata/example.com/testRemoveDontDeleteAll"
		expectedName      = "testRemoveDontDeleteAll"
		remover           = mock.NewRemover()
		pipelinesProvider = mock.NewPipelinesProvider()
		deleteAll         = false
	)

	defer Using(t, root)()

	client := fn.New(
		fn.WithRegistry(TestRegistry),
		fn.WithRemover(remover),
		fn.WithPipelinesProvider(pipelinesProvider))

	if err := client.New(context.Background(), fn.Function{Runtime: TestRuntime, Root: root}); err != nil {
		t.Fatal(err)
	}

	remover.RemoveFn = func(name string) error {
		if name != expectedName {
			t.Fatalf("Expected to remove '%v', got '%v'", expectedName, name)
		}
		return nil
	}

	if err := client.Remove(context.Background(), fn.Function{Root: root}, deleteAll); err != nil {
		t.Fatal(err)
	}

	if !remover.RemoveInvoked {
		t.Fatal("remover was not invoked")
	}

	if pipelinesProvider.RemoveInvoked {
		t.Fatal("pipelinesprovider was invoked, but should not")
	}

}

// TestClient_Remove_ByName ensures that the remover is invoked to remove the function
// of the name provided, with precidence over a provided root path.
func TestClient_Remove_ByName(t *testing.T) {
	var (
		root         = "testdata/example.com/testRemoveByName"
		expectedName = "explicitName.example.com"
		remover      = mock.NewRemover()
	)

	defer Using(t, root)()

	client := fn.New(
		fn.WithRegistry(TestRegistry),
		fn.WithRemover(remover))

	if err := client.Create(fn.Function{Runtime: TestRuntime, Root: root}); err != nil {
		t.Fatal(err)
	}

	remover.RemoveFn = func(name string) error {
		if name != expectedName {
			t.Fatalf("Expected to remove '%v', got '%v'", expectedName, name)
		}
		return nil
	}

	// Run remove with only a name
	if err := client.Remove(context.Background(), fn.Function{Name: expectedName}, false); err != nil {
		t.Fatal(err)
	}

	// Run remove with a name and a root, which should be ignored in favor of the name.
	if err := client.Remove(context.Background(), fn.Function{Name: expectedName, Root: root}, false); err != nil {
		t.Fatal(err)
	}

	if !remover.RemoveInvoked {
		t.Fatal("remover was not invoked")
	}
}

// TestClient_Remove_UninitializedFails ensures that removing a function
// by path only (no name) fails unless the function has been initialized.  I.e.
// the name will not be derived from path and the function removed by this
// derived name; which could be unexpected and destructive.
func TestClient_Remove_UninitializedFails(t *testing.T) {
	var (
		root    = "testdata/example.com/testRemoveUninitializedFails"
		remover = mock.NewRemover()
	)
	defer Using(t, root)()

	// remover fails if invoked
	remover.RemoveFn = func(name string) error {
		return fmt.Errorf("remove invoked for unitialized function %v", name)
	}

	// Instantiate the client with the failing remover.
	client := fn.New(
		fn.WithRegistry(TestRegistry),
		fn.WithRemover(remover))

	// Attempt to remove by path (uninitialized), expecting an error.
	if err := client.Remove(context.Background(), fn.Function{Root: root}, false); err == nil {
		t.Fatalf("did not received expeced error removing an uninitialized func")
	}
}

// TestClient_List merely ensures that the client invokes the configured lister.
func TestClient_List(t *testing.T) {
	lister := mock.NewLister()

	client := fn.New(fn.WithLister(lister)) // lists deployed functions.

	if _, err := client.List(context.Background()); err != nil {
		t.Fatal(err)
	}

	if !lister.ListInvoked {
		t.Fatal("list did not invoke lister implementation")
	}
}

// TestClient_List_OutsideRoot ensures that a call to a function (in this case list)
// that is not contextually dependent on being associated with a function,
// can be run from anywhere, thus ensuring that the client itself makes
// a distinction between function-scoped methods and not.
func TestClient_List_OutsideRoot(t *testing.T) {
	lister := mock.NewLister()

	// Instantiate in the current working directory, with no name.
	client := fn.New(fn.WithLister(lister))

	if _, err := client.List(context.Background()); err != nil {
		t.Fatal(err)
	}

	if !lister.ListInvoked {
		t.Fatal("list did not invoke lister implementation")
	}
}

// TestClient_Deploy_Image ensures that initially the function's image
// member has no value (not initially deployed); the value is populated
// upon deployment with a value derived from the function's name and currently
// effective client registry; that the value of f.Image will take precidence
// over .Registry, which is used to calculate a default value for image.
func TestClient_Deploy_Image(t *testing.T) {
	root, rm := Mktemp(t)
	defer rm()

	client := fn.New(
		fn.WithBuilder(mock.NewBuilder()),
		fn.WithDeployer(mock.NewDeployer()),
		fn.WithRegistry("example.com/alice"))

	err := client.Create(fn.Function{Name: "myfunc", Runtime: "go", Root: root})
	if err != nil {
		t.Fatal(err)
	}

	f, err := fn.NewFunction(root)
	if err != nil {
		t.Fatal(err)
	}

	// Upon initial creation, the value of .Image is empty
	if f.Image != "" {
		t.Fatalf("new function should have no image, got '%v'", f.Image)
	}

	// Upon deployment, the function should be populated;
	if err = client.Build(context.Background(), root); err != nil {
		t.Fatal(err)
	}
	if err = client.Deploy(context.Background(), root); err != nil {
		t.Fatal(err)
	}
	f, err = fn.NewFunction(root)
	if err != nil {
		t.Fatal(err)
	}
	expected := "example.com/alice/myfunc:latest"
	if f.Image != expected {
		t.Fatalf("expected image '%v', got '%v'", expected, f.Image)
	}
	expected = "example.com/alice"
	if f.Registry != "example.com/alice" {
		t.Fatalf("expected registry '%v', got '%v'", expected, f.Registry)
	}

	// The value of .Image always takes precidence
	f.Image = "registry2.example.com/bob/myfunc:latest"
	if err = f.Write(); err != nil {
		t.Fatal(err)
	}
	if err = client.Build(context.Background(), root); err != nil {
		t.Fatal(err)
	}
	if err = client.Deploy(context.Background(), root); err != nil {
		t.Fatal(err)
	}
	expected = "registry2.example.com/bob/myfunc:latest"
	if f.Image != expected {
		t.Fatalf("expected image '%v', got '%v'", expected, f.Image)
	}
	expected = "example.com/alice"
	if f.Registry != "example.com/alice" {
		// Note that according to current logic, the function's defined registry
		// may be inaccurate.  Consider an initial deploy to registryA, followed by
		// an explicit mutaiton of the function's .Image member.
		// This could either remain as a documented nuance:
		//   'The value of f.Registry is only used in the event an image name
		//    need be derived (f.Image =="")
		// Or we could update .Registry to always be in sync by parsing the .Image
		t.Fatalf("expected registry '%v', got '%v'", expected, f.Registry)
	}
}

// TestClient_Pipelines_Deploy_Image ensures that initially the function's image
// member has no value (not initially deployed); the value is populated
// upon pipeline run execution with a value derived from the function's name and currently
// effective client registry; that the value of f.Image will take precidence
// over .Registry, which is used to calculate a default value for image.
func TestClient_Pipelines_Deploy_Image(t *testing.T) {
	root, rm := Mktemp(t)
	defer rm()

	client := fn.New(
		fn.WithPipelinesProvider(mock.NewPipelinesProvider()),
		fn.WithRegistry("example.com/alice"))

	f := fn.Function{
		Name:    "myfunc",
		Runtime: "node",
		Root:    root,
		Build: fn.BuildSpec{
			Git: fn.Git{URL: "http://example-git.com/alice/myfunc.git"},
		},
	}

	err := client.Create(f)
	if err != nil {
		t.Fatal(err)
	}

	f, err = fn.NewFunction(root)
	if err != nil {
		t.Fatal(err)
	}

	// Upon initial creation, the value of .Image is empty
	if f.Image != "" {
		t.Fatalf("new function should have no image, got '%v'", f.Image)
	}

	// Upon pipeline run, the function should be populated;
	if f, err = client.RunPipeline(context.Background(), f); err != nil {
		t.Fatal(err)
	}
	expected := "example.com/alice/myfunc:latest"
	if f.Image != expected {
		t.Fatalf("expected image '%v', got '%v'", expected, f.Image)
	}
	expected = "example.com/alice"
	if f.Registry != expected {
		t.Fatalf("expected registry '%v', got '%v'", expected, f.Registry)
	}

	// The value of .Image always takes precidence
	f.Image = "registry2.example.com/bob/myfunc:latest"
	if err = f.Write(); err != nil {
		t.Fatal(err)
	}
	// Upon pipeline run, the function should be populated;
	if f, err = client.RunPipeline(context.Background(), f); err != nil {
		t.Fatal(err)
	}
	expected = "registry2.example.com/bob/myfunc:latest"
	if f.Image != expected {
		t.Fatalf("expected image '%v', got '%v'", expected, f.Image)
	}
	expected = "example.com/alice"
	if f.Registry != expected {
		// Note that according to current logic, the function's defined registry
		// may be inaccurate.  Consider an initial deploy to registryA, followed by
		// an explicit mutaiton of the function's .Image member.
		// This could either remain as a documented nuance:
		//   'The value of f.Registry is only used in the event an image name
		//    need be derived (f.Image =="")
		// Or we could update .Registry to always be in sync by parsing the .Image
		t.Fatalf("expected registry '%v', got '%v'", expected, f.Registry)
	}
}

// TestClient_Deploy_UnbuiltErrors ensures that a call to deploy a function
// which was not fully created (ie. was only initialized, not actually built
// or deployed) yields the expected error.
func TestClient_Deploy_UnbuiltErrors(t *testing.T) {
	root := "testdata/example.com/testDeployUnbuilt" // Root from which to run the test
	defer Using(t, root)()

	// New Client
	client := fn.New(fn.WithRegistry(TestRegistry))

	// Initialize (half-create) a new function at root
	if err := client.Create(fn.Function{Runtime: TestRuntime, Root: root}); err != nil {
		t.Fatal(err)
	}

	// Now try to deploy it.  Ie. without having run the necessary build step.
	err := client.Deploy(context.Background(), root)
	if err == nil {
		t.Fatal("did not receive an error attempting to deploy an unbuilt function")
	}

	if !errors.Is(err, fn.ErrNotBuilt) {
		t.Fatalf("did not receive expected error type.  Expected ErrNotBuilt, got %T", err)
	}
}

// TestClient_New_BuilderImagesPersisted Asserts that the client preserves user-
// provided Builder Images
func TestClient_New_BuildersPersisted(t *testing.T) {
	root := "testdata/example.com/testConfiguredBuilders" // Root from which to run the test
	defer Using(t, root)()
	client := fn.New(fn.WithRegistry(TestRegistry))

	// A function with predefined builders
	f0 := fn.Function{
		Runtime: TestRuntime,
		Root:    root,
		Build: fn.BuildSpec{
			BuilderImages: map[string]string{
				builders.Pack: "example.com/my/custom-pack-builder",
				builders.S2I:  "example.com/my/custom-s2i-builder",
			}},
	}

	// Create the function, which should preserve custom builders
	if err := client.New(context.Background(), f0); err != nil {
		t.Fatal(err)
	}

	// Load the function from disk
	f1, err := fn.NewFunction(root)
	if err != nil {
		t.Fatal(err)
	}

	// Assert that our custom builders were retained
	if !reflect.DeepEqual(f0.Build.BuilderImages, f1.Build.BuilderImages) {
		t.Fatalf("Expected %v but got %v", f0.Build.BuilderImages, f1.Build.BuilderImages)
	}

	// A Default Builder(image) is not asserted here, because that is
	// the responsibility of the Builder(type) being used to build the function.
	// The builder (Buildpack,s2i, etc) will have a default builder image for
	// the given function or will error that the function is not supported.
	// A builder image may also be manually specified of course.
}

// TestClient_New_BuildpacksPersisted ensures that provided buildpacks are
// persisted on new functions.
func TestClient_New_BuildpacksPersisted(t *testing.T) {
	root := "testdata/example.com/testConfiguredBuildpacks" // Root from which to run the test
	defer Using(t, root)()

	buildpacks := []string{
		"docker.io/example/custom-buildpack",
	}
	client := fn.New(fn.WithRegistry(TestRegistry))
	if err := client.New(context.Background(), fn.Function{
		Runtime: TestRuntime,
		Root:    root,
		Build: fn.BuildSpec{
			Buildpacks: buildpacks,
		},
	}); err != nil {
		t.Fatal(err)
	}
	f, err := fn.NewFunction(root)
	if err != nil {
		t.Fatal(err)
	}

	// Assert that our custom buildpacks were set
	if !reflect.DeepEqual(f.Build.Buildpacks, buildpacks) {
		t.Fatalf("Expected %v but got %v", buildpacks, f.Build.Buildpacks)
	}
}

// TestClient_Runtimes ensures that the total set of runtimes are returned.
func TestClient_Runtimes(t *testing.T) {
	// TODO: test when a specific repo override is indicated
	// (remote repo which takes precidence over embedded and extended)

	client := fn.New(fn.WithRepositoriesPath("testdata/repositories"))

	runtimes, err := client.Runtimes()
	if err != nil {
		t.Fatal(err)
	}

	// Runtimes from `./templates` + `./testdata/repositories`
	// Should be unique and sorted.
	//
	// Note that hard-coding the runtimes list here does add future maintenance
	// (test will fail requiring updates when either the builtin set of or test
	//  set change), but the simplicity and straightforwardness of this
	// requirement seems to outweigh the complexity of calculating the list for
	// testing, which effectively just recreates the logic within the client.
	// Additionally, this list has the benefit of creating a more understandable
	// test (a primary goal of course being human communication of libray intent).
	// If this is an incorrect assumption, we would need to calculate this
	// slice from the contents of ./templates & ./testdata/repositories, taking
	// into acount future repository manifests.
	expected := []string{
		"customRuntime",
		"go",
		"manifestedRuntime",
		"node",
		"python",
		"quarkus",
		"rust",
		"springboot",
		"test",
		"typescript",
	}

	if !reflect.DeepEqual(runtimes, expected) {
		t.Logf("expected: %v", expected)
		t.Logf("received: %v", runtimes)
		t.Fatal("Runtimes not as expected.")
	}
}

// TestClient_New_Timestamp ensures that the creation timestamp is set on functions
// which are successfully initialized using the client library.
func TestClient_New_Timestamp(t *testing.T) {
	root := "testdata/example.com/testCreateStamp"
	defer Using(t, root)()

	start := time.Now()

	client := fn.New(fn.WithRegistry(TestRegistry))

	if err := client.New(context.Background(), fn.Function{Runtime: TestRuntime, Root: root}); err != nil {
		t.Fatal(err)
	}

	f, err := fn.NewFunction(root)
	if err != nil {
		t.Fatal(err)
	}
	if !f.Created.After(start) {
		t.Fatalf("expected function timestamp to be after '%v', got '%v'", start, f.Created)
	}
}

// TestClient_Invoke_HTTP ensures that the client will attempt to invoke a default HTTP
// function using a simple HTTP POST method with the invoke message as form
// field values (as though a simple form were posted).
func TestClient_Invoke_HTTP(t *testing.T) {
	root := "testdata/example.com/testInvokeHTTP"
	defer Using(t, root)()

	// Flag indicating the function was invoked
	var invoked int32

	// The message to send to the function
	// Individual fields can be overridden, by default all fields are populeted
	// with values intended as illustrative examples plus a unique request ID.
	message := fn.NewInvokeMessage()

	// An HTTP handler which masquarades as a running function and verifies the
	// invoker POSTed the invocation message.
	handler := http.NewServeMux()
	handler.HandleFunc("/", func(res http.ResponseWriter, req *http.Request) {
		atomic.StoreInt32(&invoked, 1)

		// Verify that we POST to HTTP endpoints by default
		if req.Method != "POST" {
			t.Errorf("expected 'POST' request, got %q", req.Method)
			return
		}

		data, err := io.ReadAll(req.Body)
		if err != nil {
			t.Errorf("cannot read request body: %v", err)
			return
		}
		dataAsStr := string(data)

		// Verify the body is correct
		if dataAsStr != message.Data {
			t.Errorf("expected message data %q, got %q", message.Data, dataAsStr)
			return
		}

		_, err = res.Write([]byte("hello world"))
		if err != nil {
			t.Error(err)
			return
		}
	})

	// Expose the masquarading function on an OS-chosen port.
	l, err := net.Listen("tcp4", "127.0.0.1:")
	if err != nil {
		t.Fatal(err)
	}
	s := http.Server{Handler: handler}
	go func() {
		if err = s.Serve(l); err != nil && !errors.Is(err, http.ErrServerClosed) {
			fmt.Fprintf(os.Stderr, "error serving: %v", err)
		}
	}()
	t.Cleanup(func() {
		_ = s.Close()
	})

	// Create a client with a mock runner which will report the port at which the
	// interloping function is listening.
	runner := mock.NewRunner()
	runner.RunFn = func(ctx context.Context, f fn.Function) (*fn.Job, error) {
		_, p, _ := net.SplitHostPort(l.Addr().String())
		errs := make(chan error, 10)
		stop := func() {}
		return fn.NewJob(f, p, errs, stop)
	}
	client := fn.New(fn.WithRegistry(TestRegistry), fn.WithRunner(runner))

	// Create a new default HTTP function
	f := fn.Function{Runtime: TestRuntime, Root: root, Template: "http"}
	if err := client.New(context.Background(), f); err != nil {
		t.Fatal(err)
	}

	// Run the function
	job, err := client.Run(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(job.Stop)
	// Invoke the function, which will use the mock Runner
	h, r, err := client.Invoke(context.Background(), f.Root, "", message)
	if err != nil {
		t.Fatal(err)
	}

	// Assert the response includes headers by spot-checking Content-Type
	if _, ok := h["Content-Type"]; !ok {
		t.Fatal("expected headers not returned")
	}

	// Check the response value
	if r != "hello world" {
		t.Fatal("Unexpected response from function " + r)
	}

	// Fail if the function was never invoked.
	if atomic.LoadInt32(&invoked) == 0 {
		t.Fatal("Function was not invoked")
	}

	// Also fail if the mock runner was never invoked.
	if !runner.RunInvoked {
		t.Fatal("the runner was not")
	}
}

// TestClient_Invoke_CloudEvent ensures that the client will attempt to invoke a
// default CloudEvent function.  This also uses the HTTP protocol but asserts
// the invoker is sending the invocation message as a CloudEvent rather than
// a standard HTTP form POST.
func TestClient_Invoke_CloudEvent(t *testing.T) {
	root := "testdata/example.com/testInvokeCloudEvent"
	defer Using(t, root)()

	var (
		invoked bool // flag the function was invoked
		ctx     = context.Background()
		message = fn.NewInvokeMessage() // message to send to the function
		evt     *cloudevents.Event      // A pointer to the received event
	)

	// A CloudEvent Receiver which masquarades as a running function and
	// verifies the invoker sent the message as a populated CloudEvent.
	receiver := func(ctx context.Context, event cloudevents.Event) *cloudevents.Event {
		invoked = true
		if event.ID() != message.ID {
			t.Fatalf("expected event ID '%v', got '%v'", message.ID, event.ID())
		}
		evt = &event
		return evt
	}

	// A cloudevent receive handler which will expect the HTTP protocol
	protocol, err := cloudevents.NewHTTP() // Use HTTP protocol when receiving
	if err != nil {
		t.Fatal(err)
	}
	handler, err := cloudevents.NewHTTPReceiveHandler(ctx, protocol, receiver)
	if err != nil {
		t.Fatal(err)
	}

	// Listen and serve on an OS-chosen port
	l, err := net.Listen("tcp4", "127.0.0.1:")
	if err != nil {
		t.Fatal(err)
	}
	s := http.Server{Handler: handler}
	go func() {
		if err := s.Serve(l); err != nil && err != http.ErrServerClosed {
			fmt.Fprintf(os.Stderr, "error serving: %v", err)
		}
	}()
	defer s.Close()

	// Create a client with a mock Runner which returns its address.
	runner := mock.NewRunner()
	runner.RunFn = func(ctx context.Context, f fn.Function) (*fn.Job, error) {
		_, p, _ := net.SplitHostPort(l.Addr().String())
		errs := make(chan error, 10)
		stop := func() {}
		return fn.NewJob(f, p, errs, stop)
	}
	client := fn.New(fn.WithRegistry(TestRegistry), fn.WithRunner(runner))

	// Create a new default CloudEvents function
	f := fn.Function{Runtime: TestRuntime, Root: root, Template: "cloudevents"}
	if err := client.New(context.Background(), f); err != nil {
		t.Fatal(err)
	}

	// Run the function
	job, err := client.Run(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	defer job.Stop()

	// Invoke the function, which will use the mock Runner
	_, r, err := client.Invoke(context.Background(), f.Root, "", message)
	if err != nil {
		t.Fatal(err)
	}

	// Test the contents of the returned string.
	if r != evt.String() {
		t.Fatal("Invoke failed to return a response")
	}
	// Fail if the function was never invoked.
	if !invoked {
		t.Fatal("Function was not invoked")
	}

	// Also fail if the mock runner was never invoked.
	if !runner.RunInvoked {
		t.Fatal("the runner was not invoked")
	}
}

// TestClient_Instances ensures that when a function is run (locally) its metadata
// is available to other clients inspecting the same function using .Instances
func TestClient_Instances(t *testing.T) {
	root := "testdata/example.com/testInstances"
	defer Using(t, root)()

	// A mock runner
	runner := mock.NewRunner()
	runner.RunFn = func(_ context.Context, f fn.Function) (*fn.Job, error) {
		errs := make(chan error, 10)
		stop := func() {}
		return fn.NewJob(f, "8080", errs, stop)
	}

	// Client with the mock runner
	client := fn.New(fn.WithRegistry(TestRegistry), fn.WithRunner(runner))

	// Create the new function
	if err := client.New(context.Background(), fn.Function{Root: root, Runtime: TestRuntime}); err != nil {
		t.Fatal(err)
	}

	// Run the function, awaiting start and then canceling
	job, err := client.Run(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	defer job.Stop()

	// Load the (now fully initialized) function metadata
	f, err := fn.NewFunction(root)
	if err != nil {
		t.Fatal(err)
	}

	// Get the local function instance info
	instance, err := client.Instances().Local(context.Background(), f)
	if err != nil {
		t.Fatal(err)
	}

	// Assert the endpoint route is as expected
	expectedEndpoint := "http://localhost:8080/"
	if instance.Route != expectedEndpoint {
		t.Fatalf("Expected endpoint '%v', got '%v'", expectedEndpoint, instance.Route)
	}
}

// TestClient_BuiltStamps ensures that the client creates and considers a
// buildstamp on build which reports whether or not a given path contains a built
// function.
func TestClient_BuiltStamps(t *testing.T) {
	root, rm := Mktemp(t)
	defer rm()
	builder := mock.NewBuilder()
	client := fn.New(fn.WithBuilder(builder), fn.WithRegistry(TestRegistry))

	// paths that do not contain a function are !Built - Degenerate case
	if client.Built(root) {
		t.Fatal("path not containing a function returned as being built")
	}

	// a freshly-created function should be !Built
	if err := client.Create(fn.Function{Runtime: TestRuntime, Root: root}); err != nil {
		t.Fatal(err)
	}
	if client.Built(root) {
		t.Fatal("newly created function returned Built==true")
	}

	// a function which was successfully built should return as being Built
	if err := client.Build(context.Background(), root); err != nil {
		t.Fatal(err)
	}
	if !client.Built(root) {
		t.Fatal("freshly built function should return Built==true")
	}
}

// TestClient_CreateMigration ensures that the client includes the most recent
// migration version when creating a new function
func TestClient_CreateMigration(t *testing.T) {
	root, rm := Mktemp(t)
	defer rm()
	client := fn.New()

	// create a new function
	if err := client.Create(fn.Function{Runtime: TestRuntime, Root: root}); err != nil {
		t.Fatal(err)
	}

	// create a new function
	f, err := fn.NewFunction(root)
	if err != nil {
		t.Fatal(err)
	}

	// A freshly created function should have the latest migration
	if f.SpecVersion != fn.LastSpecVersion() {
		t.Fatal("freshly created function should have the latest migration")
	}
}

// TestClient_BuiltDetects ensures that the client's Built command detects
// filesystem changes as indicating the function is no longer Built (aka stale)
// This includes modifying timestamps, removing or adding files.
func TestClient_BuiltDetects(t *testing.T) {
	var (
		ctx      = context.Background()
		builder  = mock.NewBuilder()
		client   = fn.New(fn.WithBuilder(builder), fn.WithRegistry(TestRegistry))
		testfile = "example.go"
		root, rm = Mktemp(t)
	)
	defer rm()

	// Create and build a function
	if err := client.Create(fn.Function{Runtime: TestRuntime, Root: root}); err != nil {
		t.Fatal(err)
	}
	if err := client.Build(ctx, root); err != nil {
		t.Fatal(err)
	}

	// Prior to a filesystem edit, it will be Built.
	if !client.Built(root) {
		t.Fatal("freshly built function reported Built==false (1)")
	}

	// Release thread and wait to ensure that the clock advances even in constrained CI environments
	time.Sleep(100 * time.Millisecond)

	// Edit the filesystem by touching a file (updating modified timestamp)
	if err := os.Chtimes(filepath.Join(root, "func.yaml"), time.Now(), time.Now()); err != nil {
		fmt.Println(err)
	}

	// Release thread and wait to ensure that the clock advances even in constrained CI environments
	time.Sleep(100 * time.Millisecond)

	if client.Built(root) {
		t.Fatal("client did not detect file timestamp change as indicating build staleness")
	}

	// Build and double-check Built has been reset
	if err := client.Build(ctx, root); err != nil {
		t.Fatal(err)
	}
	if !client.Built(root) {
		t.Fatal("freshly built function reported Built==false (2)")
	}

	// Edit the function's filesystem by adding a file.
	f, err := os.Create(filepath.Join(root, testfile))
	if err != nil {
		t.Fatal(err)
	}
	f.Close()

	// The system should now detect the function is stale
	if client.Built(root) {
		t.Fatal("client did not detect an added file as indicating build staleness")
	}

	// Build and double-check Built has been reset
	if err := client.Build(ctx, root); err != nil {
		t.Fatal(err)
	}
	if !client.Built(root) {
		t.Fatal("freshly built function reported Built==false (3)")
	}

	// Remove the testfile, which should result in the client reporting that
	// the function is no longer Built (stale)
	if err := os.Remove(filepath.Join(root, testfile)); err != nil {
		t.Fatal(err)
	}
	if client.Built(root) {
		t.Fatal("client did not detect a removed file as indicating build staleness")
	}
}
