package faas_test

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/boson-project/faas"
	"github.com/boson-project/faas/mock"
)

// TestRegistry for calculating destination image during tests.
// Will be optional once we support in-cluster container registries
// by default.  See TestRegistryRequired for details.
const TestRegistry = "quay.io/alice"

// TestNew Function completes without error using defaults and zero values.
// New is the superset of creating a new fully deployed Function, and
// thus implicitly tests Create, Build and Deploy, which are exposed
// by the client API for those who prefer manual transmissions.
func TestNew(t *testing.T) {
	root := "testdata/example.com/testCreate" // Root from which to run the test
	if err := os.MkdirAll(root, 0700); err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(root)

	// New Client
	client := faas.New(faas.WithRegistry(TestRegistry))

	// New Function using Client
	if err := client.New(faas.Function{Root: root}); err != nil {
		t.Fatal(err)
	}
}

// TestTemplateWrites ensures a template is written.
func TestTemplateWrites(t *testing.T) {
	root := "testdata/example.com/testCreateWrites"
	if err := os.MkdirAll(root, 0744); err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(root)

	client := faas.New(faas.WithRegistry(TestRegistry))
	if err := client.Create(faas.Function{Root: root}); err != nil {
		t.Fatal(err)
	}

	// Assert file was written
	if _, err := os.Stat(filepath.Join(root, faas.ConfigFile)); os.IsNotExist(err) {
		t.Fatalf("Initialize did not result in '%v' being written to '%v'", faas.ConfigFile, root)
	}
}

// TestExtantAborts ensures that a directory which contains an extant
// Function does not reinitialize
func TestExtantAborts(t *testing.T) {
	root := "testdata/example.com/testCreateInitializedAborts"
	if err := os.MkdirAll(root, 0744); err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(root)

	// New once
	client := faas.New(faas.WithRegistry(TestRegistry))
	if err := client.New(faas.Function{Root: root}); err != nil {
		t.Fatal(err)
	}

	// New again should fail as already initialized
	if err := client.New(faas.Function{Root: root}); err == nil {
		t.Fatal("error expected initilizing a path already containing an initialized Function")
	}
}

// TestNonemptyDirectoryAborts ensures that a directory which contains any
// visible files aborts.
func TestNonemptyDirectoryAborts(t *testing.T) {
	root := "testdata/example.com/testCreateNonemptyDirectoryAborts" // contains only a single visible file.
	if err := os.MkdirAll(root, 0744); err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(root)

	// An unexpected, non-hidden file.
	_, err := os.Create(root + "/file.txt")
	if err != nil {
		t.Fatal(err)
	}

	client := faas.New(faas.WithRegistry(TestRegistry))
	if err := client.New(faas.Function{Root: root}); err == nil {
		t.Fatal("error expected initilizing a Function in a nonempty directory")
	}
}

// TestHiddenFilesIgnored ensures that initializing in a directory that
// only contains hidden files does not error, protecting against the naieve
// implementation of aborting initialization if any files exist, which would
// break functions tracked in source control (.git), or when used in
// conjunction with other tools (.envrc, etc)
func TestHiddenFilesIgnored(t *testing.T) {
	// Create a directory for the Function
	root := "testdata/example.com/testCreateHiddenFilesIgnored"
	if err := os.MkdirAll(root, 0744); err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(root)

	// Create a hidden file that should be ignored.
	hiddenFile := filepath.Join(root, ".envrc")
	if err := ioutil.WriteFile(hiddenFile, []byte{}, 0644); err != nil {
		t.Fatal(err)
	}

	client := faas.New(faas.WithRegistry(TestRegistry))
	if err := client.New(faas.Function{Root: root}); err != nil {
		t.Fatal(err)
	}
}

