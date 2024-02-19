//go:build !integration
// +build !integration

package functions_test

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	"knative.dev/func/pkg/builders"
	fn "knative.dev/func/pkg/functions"
	"knative.dev/func/pkg/mock"
	"knative.dev/func/pkg/oci"
	. "knative.dev/func/pkg/testing"
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

var (
	// TestPlatforms to use when a multi-architecture build is not necessary
	// for testing.
	TestPlatforms = []fn.Platform{{OS: runtime.GOOS, Architecture: runtime.GOARCH}}
)

// TestClient_New function completes without error using defaults and zero values.
// New is the superset of creating a new fully deployed function, and
// thus implicitly tests Create, Build and Deploy, which are exposed
// by the client API for those who prefer manual transmissions.
func TestClient_New(t *testing.T) {
	root := "testdata/example.com/test-new"
	defer Using(t, root)()

	client := fn.New(fn.WithRegistry(TestRegistry), fn.WithVerbose(true))

	if _, _, err := client.New(context.Background(), fn.Function{Root: root, Runtime: TestRuntime}); err != nil {
		t.Fatal(err)
	}
}

// TestClient_New_RunData ensures that the .func runtime directory is
// correctly created.
func TestClient_New_RunDataDir(t *testing.T) {
	root, rm := Mktemp(t)
	defer rm()
	ctx := context.Background()

	// Ensure the run data directory is created when the function is created
	if _, _, err := fn.New().New(ctx, fn.Function{Root: root, Runtime: "go", Registry: TestRegistry}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, fn.RunDataDir)); os.IsNotExist(err) {
		t.Fatal("runtime directory not created when function created.")
	}

	// Ensure it is set as ignored in a .gitignore
	file, err := os.Open(filepath.Join(root, ".gitignore"))
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	foundEntry := false
	s := bufio.NewScanner(file)
	for s.Scan() {
		if strings.HasPrefix(s.Text(), "/"+fn.RunDataDir) {
			foundEntry = true
			break
		}
	}
	if !foundEntry {
		t.Fatal("run data dir not added to .gitignore")
	}

	// Ensure that if .gitignore already existed, it is modified not overwritten
	root, rm = Mktemp(t)
	defer rm()
	if err = os.WriteFile(filepath.Join(root, ".gitignore"), []byte("user-directive\n"), os.ModePerm); err != nil {
		t.Fatal(err)
	}
	if _, _, err := fn.New().New(ctx, fn.Function{Root: root, Runtime: "go", Registry: TestRegistry}); err != nil {
		t.Fatal(err)
	}
	containsUserDirective, containsFuncDirective := false, false
	if file, err = os.Open(filepath.Join(root, ".gitignore")); err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	s = bufio.NewScanner(file)
	for s.Scan() { // scan each line
		if strings.HasPrefix(s.Text(), "user-directive") {
			containsUserDirective = true
		}
		if strings.HasPrefix(s.Text(), "/"+fn.RunDataDir) {
			containsFuncDirective = true
		}
	}
	if !containsUserDirective {
		t.Fatal("extant .gitignore did not retain user direcives after creation")
	}
	if !containsFuncDirective {
		t.Fatal("extant .gitignore was not modified with func data ignore directive")
	}

	// Ensure that the user can cancel this behavior entirely by including the
	// ignore directive, but commented out.
	root, rm = Mktemp(t)
	defer rm()

	userDirective := fmt.Sprintf("# /%v", fn.RunDataDir) // User explicitly commented
	funcDirective := fmt.Sprintf("/%v", fn.RunDataDir)
	if err = os.WriteFile(filepath.Join(root, ".gitignore"), []byte(userDirective+"/n"), os.ModePerm); err != nil {
		t.Fatal(err)
	}
	if _, _, err := fn.New().New(ctx, fn.Function{Root: root, Runtime: "go", Registry: TestRegistry}); err != nil {
		t.Fatal(err)
	}
	containsUserDirective, containsFuncDirective = false, false
	if file, err = os.Open(filepath.Join(root, ".gitignore")); err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	s = bufio.NewScanner(file)
	for s.Scan() { // scan each line
		if strings.HasPrefix(s.Text(), userDirective) {
			containsUserDirective = true
		}
		if strings.HasPrefix(s.Text(), funcDirective) {
			containsFuncDirective = true
		}
	}
	if !containsUserDirective {
		t.Fatal("The user's directive to disable modifing .gitignore was removed")
	}
	if containsFuncDirective {
		t.Fatal("The user's directive to explicitly allow .func in source control was not respected")
	}

	// Ensure that in addition the the correctly formatted comment "# /.func",
	// it will work if the user omits the space: "#/.func"
	root, rm = Mktemp(t)
	defer rm()
	userDirective = fmt.Sprintf("#/%v", fn.RunDataDir) // User explicitly commented but without space
	if err = os.WriteFile(filepath.Join(root, ".gitignore"), []byte(userDirective+"/n"), os.ModePerm); err != nil {
		t.Fatal(err)
	}
	if _, _, err := fn.New().New(ctx, fn.Function{Root: root, Runtime: "go", Registry: TestRegistry}); err != nil {
		t.Fatal(err)
	}
	containsFuncDirective = false
	if file, err = os.Open(filepath.Join(root, ".gitignore")); err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	s = bufio.NewScanner(file)
	for s.Scan() { // scan each line
		if strings.HasPrefix(s.Text(), funcDirective) {
			containsFuncDirective = true
			break
		}
	}
	if containsFuncDirective {
		t.Fatal("The user's directive to explicitly allow .func in source control was not respected")
	}

	// TODO: It is possible that we need to consider more complex situations,
	// such as ensuring that files and directories with just the prefix are not
	// matched, that the user can use non-absolute ignores (no slash prefix), etc.
	// If this turns out to be necessary, we will need to add the test cases
	// and have the implementation actually parse the file rather that simple
	// line prefix checks.
}