// TestDefaultRuntime ensures that the default runtime is applied to new
// Functions and persisted.
func TestDefaultRuntime(t *testing.T) {
	// Create a root for the new Function
	root := "testdata/example.com/testCreateDefaultRuntime"
	if err := os.MkdirAll(root, 0744); err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(root)

	// Create a new function at root with all defaults.
	client := faas.New(faas.WithRegistry(TestRegistry))
	if err := client.New(faas.Function{Root: root}); err != nil {
		t.Fatal(err)
	}

	// Load the function
	f, err := faas.NewFunction(root)
	if err != nil {
		t.Fatal(err)
	}

	// Ensure it has defaulted runtime
	if f.Runtime != faas.DefaultRuntime {
		t.Fatal("The default runtime was not applied or persisted.")
	}
}

// TestDefaultTemplate ensures that the default template is
// applied when not provided.
func TestDefaultTrigger(t *testing.T) {
	// TODO: need to either expose accessor for introspection, or compare
	// the files written to those in the embedded repisotory?
}

// TestExtensibleTemplates templates.  Ensures that templates are extensible
// using a custom path to a template repository on disk.  Custom repository
// location is not defined herein but expected to be provided because, for
// example, a CLI may want to use XDG_CONFIG_HOME.  Assuming a repository path
// $FUNC_TEMPLATES, a Go template named 'json' which is provided in the
// repository 'boson-experimental', would be expected to be in the location:
// $FUNC_TEMPLATES/boson-experimental/go/json
// See the CLI for full details, but a standard default location is
// $HOME/.config/templates/boson-experimental/go/json
func TestExtensibleTemplates(t *testing.T) {
	// Create a directory for the new Function
	root := "testdata/example.com/testExtensibleTemplates"
	if err := os.MkdirAll(root, 0744); err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(root)

	// Create a new client with a path to the extensible templates
	client := faas.New(
		faas.WithTemplates("testdata/templates"),
		faas.WithRegistry(TestRegistry))

	// Create a Function specifying a template, 'json' that only exists in the extensible set
	if err := client.New(faas.Function{Root: root, Trigger: "boson-experimental/json"}); err != nil {
		t.Fatal(err)
	}

	// Ensure that a file from that only exists in that template set was actually written 'json.js'
	if _, err := os.Stat(filepath.Join(root, "json.js")); os.IsNotExist(err) {
		t.Fatalf("Initializing a custom did not result in json.js being written to '%v'", root)
	} else if err != nil {
		t.Fatal(err)
	}
}

// TestUnsupportedRuntime generates an error.
func TestUnsupportedRuntime(t *testing.T) {
	// Create a directory for the Function
	root := "testdata/example.com/testUnsupportedRuntime"
	if err := os.MkdirAll(root, 0700); err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(root)

	client := faas.New(faas.WithRegistry(TestRegistry))

	// create a Function call witn an unsupported runtime should bubble
	// the error generated by the underlying initializer.
	if err := client.New(faas.Function{Root: root, Runtime: "invalid"}); err == nil {
		t.Fatal("unsupported runtime did not generate error")
	}
}

// TestNamed ensures that an explicitly passed name is used in leau of the
// path derived name when provided, and persists through instantiations.
func TestNamed(t *testing.T) {
	// Explicit name to use
	name := "service.example.com"

	// Path which would derive to testWithHame.example.com were it not for the
	// explicitly provided name.
	root := "testdata/example.com/testWithName"

	// Create a root directory for the Function
	if err := os.MkdirAll(root, 0700); err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(root)

	client := faas.New(faas.WithRegistry(TestRegistry))

	if err := client.New(faas.Function{Root: root, Name: name}); err != nil {
		t.Fatal(err)
	}

	f, err := faas.NewFunction(root)
	if err != nil {
		t.Fatal(err)
	}

	if f.Name != name {
		t.Fatalf("expected name '%v' got '%v", name, f.Name)
	}
}

// TestRegistryRequired ensures that a registry is required, and is
// prepended with the DefaultRegistry if a single token.
// Registry is the namespace at the container image registry.
// If not prepended with the registry, it will be defaulted:
// Examples:  "docker.io/alice"
//            "quay.io/bob"
//            "charlie" (becomes [DefaultRegistry]/charlie
// At this time a registry namespace is required as we rely on a third-party
// registry in all cases.  When we support in-cluster container registries,
// this configuration parameter will become optional.
func TestRegistryRequired(t *testing.T) {
	// Create a root for the Function
	root := "testdata/example.com/testRegistry"
	if err := os.MkdirAll(root, 0700); err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(root)

	client := faas.New()
	var err error
	if err = client.New(faas.Function{Root: root}); err == nil {
		t.Fatal("did not receive expected error creating a Function without specifying Registry")
	}
	fmt.Println(err)
}

// TestDeriveImage ensures that the full image (tag) of the resultant OCI
// container is populated based of a derivation using configured registry
// plus the service name.
func TestDeriveImage(t *testing.T) {
	// Create the root Function directory
	root := "testdata/example.com/testDeriveImage"
	if err := os.MkdirAll(root, 0700); err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(root)

	// Create the function which calculates fields such as name and image.
	client := faas.New(faas.WithRegistry(TestRegistry))
	if err := client.New(faas.Function{Root: root}); err != nil {
		t.Fatal(err)
	}

	// Load the function with the now-populated fields.
	f, err := faas.NewFunction(root)
	if err != nil {
		t.Fatal(err)
	}

	// In form: [Default Registry]/[Registry Namespace]/[Service Name]:latest
	expected := TestRegistry + "/" + f.Name + ":latest"
	if f.Image != expected {
		t.Fatalf("expected image '%v' got '%v'", expected, f.Image)
	}
}

// TestDeriveImageDefaultRegistry ensures that a Registry which does not have
// a registry prefix has the DefaultRegistry prepended.
// For example "alice" becomes "docker.io/alice"
func TestDeriveImageDefaultRegistry(t *testing.T) {
	// Create the root Function directory
	root := "testdata/example.com/testDeriveImageDefaultRegistry"
	if err := os.MkdirAll(root, 0700); err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(root)

	// Create the function which calculates fields such as name and image.
	// Rather than use TestRegistry, use a single-token name and expect
	// the DefaultRegistry to be prepended.
	client := faas.New(faas.WithRegistry("alice"))
	if err := client.New(faas.Function{Root: root}); err != nil {
		t.Fatal(err)
	}

	// Load the function with the now-populated fields.
	f, err := faas.NewFunction(root)
	if err != nil {
		t.Fatal(err)
	}
	// Expected image is [DefaultRegistry]/[namespace]/[servicename]:latest
	expected := faas.DefaultRegistry + "/alice/" + f.Name + ":latest"
	if f.Image != expected {
		t.Fatalf("expected image '%v' got '%v'", expected, f.Image)
	}
}