// TestClient_New_RuntimeRequired ensures that the the runtime is an expected value.
func TestClient_New_RuntimeRequired(t *testing.T) {
	// Create a root for the new function
	root := "testdata/example.com/testRuntimeRequired"
	defer Using(t, root)()

	client := fn.New(fn.WithRegistry(TestRegistry))

	// Create a new function at root with all defaults.
	_, _, err := client.New(context.Background(), fn.Function{Root: root})
	if err == nil {
		t.Fatalf("did not receive error creating a function without specifying runtime")
	}
}

// TestClient_New_NameDefaults ensures that a newly created function has its name defaulted
// to a name which can be derived from the last part of the given root path.
func TestClient_New_NameDefaults(t *testing.T) {
	root := "testdata/example.com/test-name-defaults"
	defer Using(t, root)()

	client := fn.New(fn.WithRegistry(TestRegistry))

	f := fn.Function{
		Runtime: TestRuntime,
		// NO NAME
		Root: root,
	}

	if _, _, err := client.New(context.Background(), f); err != nil {
		t.Fatal(err)
	}

	f, err := fn.NewFunction(root)
	if err != nil {
		t.Fatal(err)
	}

	expected := "test-name-defaults"
	if f.Name != expected {
		t.Fatalf("name was not defaulted. expected '%v' got '%v'", expected, f.Name)
	}
}