// TestDelegation ensures that Create invokes each of the individual
// subcomponents via delegation through Build, Push and
// Deploy (and confirms expected fields calculated).
func TestNewDelegates(t *testing.T) {
	var (
		root          = "testdata/example.com/testCreateDelegates" // .. in which to initialize
		expectedName  = "testCreateDelegates"                      // expected to be derived
		expectedImage = "quay.io/alice/testCreateDelegates:latest"
		builder       = mock.NewBuilder()
		pusher        = mock.NewPusher()
		deployer      = mock.NewDeployer()
	)

	// Create a directory for the new Function
	if err := os.MkdirAll(root, 0700); err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(root)

	// Create a client with mocks for each of the subcomponents.
	client := faas.New(
		faas.WithRegistry(TestRegistry),
		faas.WithBuilder(builder),   // builds an image
		faas.WithPusher(pusher),     // pushes images to a registry
		faas.WithDeployer(deployer), // deploys images as a running service
	)

	// Register Function delegates on the mocks which validate assertions
	// -------------

	// The builder should be invoked with a path to a Function project's source
	// An example image name is returned.
	builder.BuildFn = func(f faas.Function) error {
		expectedPath, err := filepath.Abs(root)
		if err != nil {
			t.Fatal(err)
		}
		if expectedPath != f.Root {
			t.Fatalf("builder expected path %v, got '%v'", expectedPath, f.Root)
		}
		return nil
	}

	pusher.PushFn = func(f faas.Function) (string, error) {
		if f.Image != expectedImage {
			t.Fatalf("pusher expected image '%v', got '%v'", expectedImage, f.Image)
		}
		return "", nil
	}

	deployer.DeployFn = func(f faas.Function) error {
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

	// Invoke the creation, triggering the Function delegates, and
	// perform follow-up assertions that the Functions were indeed invoked.
	if err := client.New(faas.Function{Root: root}); err != nil {
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

// TestRun ensures that the runner is invoked with the absolute path requested.
func TestRun(t *testing.T) {
	// Create the root Function directory
	root := "testdata/example.com/testRun"
	if err := os.MkdirAll(root, 0700); err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(root)

	// Create a client with the mock runner and the new test Function
	runner := mock.NewRunner()
	client := faas.New(faas.WithRegistry(TestRegistry), faas.WithRunner(runner))
	if err := client.New(faas.Function{Root: root}); err != nil {
		t.Fatal(err)
	}

	// Run the newly created function
	if err := client.Run(root); err != nil {
		t.Fatal(err)
	}

	// Assert the runner was invoked, and with the expected root.
	if !runner.RunInvoked {
		t.Fatal("run did not invoke the runner")
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		t.Fatal(err)
	}
	if runner.RootRequested != absRoot {
		t.Fatalf("expected path '%v', got '%v'", absRoot, runner.RootRequested)
	}
}

// TestUpdate ensures that the deployer properly invokes the build/push/deploy
// process, erroring if run on a directory uncreated.
func TestUpdate(t *testing.T) {
	var (
		root          = "testdata/example.com/testUpdate"
		expectedName  = "testUpdate"
		expectedImage = "quay.io/alice/testUpdate:latest"
		builder       = mock.NewBuilder()
		pusher        = mock.NewPusher()
		deployer      = mock.NewDeployer()
	)

	// Create the root Function directory
	if err := os.MkdirAll(root, 0700); err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(root)

	// A client with mocks whose implementaton will validate input.
	client := faas.New(
		faas.WithRegistry(TestRegistry),
		faas.WithBuilder(builder),
		faas.WithPusher(pusher),
		faas.WithDeployer(deployer))

	// create the new Function which will be updated
	if err := client.New(faas.Function{Root: root}); err != nil {
		t.Fatal(err)
	}

	// Builder whose implementation verifies the expected root
	builder.BuildFn = func(f faas.Function) error {
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
	pusher.PushFn = func(f faas.Function) (string, error) {
		if f.Image != expectedImage {
			t.Fatalf("pusher expected image '%v', got '%v'", expectedImage, f.Image)
		}
		// image of given name wouold be pushed to the configured registry.
		return "", nil
	}

	// Update whose implementaiton verifed the expected name and image
	deployer.DeployFn = func(f faas.Function) error {
		if f.Name != expectedName {
			t.Fatalf("updater expected name '%v', got '%v'", expectedName, f.Name)
		}
		if f.Image != expectedImage {
			t.Fatalf("updater expected image '%v', got '%v'", expectedImage, f.Image)
		}
		return nil
	}

	// Invoke the creation, triggering the Function delegates, and
	// perform follow-up assertions that the Functions were indeed invoked.
	if err := client.Deploy(root); err != nil {
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
}

// TestRemoveByPath ensures that the remover is invoked to remove
// the Function with the name of the function at the provided root.
func TestRemoveByPath(t *testing.T) {
	var (
		root         = "testdata/example.com/testRemoveByPath"
		expectedName = "testRemoveByPath"
		remover      = mock.NewRemover()
	)

	if err := os.MkdirAll(root, 0700); err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(root)

	client := faas.New(
		faas.WithRegistry(TestRegistry),
		faas.WithRemover(remover))

	if err := client.New(faas.Function{Root: root}); err != nil {
		t.Fatal(err)
	}

	remover.RemoveFn = func(name string) error {
		if name != expectedName {
			t.Fatalf("Expected to remove '%v', got '%v'", expectedName, name)
		}
		return nil
	}

	if err := client.Remove(faas.Function{Root: root}); err != nil {
		t.Fatal(err)
	}

	if !remover.RemoveInvoked {
		t.Fatal("remover was not invoked")
	}

}

// TestRemoveByName ensures that the remover is invoked to remove the function
// of the name provided, with precidence over a provided root path.
func TestRemoveByName(t *testing.T) {
	var (
		root         = "testdata/example.com/testRemoveByPath"
		expectedName = "explicitName.example.com"
		remover      = mock.NewRemover()
	)

	if err := os.MkdirAll(root, 0700); err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(root)

	client := faas.New(
		faas.WithRegistry(TestRegistry),
		faas.WithRemover(remover))

	if err := client.Create(faas.Function{Root: root}); err != nil {
		t.Fatal(err)
	}

	remover.RemoveFn = func(name string) error {
		if name != expectedName {
			t.Fatalf("Expected to remove '%v', got '%v'", expectedName, name)
		}
		return nil
	}

	// Run remove with only a name
	if err := client.Remove(faas.Function{Name: expectedName}); err != nil {
		t.Fatal(err)
	}

	// Run remove with a name and a root, which should be ignored in favor of the name.
	if err := client.Remove(faas.Function{Name: expectedName, Root: root}); err != nil {
		t.Fatal(err)
	}

	if !remover.RemoveInvoked {
		t.Fatal("remover was not invoked")
	}
}

// TestRemoveUninitializedFails ensures that attempting to remove a Function
// by path only (no name) fails unless the Function has been initialized.  I.e.
// the name will not be derived from path and the Function removed by this
// derived name; which could be unexpected and destructive.
func TestRemoveUninitializedFails(t *testing.T) {
	var (
		root    = "testdata/example.com/testRemoveUninitializedFails"
		remover = mock.NewRemover()
	)
	err := os.MkdirAll(root, 0700)
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(root)

	// remover fails if invoked
	remover.RemoveFn = func(name string) error {
		return fmt.Errorf("remove invoked for unitialized Function %v", name)
	}

	// Instantiate the client with the failing remover.
	client := faas.New(
		faas.WithRegistry(TestRegistry),
		faas.WithRemover(remover))

	// Attempt to remove by path (uninitialized), expecting an error.
	if err := client.Remove(faas.Function{Root: root}); err == nil {
		t.Fatalf("did not received expeced error removing an uninitialized func")
	}
}

// TestList merely ensures that the client invokes the configured lister.
func TestList(t *testing.T) {
	lister := mock.NewLister()

	client := faas.New(faas.WithLister(lister)) // lists deployed Functions.

	if _, err := client.List(); err != nil {
		t.Fatal(err)
	}

	if !lister.ListInvoked {
		t.Fatal("list did not invoke lister implementation")
	}
}

// TestListOutsideRoot ensures that a call to a Function (in this case list)
// that is not contextually dependent on being associated with a Function,
// can be run from anywhere, thus ensuring that the client itself makes
// a distinction between Function-scoped methods and not.
func TestListOutsideRoot(t *testing.T) {
	lister := mock.NewLister()

	// Instantiate in the current working directory, with no name.
	client := faas.New(faas.WithLister(lister))

	if _, err := client.List(); err != nil {
		t.Fatal(err)
	}

	if !lister.ListInvoked {
		t.Fatal("list did not invoke lister implementation")
	}
}

// TODO: The tests which confirm an error is generated do not currently test
// that the expected error is received; just that any error is generated.
// This should be replaced with typed errors or at a minimum code prefixes
// on the string to avoid tests passing for unrelated errors.