// TestClient_New_WritesTemplate ensures the config file and files from the template
// are written on new.
func TestClient_New_WritesTemplate(t *testing.T) {
	root := "testdata/example.com/test-writes-template"
	defer Using(t, root)()

	client := fn.New(fn.WithRegistry(TestRegistry))

	if _, _, err := client.New(context.Background(), fn.Function{Runtime: TestRuntime, Root: root}); err != nil {
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
	root := "testdata/example.com/test-extant-aborts"
	defer Using(t, root)()

	client := fn.New(fn.WithRegistry(TestRegistry))

	// First .New should succeed...
	if _, _, err := client.New(context.Background(), fn.Function{Runtime: TestRuntime, Root: root}); err != nil {
		t.Fatal(err)
	}

	// Calling again should abort...
	if _, _, err := client.New(context.Background(), fn.Function{Root: root}); err == nil {
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
	if _, _, err := client.New(context.Background(), fn.Function{Root: root}); err == nil {
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
	root := "testdata/example.com/test-hidden-files-ignored"
	defer Using(t, root)()

	client := fn.New(fn.WithRegistry(TestRegistry))

	// Create a hidden file that should be ignored.
	hiddenFile := filepath.Join(root, ".envrc")
	if err := os.WriteFile(hiddenFile, []byte{}, 0644); err != nil {
		t.Fatal(err)
	}

	// Should succeed without error, ignoring the hidden file.
	if _, _, err := client.New(context.Background(), fn.Function{Runtime: TestRuntime, Root: root}); err != nil {
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
	root := "testdata/example.com/test-repositories-extensible"
	defer Using(t, root)()

	client := fn.New(
		fn.WithRepositoriesPath("testdata/repositories"),
		fn.WithRegistry(TestRegistry))

	// Create a function specifying a template which only exists in the extensible set
	if _, _, err := client.New(context.Background(), fn.Function{Root: root, Runtime: "test", Template: "customTemplateRepo/tplc"}); err != nil {
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
// runtime is not found (embedded default repository).
func TestClient_New_RuntimeNotFoundError(t *testing.T) {
	root := "testdata/example.com/testRuntimeNotFound"
	defer Using(t, root)()

	client := fn.New(fn.WithRegistry(TestRegistry))

	// creating a function with an unsupported runtime should bubble
	// the error generated by the underlying template initializer.
	_, _, err := client.New(context.Background(), fn.Function{Root: root, Runtime: "invalid"})
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
	_, _, err := client.New(context.Background(), f)
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
	_, _, err := client.New(context.Background(), f)
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
	_, _, err := client.New(context.Background(), f)
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

	var f fn.Function
	var err error
	if _, f, err = client.New(context.Background(), fn.Function{Runtime: TestRuntime, Root: root, Name: name}); err != nil {
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
	if _, _, err = client.New(context.Background(), fn.Function{Root: root}); err == nil {
		t.Fatal("did not receive expected error creating a function without specifying Registry")
	}
}

// TestClient_New_ImageNamePopulated ensures that the full image (tag) of the
// resultant OCI container is populated.
func TestClient_New_ImageNamePopulated(t *testing.T) {
	// Create the root function directory
	root := "testdata/example.com/test-derive-image"
	defer Using(t, root)()

	// Create the function which calculates fields such as name and image.
	client := fn.New(fn.WithRegistry(TestRegistry))
	var f fn.Function
	var err error
	if _, f, err = client.New(context.Background(), fn.Function{Runtime: TestRuntime, Root: root}); err != nil {
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
	if f.Build.Image != imageTag {
		t.Fatalf("expected image '%v' got '%v'", imageTag, f.Build.Image)
	}
}

// TestCleint_New_ImageRegistryDefaults ensures that a Registry which does not have
// a registry prefix has the DefaultRegistry prepended.
// For example "alice" becomes "docker.io/alice"
func TestClient_New_ImageRegistryDefaults(t *testing.T) {
	// Create the root function directory
	root := "testdata/example.com/test-derive-image-default-registry"
	defer Using(t, root)()

	// Create the function which calculates fields such as name and image.
	// Rather than use TestRegistry, use a single-token name and expect
	// the DefaultRegistry to be prepended.
	client := fn.New(fn.WithRegistry("alice"))
	var f fn.Function
	var err error
	if _, f, err = client.New(context.Background(), fn.Function{Runtime: TestRuntime, Root: root}); err != nil {
		t.Fatal(err)
	}

	// Expected image is [DefaultRegistry]/[namespace]/[servicename]:latest
	expected := fn.DefaultRegistry + "/alice/" + f.Name + ":latest"
	if f.Build.Image != expected {
		t.Fatalf("expected image '%v' got '%v'", expected, f.Build.Image)
	}
}

// TestClient_New_Delegation ensures that Create invokes each of the individual
// subcomponents via delegation through Build, Push and
// Deploy (and confirms expected fields calculated).
func TestClient_New_Delegation(t *testing.T) {
	var (
		root          = "testdata/example.com/test-new-delegates" // .. in which to initialize
		expectedName  = "test-new-delegates"                      // expected to be derived
		expectedImage = "example.com/alice/test-new-delegates:latest"
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
		if f.Build.Image != expectedImage {
			t.Fatalf("pusher expected image '%v', got '%v'", expectedImage, f.Build.Image)
		}
		return "", nil
	}

	deployer.DeployFn = func(_ context.Context, f fn.Function) (res fn.DeploymentResult, err error) {
		if f.Name != expectedName {
			t.Fatalf("deployer expected name '%v', got '%v'", expectedName, f.Name)
		}
		if f.Build.Image != expectedImage {
			t.Fatalf("deployer expected image '%v', got '%v'", expectedImage, f.Build.Image)
		}
		return
	}

	// Invocation
	// -------------

	// Invoke the creation, triggering the function delegates, and
	// perform follow-up assertions that the functions were indeed invoked.
	if _, _, err := client.New(context.Background(), fn.Function{Runtime: TestRuntime, Root: root}); err != nil {
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

// TestClient_Run ensures that the runner is invoked with the path requested.
// Implicitly checks that the stop fn returned also is respected.
// See TestClient_Runner for the test of the default runner implementation.
func TestClient_Run(t *testing.T) {
	// Create the root function directory
	root := "testdata/example.com/test-run"
	defer Using(t, root)()

	// client with the mock runner and the new test function
	runner := mock.NewRunner()
	client := fn.New(fn.WithRegistry(TestRegistry), fn.WithRunner(runner))
	var f fn.Function
	var err error
	if _, f, err = client.New(context.Background(), fn.Function{Runtime: TestRuntime, Root: root}); err != nil {
		t.Fatal(err)
	}

	// Run the newly created function
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	job, err := client.Run(ctx, f)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = job.Stop() }()

	// Assert the runner was invoked, and with the expected root.
	if !runner.RunInvoked {
		t.Fatal("run did not invoke the runner")
	}
	if runner.RootRequested != root {
		t.Fatalf("expected path '%v', got '%v'", root, runner.RootRequested)
	}
}

// TestClient_Runner ensures that the default internal runner correctly executes
// a scaffolded function.
func TestClient_Runner(t *testing.T) {
	// This integration test explicitly requires the "host" builder due to its
	// lack of a dependency on a container runtime, and the other builders not
	// taking advantage of Scaffolding (expected by this runner).
	// See E2E tests for testing of running functions built using Pack or S2I and
	// which are dependent on Podman or Docker.
	// Currently only a Go function is tested because other runtimes do not yet
	// have scaffolding.

	root, cleanup := Mktemp(t)
	defer cleanup()
	ctx, cancel := context.WithCancel(context.Background())
	client := fn.New(fn.WithBuilder(oci.NewBuilder("", true)), fn.WithVerbose(true))

	// Initialize
	f, err := client.Init(fn.Function{Root: root, Runtime: "go", Registry: TestRegistry})
	if err != nil {
		t.Fatal(err)
	}

	// Build
	if f, err = client.Build(ctx, f, fn.BuildWithPlatforms(TestPlatforms)); err != nil {
		t.Fatal(err)
	}

	// Run
	job, err := client.Run(ctx, f)
	if err != nil {
		t.Fatal(err)
	}

	// Invoke
	resp, err := http.Get(fmt.Sprintf("http://%s:%s", job.Host, job.Port))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Fatalf("unexpected response code: %v", resp.StatusCode)
	}

	cancel()
}

// TestClient_Run_DataDir ensures that when a function is created, it also
// includes a .func (runtime data) directory which is registered as ignored for
// functions which will be tracked in git source control.
// Note that this test is somewhat testing an implementation detail of `.Run(`
// (it writes runtime data to files in .func) but since the feature of adding
// .func to .gitignore is an important externally visible "feature", an explicit
// test is warranted.
func TestClient_Run_DataDir(t *testing.T) {
	root := "testdata/example.com/test-run-data-dir"
	defer Using(t, root)()

	// Create a function at root
	client := fn.New(fn.WithRegistry(TestRegistry))
	if _, _, err := client.New(context.Background(), fn.Function{Root: root, Runtime: TestRuntime}); err != nil {
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

// TestClient_RunTimeout ensures that the run task bubbles a timeout
// error if the function does not report ready within the allotted timeout.
func TestClient_RunTimeout(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	root, cleanup := Mktemp(t)
	defer cleanup()

	// A client with a shorter global timeout.
	client := fn.New(
		fn.WithBuilder(oci.NewBuilder("", true)),
		fn.WithVerbose(true),
		fn.WithStartTimeout(2*time.Second))

	// Initialize
	f, err := client.Init(fn.Function{Root: root, Runtime: "go", Registry: TestRegistry})
	if err != nil {
		t.Fatal(err)
	}

	// Replace the implementation with the test implementation which will
	// return a non-200 response for the first 10 seconds.  This confirms
	// the client is waiting and retrying.
	// TODO: we need an init option which skips writing example source-code.
	_ = os.Remove(filepath.Join(root, "function.go"))
	_ = os.Remove(filepath.Join(root, "function_test.go"))
	_ = os.Remove(filepath.Join(root, "handle.go"))
	_ = os.Remove(filepath.Join(root, "handle_test.go"))
	src, err := os.Open(filepath.Join(cwd, "testdata", "testClientRunTimeout", "f.go"))
	if err != nil {
		t.Fatal(err)
	}
	dst, err := os.Create(filepath.Join(root, "f.go"))
	if err != nil {
		t.Fatal(err)
	}

	if _, err = io.Copy(dst, src); err != nil {
		t.Fatal(err)
	}
	src.Close()
	dst.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Build
	if f, err = client.Build(ctx, f, fn.BuildWithPlatforms(TestPlatforms)); err != nil {
		t.Fatal(err)
	}

	// Run
	// with a fairly short timeout so as not to hold up tests.
	_, err = client.Run(ctx, f, fn.RunWithStartTimeout(1*time.Second))
	if !errors.As(err, &fn.ErrRunTimeout{}) {
		t.Fatalf("did not receive ErrRunTimeout.  Got %v", err)
	}
}

// TestClient_Update ensures that updating invokes the build/push/deploy
// process, erroring if run on a directory uncreated.
func TestClient_Update(t *testing.T) {
	var (
		root          = "testdata/example.com/test-update"
		expectedName  = "test-update"
		expectedImage = "example.com/alice/test-update:latest"
		builder       = mock.NewBuilder()
		pusher        = mock.NewPusher()
		deployer      = mock.NewDeployerWithResult(fn.DeploymentResult{
			Status:    fn.Deployed,
			URL:       "example.com",
			Namespace: "test-ns",
		})
		deployerUpdated = mock.NewDeployerWithResult(fn.DeploymentResult{
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
	var f fn.Function
	var err error
	if _, f, err = client.New(context.Background(), fn.Function{Runtime: TestRuntime, Root: root}); err != nil {
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
		if f.Build.Image != expectedImage {
			t.Fatalf("pusher expected image '%v', got '%v'", expectedImage, f.Build.Image)
		}
		// image of given name wouold be pushed to the configured registry.
		return "", nil
	}

	// Update whose implementaiton verifed the expected name and image
	deployer.DeployFn = func(_ context.Context, f fn.Function) (res fn.DeploymentResult, err error) {
		if f.Name != expectedName {
			t.Fatalf("updater expected name '%v', got '%v'", expectedName, f.Name)
		}
		if f.Build.Image != expectedImage {
			t.Fatalf("updater expected image '%v', got '%v'", expectedImage, f.Build.Image)
		}
		return
	}

	// Invoke the creation, triggering the function delegates, and
	// perform follow-up assertions that the functions were indeed invoked.
	if f, err = client.Deploy(context.Background(), f); err != nil {
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
	if _, err = client.Deploy(context.Background(), f); err != nil {
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
// its image member on initial deploy, and on subsequent deploys where f.Image
// takes precedence
func TestClient_Deploy_RegistryUpdate(t *testing.T) {
	root, rm := Mktemp(t)
	defer rm()
	client := fn.New(fn.WithRegistry("example.com/alice"))

	// New runs build and deploy, thus the initial instantiation should result in
	// the member being populated from the client's registry and function name.
	var f fn.Function
	var err error
	if _, f, err = client.New(context.Background(), fn.Function{Runtime: "go", Name: "f", Root: root}); err != nil {
		t.Fatal(err)
	}
	if f.Build.Image != "example.com/alice/f:latest" {
		t.Error("image was not built")
	}

	// Updating the registry and performing a subsequent update should not result
	// in the image member being updated to the new value: registry is only used
	// when calculating a nonexistent value
	f.Registry = "example.com/bob"
	if f, err = client.Build(context.Background(), f); err != nil {
		t.Fatal(err)
	}
	// if f, err = client.Deploy(context.Background(), f); err != nil {
	// 	t.Fatal(err)
	// }
	expected := "example.com/bob/f:latest"
	if f.Build.Image != expected { // CHANGE to bob since its the first f.Registry
		t.Errorf("expected image name to change to '%v', but got '%v'", expected, f.Build.Image)
	}

	// Set the value of .Image which should override current image
	f.Image = "example.com/fred/f:latest"
	if f, err = client.Build(context.Background(), f); err != nil {
		t.Fatal(err)
	}
	// if f, err = client.Deploy(context.Background(), f); err != nil {
	// 	t.Fatal(err)
	// }
	expected = "example.com/fred/f:latest"
	if f.Build.Image != expected { // DOES change to bob
		t.Errorf("expected image name to change to '%v', but got '%v'", expected, f.Build.Image)
	}

	// Set the value of f.Image to "" to ensure the registry is used for new
	// image calculation
	f.Image = ""
	// f.Registry is "example.com/bob"

	if f, err = client.Build(context.Background(), f); err != nil {
		t.Fatal(err)
	}

	expected = "example.com/bob/f:latest"
	if f.Build.Image != expected {
		t.Errorf("expected image name to change to '%v', but got '%v'", expected, f.Build.Image)
	}
}

// TestClient_Deploy_NamespaceUpdate ensures that namespace deployment has
// the correct priorities, that means:
// 'default' gets overridden by 'already deployed' if aplicable and all gets
// overridden by 'specifically desired namespace'.
func TestClient_Deploy_NamespaceUpdate(t *testing.T) {
	root, rm := Mktemp(t)
	defer rm()

	var (
		ctx      = context.Background()
		deployer = mock.NewDeployer()
		f        fn.Function
		err      error
	)

	client := fn.New(
		fn.WithRegistry("example.com/alice"),
		fn.WithDeployer(deployer),
	)

	// New runs build and deploy, thus the initial instantiation should result in
	// the namespace member being populated into the most default namespace
	if _, f, err = client.New(ctx, fn.Function{Runtime: "go", Name: "f", Root: root}); err != nil {
		t.Fatal(err)
	}
	if f.Deploy.Namespace == "" {
		t.Fatal("namespace should be populated in deployer when initially undefined")
	}

	// change deployed namespace to simulate already deployed function -- should
	// take precedence
	f.Deploy.Namespace = "alreadydeployed"
	f, err = client.Deploy(ctx, f)
	if err != nil {
		t.Fatal(err)
	}

	if f.Deploy.Namespace != "alreadydeployed" {
		err = fmt.Errorf("namespace should match the already deployed function ns")
		t.Fatal(err)
	}

	// desired namespace takes precedence
	f.Namespace = "desiredns"
	f, err = client.Deploy(ctx, f)
	if err != nil {
		t.Fatal(err)
	}
}

// TestClient_Remove_ByPath ensures that the remover is invoked to remove
// the function with the name of the function at the provided root.
func TestClient_Remove_ByPath(t *testing.T) {
	var (
		root         = "testdata/example.com/test-remove-by-path"
		expectedName = "test-remove-by-path"
		remover      = mock.NewRemover()
		namespace    = "func"
	)

	defer Using(t, root)()

	client := fn.New(
		fn.WithRegistry(TestRegistry),
		fn.WithRemover(remover))

	var f fn.Function
	var err error
	if _, f, err = client.New(context.Background(), fn.Function{Runtime: TestRuntime, Root: root, Namespace: namespace}); err != nil {
		t.Fatal(err)
	}

	remover.RemoveFn = func(name, _ string) error {
		if name != expectedName {
			t.Fatalf("Expected to remove '%v', got '%v'", expectedName, name)
		}
		return nil
	}

	if err := client.Remove(context.Background(), f, false); err != nil {
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
		root              = "testdata/example.com/test-remove-delete-all"
		expectedName      = "test-remove-delete-all"
		remover           = mock.NewRemover()
		pipelinesProvider = mock.NewPipelinesProvider()
		deleteAll         = true
		namespace         = "func"
	)

	defer Using(t, root)()

	client := fn.New(
		fn.WithRegistry(TestRegistry),
		fn.WithRemover(remover),
		fn.WithPipelinesProvider(pipelinesProvider))

	var f fn.Function
	var err error
	if _, f, err = client.New(context.Background(), fn.Function{Runtime: TestRuntime, Root: root, Namespace: namespace}); err != nil {
		t.Fatal(err)
	}

	remover.RemoveFn = func(name, _ string) error {
		if name != expectedName {
			t.Fatalf("Expected to remove '%v', got '%v'", expectedName, name)
		}
		return nil
	}

	if err := client.Remove(context.Background(), f, deleteAll); err != nil {
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
		root              = "testdata/example.com/test-remove-dont-delete-all"
		expectedName      = "test-remove-dont-delete-all"
		remover           = mock.NewRemover()
		pipelinesProvider = mock.NewPipelinesProvider()
		deleteAll         = false
		namespace         = "func"
	)

	defer Using(t, root)()

	client := fn.New(
		fn.WithRegistry(TestRegistry),
		fn.WithRemover(remover),
		fn.WithPipelinesProvider(pipelinesProvider))

	var f fn.Function
	var err error
	if _, f, err = client.New(context.Background(), fn.Function{Runtime: TestRuntime, Root: root, Namespace: namespace}); err != nil {
		t.Fatal(err)
	}

	remover.RemoveFn = func(name, _ string) error {
		if name != expectedName {
			t.Fatalf("Expected to remove '%v', got '%v'", expectedName, name)
		}
		return nil
	}

	if err := client.Remove(context.Background(), f, deleteAll); err != nil {
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
// of the name provided, with precedence over a provided root path.
func TestClient_Remove_ByName(t *testing.T) {
	var (
		root         = "testdata/example.com/testRemoveByName"
		expectedName = "explicitName.example.com"
		remover      = mock.NewRemover()
		namespace    = "func"
	)

	defer Using(t, root)()

	client := fn.New(
		fn.WithRegistry(TestRegistry),
		fn.WithRemover(remover))

	if _, err := client.Init(fn.Function{Runtime: TestRuntime, Root: root}); err != nil {
		t.Fatal(err)
	}

	remover.RemoveFn = func(name, _ string) error {
		if name != expectedName {
			t.Fatalf("Expected to remove '%v', got '%v'", expectedName, name)
		}
		return nil
	}

	// Run remove with name (and namespace in .Deploy to simulate deployed function)
	if err := client.Remove(context.Background(), fn.Function{Name: expectedName, Deploy: fn.DeploySpec{Namespace: namespace}}, false); err != nil {
		t.Fatal(err)
	}

	// Run remove with a name and a root, which should be ignored in favor of the name.
	if err := client.Remove(context.Background(), fn.Function{Name: expectedName, Root: root, Deploy: fn.DeploySpec{Namespace: namespace}}, false); err != nil {
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
	remover.RemoveFn = func(name, _ string) error {
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
// effective client registry; that the value of f.Image will take precedence
// over .Registry, which is used to calculate a default value for image.
func TestClient_Deploy_Image(t *testing.T) {
	root, rm := Mktemp(t)
	defer rm()

	client := fn.New(
		fn.WithBuilder(mock.NewBuilder()),
		fn.WithDeployer(mock.NewDeployer()),
		fn.WithRegistry("example.com/alice"))

	f, err := client.Init(fn.Function{Name: "myfunc", Runtime: "go", Root: root})
	if err != nil {
		t.Fatal(err)
	}

	// Upon initial creation, the value of .Image is empty
	if f.Build.Image != "" {
		t.Fatalf("new function should have no image, got '%v'", f.Build.Image)
	}

	// Upon deployment, the function should be populated;
	if f, err = client.Build(context.Background(), f); err != nil {
		t.Fatal(err)
	}
	if f, err = client.Deploy(context.Background(), f); err != nil {
		t.Fatal(err)
	}
	expected := "example.com/alice/myfunc:latest"
	if f.Build.Image != expected {
		t.Fatalf("expected image '%v', got '%v'", expected, f.Build.Image)
	}
	expected = "example.com/alice"
	if f.Registry != "example.com/alice" {
		t.Fatalf("expected registry '%v', got '%v'", expected, f.Registry)
	}

	// The value of .Image always takes precedence
	f.Image = "registry2.example.com/bob/myfunc:latest"
	if err = f.Write(); err != nil {
		t.Fatal(err)
	}
	if f, err = client.Build(context.Background(), f); err != nil {
		t.Fatal(err)
	}
	if f, err = client.Deploy(context.Background(), f); err != nil {
		t.Fatal(err)
	}
	expected = "registry2.example.com/bob/myfunc:latest"
	if f.Build.Image != expected {
		t.Fatalf("expected image '%v', got '%v'", expected, f.Build.Image)
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
// effective client registry; that the value of f.Image will take precedence
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

	f, err := client.Init(f)
	if err != nil {
		t.Fatal(err)
	}

	// Upon initial creation, the value of .Build.Image is empty and .Deploy.Image
	// is empty because Function is not deployed yet.
	if f.Build.Image != "" && f.Deploy.Image != "" {
		t.Fatalf("new function should have no image, got '%v'", f.Build.Image)
	}

	// Upon pipeline run, the .Deploy.Image should be populated
	if f, err = client.RunPipeline(context.Background(), f); err != nil {
		t.Fatal(err)
	}
	expected := "example.com/alice/myfunc:latest"
	if f.Deploy.Image != expected {
		t.Fatalf("expected image '%v', got '%v'", expected, f.Deploy.Image)
	}
	expected = "example.com/alice"
	if f.Registry != expected {
		t.Fatalf("expected registry '%v', got '%v'", expected, f.Registry)
	}

	// The value of .Image always takes precedence
	f.Image = "registry2.example.com/bob/myfunc:latest"
	if err = f.Write(); err != nil {
		t.Fatal(err)
	}
	// Upon pipeline run, the function should be populated;
	if f, err = client.RunPipeline(context.Background(), f); err != nil {
		t.Fatal(err)
	}
	expected = "registry2.example.com/bob/myfunc:latest"

	if f.Deploy.Image != expected {
		t.Fatalf("expected image '%v', got '%v'", expected, f.Deploy.Image)
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

// TestClient_Pipelines_Deploy_Namespace ensures that correct namespace is returned
// when using remote deployment
func TestClient_Pipelines_Deploy_Namespace(t *testing.T) {
	root, rm := Mktemp(t)
	defer rm()

	pprovider := mock.NewPipelinesProvider()
	pprovider.RunFn = func(f fn.Function) (string, string, error) {
		// simulate function getting deployed here and return namespace
		return "", f.Namespace, nil
	}

	client := fn.New(
		fn.WithPipelinesProvider(pprovider),
		fn.WithRegistry("example.com/alice"))

	f := fn.Function{
		Name:      "myfunc",
		Runtime:   "node",
		Root:      root,
		Namespace: "myns",
		Build: fn.BuildSpec{
			Git: fn.Git{URL: "http://example-git.com/alice/myfunc.git"},
		},
	}

	f, err := client.Init(f)
	if err != nil {
		t.Fatal(err)
	}

	if f, err = client.RunPipeline(context.Background(), f); err != nil {
		t.Fatal(err)
	}

	// function is deployed in correct ns
	if f.Deploy.Namespace != "myns" {
		t.Fatalf("expected namespace to be '%s' but is '%s'", "myns", f.Deploy.Namespace)
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
	f, err := client.Init(fn.Function{Runtime: TestRuntime, Root: root})
	if err != nil {
		t.Fatal(err)
	}

	// Now try to deploy it.  Ie. without having run the necessary build step.
	_, err = client.Deploy(context.Background(), f)
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
	root := "testdata/example.com/test-configured-builders" // Root from which to run the test
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
	var f1 fn.Function
	var err error
	if _, f1, err = client.New(context.Background(), f0); err != nil {
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
	root := "testdata/example.com/test-configured-buildpacks" // Root from which to run the test
	defer Using(t, root)()

	buildpacks := []string{
		"docker.io/example/custom-buildpack",
	}
	client := fn.New(fn.WithRegistry(TestRegistry))
	var f fn.Function
	var err error
	if _, f, err = client.New(context.Background(), fn.Function{
		Runtime: TestRuntime,
		Root:    root,
		Build: fn.BuildSpec{
			Buildpacks: buildpacks,
		},
	}); err != nil {
		t.Fatal(err)
	}

	// Assert that our custom buildpacks were set
	if !reflect.DeepEqual(f.Build.Buildpacks, buildpacks) {
		t.Fatalf("Expected %v but got %v", buildpacks, f.Build.Buildpacks)
	}
}

// TestClient_Scaffold ensures that scaffolding a function writes its
// scaffolding code to the given directory correctly, including not listing
// the scaffolding directory as a template (it's a special reserved word).
func TestClient_Scaffold(t *testing.T) {
	root, rm := Mktemp(t)
	defer rm()
	var out = "result"

	// Assert "scaffolding" is a reserved word; not listed as aavailable
	// template despite being in the templates' directory.
	client := fn.New()
	tt, err := client.Templates().List("go")
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range tt {
		if v == "scaffolding" {
			t.Fatal("scaffolding is a reserved word and should not be listed as an available template")
		}
	}

	// Create a Golang function in root and scaffold.
	f, err := client.Init(fn.Function{Root: root, Runtime: "go"})
	if err != nil {
		t.Fatal(err)
	}
	if err := client.Scaffold(context.Background(), f, filepath.Join(root, out)); err != nil {
		t.Fatal(err)
	}

	// Test for the existence of the main.go file we know only exists in the Go
	// scaffolding.
	//
	// TODO: This is admittedly a quick way to check that it was scaffolded, which
	// creates a dependency between this test and the implementation of the go
	// scaffolding internals.  A better way would perhaps to be to actually try
	// to run the scaffolded function; but that's precisely what integration tests
	// do, so this expedient is probably passable.
	if _, err := os.Stat(filepath.Join(root, out, "main.go")); err != nil {
		t.Fatalf("error checking for 'main.go' in the scaffolded Go project. %v", err)
	}
}

// TestClient_Runtimes ensures that the total set of runtimes are returned.
func TestClient_Runtimes(t *testing.T) {
	// TODO: test when a specific repo override is indicated
	// (remote repo which takes precedence over embedded and extended)

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
	root := "testdata/example.com/test-create-stamp"
	defer Using(t, root)()

	start := time.Now()

	client := fn.New(fn.WithRegistry(TestRegistry))

	var f fn.Function
	var err error
	if _, f, err = client.New(context.Background(), fn.Function{Runtime: TestRuntime, Root: root}); err != nil {
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
	root := "testdata/example.com/test-invoke-http"
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
	runner.RunFn = func(ctx context.Context, f fn.Function, _ time.Duration) (*fn.Job, error) {
		_, p, _ := net.SplitHostPort(l.Addr().String())
		errs := make(chan error, 10)
		stop := func() error { return nil }
		return fn.NewJob(f, "127.0.0.1", p, errs, stop, false)
	}
	client := fn.New(fn.WithRegistry(TestRegistry), fn.WithRunner(runner))

	// Create a new default HTTP function
	f := fn.Function{Runtime: TestRuntime, Root: root, Template: "http"}
	if _, f, err = client.New(context.Background(), f); err != nil {
		t.Fatal(err)
	}

	// Run the function
	job, err := client.Run(context.Background(), f)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = job.Stop() })
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
	root := "testdata/example.com/test-invoke-cloud-event"
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
	runner.RunFn = func(ctx context.Context, f fn.Function, _ time.Duration) (*fn.Job, error) {
		_, p, _ := net.SplitHostPort(l.Addr().String())
		errs := make(chan error, 10)
		stop := func() error { return nil }
		return fn.NewJob(f, "127.0.0.1", p, errs, stop, false)
	}
	client := fn.New(fn.WithRegistry(TestRegistry), fn.WithRunner(runner))

	// Create a new default CloudEvents function
	f := fn.Function{Runtime: TestRuntime, Root: root, Template: "cloudevents"}
	if _, f, err = client.New(context.Background(), f); err != nil {
		t.Fatal(err)
	}

	// Run the function
	job, err := client.Run(context.Background(), f)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = job.Stop() }()

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
	root := "testdata/example.com/test-instances"
	defer Using(t, root)()

	// A mock runner
	runner := mock.NewRunner()
	runner.RunFn = func(_ context.Context, f fn.Function, _ time.Duration) (*fn.Job, error) {
		errs := make(chan error, 10)
		stop := func() error { return nil }
		return fn.NewJob(f, "127.0.0.1", "8080", errs, stop, false)
	}

	// Client with the mock runner
	client := fn.New(fn.WithRegistry(TestRegistry), fn.WithRunner(runner))

	// Create the new function
	var f fn.Function
	var err error
	if _, f, err = client.New(context.Background(), fn.Function{Root: root, Runtime: TestRuntime}); err != nil {
		t.Fatal(err)
	}

	// Run the function, awaiting start and then canceling
	job, err := client.Run(context.Background(), f)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = job.Stop() }()

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

	f, err := fn.NewFunction(root)
	if err != nil {
		t.Fatal(err)
	}
	// paths that do not contain a function are !Built - Degenerate case
	if f.Built() {
		t.Fatal("path not containing a function returned as being built")
	}

	// a freshly-created function should be !Built
	f, err = client.Init(fn.Function{Runtime: TestRuntime, Root: root})
	if err != nil {
		t.Fatal(err)
	}
	if f.Built() {
		t.Fatal("newly created function returned Built==true")
	}

	// a function which was successfully built should return as being Built
	if f, err = client.Build(context.Background(), f); err != nil {
		t.Fatal(err)
	}
	if !f.Built() {
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
	f, err := client.Init(fn.Function{Runtime: TestRuntime, Root: root})
	if err != nil {
		t.Fatal(err)
	}

	// A freshly created function should have the latest migration
	if f.SpecVersion != fn.LastSpecVersion() {
		t.Fatal("freshly created function should have the latest migration")
	}
}

// TestClient_RunReadiness ensures that the run task awaits a ready response
// from the job before returning.
func TestClient_RunRediness(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	root, cleanup := Mktemp(t)
	defer cleanup()

	client := fn.New(fn.WithBuilder(oci.NewBuilder("", true)), fn.WithVerbose(true))

	// Initialize
	f, err := client.Init(fn.Function{Root: root, Runtime: "go", Registry: TestRegistry})
	if err != nil {
		t.Fatal(err)
	}

	// Replace the implementation with the test implementation which will
	// return a non-200 response for the first few seconds.  This confirms
	// the client is waiting and retrying.
	// TODO: we need an init option which skips writing example source-code.
	_ = os.Remove(filepath.Join(root, "function.go"))
	_ = os.Remove(filepath.Join(root, "function_test.go"))
	_ = os.Remove(filepath.Join(root, "handle.go"))
	_ = os.Remove(filepath.Join(root, "handle_test.go"))
	src, err := os.Open(filepath.Join(cwd, "testdata", "testClientRunReadiness", "f.go"))
	if err != nil {
		t.Fatal(err)
	}
	dst, err := os.Create(filepath.Join(root, "f.go"))
	if err != nil {
		t.Fatal(err)
	}

	if _, err = io.Copy(dst, src); err != nil {
		t.Fatal(err)
	}
	src.Close()
	dst.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Build
	if f, err = client.Build(ctx, f, fn.BuildWithPlatforms(TestPlatforms)); err != nil {
		t.Fatal(err)
	}

	// Run
	// The function returns a non-200 from its readiness handler at first.
	// Since we already confirmed in another test that a timeout awaiting a
	// 200 response from this endpoint does indeed fail the run task, this
	// delayed 200 confirms there is a retry in place.
	job, err := client.Run(ctx, f)
	if err != nil {
		t.Fatal(err)
	}
	if err := job.Stop(); err != nil {
		t.Fatalf("err on job stop. %v", err)
	}
}

// TestClient_BuildCleanFingerprint ensures that when building a Function the
// source controlled state is not modified (git would show no unstaged changes).
// For example, the image name generated when building should not be stored
// in function metadata that is checked into source control (func.yaml).
func TestClient_BuildCleanFingerprint(t *testing.T) {

	// Create a temporary directory
	root, cleanup := Mktemp(t)
	defer cleanup()

	// create new client
	client := fn.New()

	f := fn.Function{Root: root, Runtime: TestRuntime, Registry: TestRegistry}
	ctx := context.Background()

	// init a new Function
	f, err := client.Init(f)
	if err != nil {
		t.Fatal(err)
	}

	// NOTE: Practically one would initialize a git repository, check the source code
	// and compare that way. For now this only compares fingerprint before and after
	// building Function

	// get fingerprint before building
	hashA, _, err := fn.Fingerprint(root)
	if err != nil {
		t.Fatal(err)
	}

	// Build a function
	if f, err = client.Build(ctx, f); err != nil {
		t.Fatal(err)
	}

	// Write to disk because client.Build just stamps (writing is handled in its caller)
	if err = f.Write(); err != nil {
		t.Fatal(err)
	}

	// compare fingerprints before and after
	hashB, _, err := fn.Fingerprint(root)
	if err != nil {
		t.Fatal(err)
	}
	if hashA != hashB {
		t.Fatal("just building a Function resulted in a dirty function state (fingerprint changed)")
	}
}

// TestClient_DeployRemoves ensures that the Remover is invoked when a
// function is moved to a new namespace.
// specifically: deploy to 'nsone' -> simulate change of namespace with change to
// f.Namespace -> redeploy to that namespace and expect the remover to be invoked
// for old Function in ns 'nsone'.
func TestClient_DeployRemoves(t *testing.T) {
	// Create a temporary directory
	root, cleanup := Mktemp(t)
	defer cleanup()

	var (
		ctx      = context.Background()
		nsOne    = "nsone"
		nsTwo    = "nstwo"
		testname = "testfunc"
		remover  = mock.NewRemover()
	)

	remover.RemoveFn = func(n, ns string) error {
		if ns != nsOne {
			t.Fatalf("expected delete namespace %v, got %v", nsOne, ns)
		}
		if n != testname {
			t.Fatalf("expected delete name %v, got %v", testname, n)
		}
		return nil
	}

	client := fn.New(fn.WithRemover(remover))
	// initialize function with namespace defined as nsone

	f, err := client.Init(fn.Function{Runtime: "go", Root: root,
		Namespace: nsOne, Name: testname, Registry: TestRegistry})
	if err != nil {
		t.Fatal(err)
	}

	f, err = client.Build(ctx, f)
	if err != nil {
		t.Fatal(err)
	}

	// first deploy
	f, err = client.Deploy(ctx, f)
	if err != nil {
		t.Fatal(err)
	}

	// change namespace
	f.Namespace = nsTwo

	// redeploy to different namespace
	f, err = client.Deploy(ctx, f)
	if err != nil {
		t.Fatal(err)
	}

	// check that a remove was invoked getting rid of the old Function
	if !remover.RemoveInvoked {
		t.Fatal(fmt.Errorf("remover was not invoked on an old function"))
	}
}

// TestClient_BuildPopulatesRuntimeImage ensures that building populates runtime
// metadata (.func/built-image) image.
func TestClient_BuildPopulatesRuntimeImage(t *testing.T) {
	// Create a temporary directory
	root, cleanup := Mktemp(t)
	defer cleanup()

	client := fn.New()
	f, err := client.Init(fn.Function{Runtime: "go", Root: root, Registry: TestRegistry})
	if err != nil {
		t.Fatal(err)
	}

	expect := f.Registry + "/" + f.Name + ":latest"

	f, err = client.Build(context.Background(), f)
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path.Join(root, ".func/built-image"))
	if err != nil {
		t.Fatal(err)
	}

	if string(got) != expect {
		t.Fatalf("written image in ./.func/built-image '%s' does not match expected '%s'", got, expect)
	}
}
