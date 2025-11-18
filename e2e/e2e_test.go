//go:build e2e
// +build e2e

/*
Package e2e provides an end-to-end test suite for the Functions CLI "func".

See README.md for more details.
*/
package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	fn "knative.dev/func/pkg/functions"
	"knative.dev/func/pkg/knative"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"knative.dev/func/pkg/k8s"
)

const (
	// DefaultBin is the default binary to run, relative to this test file.
	// This is the binary built by default when running 'make'.
	// This can be customized with FUNC_E2E_BIN.
	// NOte this is always relative to this test file.
	DefaultBin = "../func"

	// DefaultClean indicates whether or not tests should clean up after
	// themselves by deleting Function instances created during run.
	// Setting this to false significantly increases testing speed, but
	// results in lingering function instances after at test run.  Set to
	// "false" when expecting the test cluster to be removed after a test run,
	// such as in CI, but set to "true" for development, when the same test
	// cluster may be used across multiple test runs during debugging.
	DefaultClean = true

	// DefaultCleanImages indicates whether or not tests should clean up container
	// images and volumes after themselves. This is separate from DefaultClean
	// which handles cluster resources. This is necessary in CI which quickly
	// runs out of disk space during Quarkus and Springboot builds.
	// Disable with FUNC_E2E_CLEAN_IMAGES="false".
	DefaultCleanImages = true

	// DefaultGocoverdir defines the default path to use for the GOCOVERDIR
	// while executing tests.  This value can be altered using
	// FUNC_E2E_GOCOVERDIR. While this value could be passed through using
	// its original environment variable name "GOCOVERDIR", to keep with the
	// isolation of environment provided for all other values, this one is
	// likewise also isolated using the "FUNC_E2E_" prefix.
	DefaultGocoverdir = "../.coverage"

	// DefaultKubeconfig is the default path (relative to this test file) at
	// which the kubeconfig can be found which was created when setting up
	// a local test cluster using the cluster.sh script.  This can be
	// overridden using FUNC_E2E_KUBECONFIG.
	DefaultKubeconfig = "../hack/bin/kubeconfig.yaml"

	// DefaultNamespace for E2E tests. Defaults to "default" but can be
	// overridden using FUNC_E2E_NAMESPACE environment variable.
	DefaultNamespace = "default"

	// DefaultRegistry to use when running the e2e tests.  This is the URL
	// of the registry created by default when using the cluster.sh script
	// to set up a local testing cluster, but can be customized with
	// FUNC_E2E_REGISTRY.
	DefaultRegistry = "localhost:50000/func"

	// DefaultVerbose sets the default for the --verbose flag of all commands.
	DefaultVerbose = false

	// DefaultTools is the path to supporting tools.
	DefaultTools = "../hack/bin"

	// DefaultTestdata is the path to supporting testdata
	DefaultTestdata = "./testdata"
)

// Final Settings
// Populated during init phase (see init func in Helpers below)
var (
	// Bin is the absolute path to the binary to use when testing.
	// Can be set with FUNC_E2E_BIN.
	Bin string

	// Clean instructs the system to remove resources created during testing.
	// Defaults to tru.  Can be disabled with FUNC_E2E_CLEAN to speed up tests,
	// if the cluster is expected to be removed upon completion (such as in CI)
	Clean bool

	// CleanImages instructs the system to remove container images and volumes
	// created during testing. Separate from Clean which handles cluster resources.
	CleanImages bool

	// DockerHost is the DOCKER_HOST value to use for tests.
	// Can be set with FUNC_E2E_DOCKER_HOST.
	DockerHost string

	// Gocoverdir is the path to the directory which will be used for Go's
	// coverage reporting, provided to the test binary as GOCOVERDIR.  By
	// default the current user's environment is not used, and by default this
	// is set to ../.coverage (as relative to this test file).  This value
	// can be overridden with FUNC_E2E_GOCOVERDIR.
	Gocoverdir string

	// Kubeconfig is the path at which a kubeconfig suitable for running
	// E2E tests can be found.  By default the config located in
	// hack/bin/kubeconfig.yaml will be used.  This is created when running
	// hack/cluster.sh to set up a local test cluster.
	// To avoid confusion, the current user's KUBECONFIG will not be used.
	// Instead, this can be set explicitly using FUNC_E2E_KUBECONFIG.
	Kubeconfig string

	// Matrix indicates a full matrix test should be run.  Defaults to false.
	// Enable with FUNC_E2E_MATRIX=true
	Matrix bool

	// MatrixBuilders specifies builders to check during matrix tests.
	// Can be set with FUNC_E2E_MATRIX_BUILDERS.
	MatrixBuilders = []string{"host", "s2i", "pack"}

	// MatrixRuntimes for which runtime-specific tests should be run.  Defaults
	// to all core language runtimes.  Can be set with FUNC_E2E_MATRIX_RUNTIMES
	MatrixRuntimes = []string{"go", "python", "node", "typescript", "rust", "quarkus", "springboot"}

	// MatrixTemplates specifies the templates to check during matrix tests.
	MatrixTemplates = []string{"http", "cloudevents"}

	// Namespace is the Kubernetes namespace where functions will be deployed
	// during tests. Defaults to "default". When using a custom namespace,
	// ensure DNS is configured for {function}.{namespace}.localtest.me patterns.
	// Can be set with FUNC_E2E_NAMESPACE
	Namespace string

	// Plugin indicates func is being run as a plugin within Bin, and
	// the value of this argument is the subcommand.  For example, when
	// running e2e tests as a plugin to `kn`, Bin will be /path/to/kn and
	// 'Plugin' would be 'func'.  The resultant commands would then be
	//  /path/to/kn func {command}
	// Can be set with FUNC_E2E_PLUGIN
	Plugin string

	// PodmanHost is the DOCKER_HOST value to use specifically for Podman tests.
	// Can be set with FUNC_E2E_PODMAN_HOST.
	PodmanHost string

	// Registry is the container registry to use by default for tests;
	// defaulting to the local container registry set up by the allocation
	// script running on localhost:5000.
	// Can be set with FUNC_E2E_REGISTRY
	Registry string

	// Podman indicates that the Pack and S2I builders should be used and
	// checked with the Podman container engine.
	// Set with FUNC_E2E_PODMAN
	Podman bool = false

	// Verbose mode for all command runs.
	// Set with FUNC_E2E_VERBOSE
	Verbose bool

	// Tools is the path to tools which the E2E tests should use with
	// precedence.  It's a path, and is prepended to PATH.  By default this
	// is ./hack/bin which contains commands installed via ./hack/binaries.sh
	// (and should be of a known compatible version). Set with FUNC_E2E_TOOLS
	Tools string

	// Testdata is the path to the testdata directory, defaulting to ./testdata
	// Set with FUNC_E2E_TESTDATA
	Testdata string
)

// ----------------------------------------------------------------------------
// Test Initialization
// ----------------------------------------------------------------------------
//
// NOTE: Deprecated ENVS for backwards compatibility are mapped as follows:
// OLD                  New                           Final Variable
// ---------------------------------------------------
// E2E_FUNC_BIN      => FUNC_E2E_BIN               => Bin
// E2E_USE_KN_FUNC   => FUNC_E2E_PLUGIN            => Plugin
// E2E_REGISTRY_URL  => FUNC_E2E_REGISTRY          => Registry
// E2E_RUNTIMES      => FUNC_E2E_MATRIX_RUNTIMES   => MatrixRuntimes
//
// init global settings for the current run from environment
// we read E2E config settings passed via the FUNC_E2E_* environment
// variables.  These globals are used when creating test cases.
// Some tests pass these values as flags, sometimes as environment variables,
// sometimes not at all; hence why the actual environment setup is deferred
// into each test, merely reading them in here during E2E process init.
func init() {
	fmt.Fprintln(os.Stderr, "Initializing E2E Tests")
	fmt.Fprintln(os.Stderr, "----------------------")
	// Useful for CI debugging:
	// fmt.Fprintln(os.Stderr, "--  Initial Environment: ")
	// for _, env := range os.Environ() {
	// 	fmt.Println(env)
	// }
	fmt.Fprintln(os.Stderr, "--  Preserved Environment: ")
	fmt.Fprintf(os.Stderr, "  HOME=%v\n", os.Getenv("HOME"))
	fmt.Fprintf(os.Stderr, "  PATH=%v\n", os.Getenv("PATH"))
	fmt.Fprintf(os.Stderr, "  XDG_CONFIG_HOME=%v\n", os.Getenv("XDG_CONFIG_HOME"))
	fmt.Fprintf(os.Stderr, "  XDG_RUNTIME_DIR=%v\n", os.Getenv("XDG_RUNTIME_DIR"))
	fmt.Fprintln(os.Stderr, "--  Config Provided: ")
	fmt.Fprintf(os.Stderr, "  FUNC_E2E_BIN=%v\n", os.Getenv("FUNC_E2E_BIN"))
	fmt.Fprintf(os.Stderr, "  FUNC_E2E_CLEAN=%v\n", os.Getenv("FUNC_E2E_CLEAN"))
	fmt.Fprintf(os.Stderr, "  FUNC_E2E_CLEAN_IMAGES=%v\n", os.Getenv("FUNC_E2E_CLEAN_IMAGES"))
	fmt.Fprintf(os.Stderr, "  FUNC_E2E_DOCKER_HOST=%v\n", os.Getenv("FUNC_E2E_DOCKER_HOST"))
	fmt.Fprintf(os.Stderr, "  FUNC_E2E_GOCOVERDIR=%v\n", os.Getenv("FUNC_E2E_GOCOVERDIR"))
	fmt.Fprintf(os.Stderr, "  FUNC_E2E_HOME=%v\n", os.Getenv("FUNC_E2E_HOME"))
	fmt.Fprintf(os.Stderr, "  FUNC_E2E_KUBECONFIG=%v\n", os.Getenv("FUNC_E2E_KUBECONFIG"))
	fmt.Fprintf(os.Stderr, "  FUNC_E2E_MATRIX=%v\n", os.Getenv("FUNC_E2E_MATRIX"))
	fmt.Fprintf(os.Stderr, "  FUNC_E2E_MATRIX_BUILDERS=%v\n", os.Getenv("FUNC_E2E_MATRIX_BUILDERS"))
	fmt.Fprintf(os.Stderr, "  FUNC_E2E_MATRIX_RUNTIMES=%v\n", os.Getenv("FUNC_E2E_MATRIX_RUNTIMES"))
	fmt.Fprintf(os.Stderr, "  FUNC_E2E_NAMESPACE=%v\n", os.Getenv("FUNC_E2E_NAMESPACE"))
	fmt.Fprintf(os.Stderr, "  FUNC_E2E_PLUGIN=%v\n", os.Getenv("FUNC_E2E_PLUGIN"))
	fmt.Fprintf(os.Stderr, "  FUNC_E2E_PODMAN_HOST=%v\n", os.Getenv("FUNC_E2E_PODMAN_HOST"))
	fmt.Fprintf(os.Stderr, "  FUNC_E2E_REGISTRY=%v\n", os.Getenv("FUNC_E2E_REGISTRY"))
	fmt.Fprintf(os.Stderr, "  FUNC_E2E_PODMAN=%v\n", os.Getenv("FUNC_E2E_PODMAN"))
	fmt.Fprintf(os.Stderr, "  FUNC_E2E_TOOLS=%v\n", os.Getenv("FUNC_E2E_TOOLS"))
	fmt.Fprintf(os.Stderr, "  FUNC_E2E_TESTDATA=%v\n", os.Getenv("FUNC_E2E_TESTDATA"))
	fmt.Fprintf(os.Stderr, "  FUNC_E2E_VERBOSE=%v\n", os.Getenv("FUNC_E2E_VERBOSE"))
	fmt.Fprintf(os.Stderr, "  (deprecated) E2E_FUNC_BIN=%v\n", os.Getenv("E2E_FUNC_BIN"))
	fmt.Fprintf(os.Stderr, "  (deprecated) E2E_REGISTRY_URL=%v\n", os.Getenv("E2E_REGISTRY_URL"))
	fmt.Fprintf(os.Stderr, "  (deprecated) E2E_RUNTIMES=%v\n", os.Getenv("E2E_RUNTIMES"))
	fmt.Fprintf(os.Stderr, "  (deprecated) E2E_USE_KN_FUNC=%v\n", os.Getenv("E2E_USE_KN_FUNC"))

	fmt.Fprintln(os.Stderr, "---------------------")

	// Read all envs into their final variables
	readEnvs()

	fmt.Fprintln(os.Stderr, "Final Config:")
	fmt.Fprintf(os.Stderr, "  Bin=%v\n", Bin)
	fmt.Fprintf(os.Stderr, "  Clean=%v\n", Clean)
	fmt.Fprintf(os.Stderr, "  CleanImages=%v\n", CleanImages)
	fmt.Fprintf(os.Stderr, "  DockerHost=%v\n", DockerHost)
	fmt.Fprintf(os.Stderr, "  Gocoverdir=%v\n", Gocoverdir)
	fmt.Fprintf(os.Stderr, "  Kubeconfig=%v\n", Kubeconfig)
	fmt.Fprintf(os.Stderr, "  Matrix=%v\n", Matrix)
	fmt.Fprintf(os.Stderr, "  MatrixBuilders=%v\n", toCSV(MatrixBuilders))
	fmt.Fprintf(os.Stderr, "  MatrixRuntimes=%v\n", toCSV(MatrixRuntimes))
	fmt.Fprintf(os.Stderr, "  MatrixTemplates=%v\n", toCSV(MatrixTemplates))
	fmt.Fprintf(os.Stderr, "  Namespace=%v\n", Namespace)
	fmt.Fprintf(os.Stderr, "  Plugin=%v\n", Plugin)
	fmt.Fprintf(os.Stderr, "  PodmanHost=%v\n", PodmanHost)
	fmt.Fprintf(os.Stderr, "  Registry=%v\n", Registry)
	fmt.Fprintf(os.Stderr, "  Podman=%v\n", Podman)
	fmt.Fprintf(os.Stderr, "  Tools=%v\n", Tools)
	fmt.Fprintf(os.Stderr, "  Testdata=%v\n", Testdata)
	fmt.Fprintf(os.Stderr, "  Verbose=%v\n", Verbose)

	// Coverage
	// --------
	// Create Gocoverdir if it does not already exist
	if err := os.MkdirAll(Gocoverdir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "error creating coverage directory %q: %v\n", Gocoverdir, err)
	}

	// Version
	fmt.Fprintln(os.Stderr, "---------------------")
	fmt.Fprintln(os.Stderr, "Func Version:")
	printVersion()

	fmt.Fprintln(os.Stderr, "--- init complete ---")
	fmt.Fprintln(os.Stderr, "") // TODO: there is a superfluous linebreak from "func version".  This balances the whitespace.
}

// readEnvs and apply defaults, populating the named global variables with
// the final values which will be used by all tests.
func readEnvs() {
	// Bin - path to binary which will be used when running the tests.
	Bin = getEnvPath("FUNC_E2E_BIN", "E2E_FUNC_BIN", DefaultBin)
	// Final =          current ENV, deprecated ENV, default

	// Clean up deployed functions before starting next test
	Clean = getEnvBool("FUNC_E2E_CLEAN", "", DefaultClean)

	// Clean up container images and volumes after tests
	CleanImages = getEnvBool("FUNC_E2E_CLEAN_IMAGES", "", DefaultCleanImages)

	// DockerHost - the DOCKER_HOST to use for container operations (not including podman-specific tests)
	DockerHost = getEnv("FUNC_E2E_DOCKER_HOST", "", "")

	// Gocoverdir - the coverage directory to use while testing the go binary.
	Gocoverdir = getEnvPath("FUNC_E2E_GOCOVERDIR", "", DefaultGocoverdir)

	// Kubeconfig - the kubeconfig to pass ass KUBECONFIG env to test
	// environments.
	Kubeconfig = getEnvPath("FUNC_E2E_KUBECONFIG", "", DefaultKubeconfig)

	// Matrix - optionally enable matrix test
	Matrix = getEnvBool("FUNC_E2E_MATRIX", "", false)

	// Builders - can optionally pass a list of builders to test, overriding
	// the default of testing all. Example "FUNC_E2E_MATRIX_BUILDERS=pack,s2i"
	MatrixBuilders = getEnvList("FUNC_E2E_MATRIX_BUILDERS", "", toCSV(MatrixBuilders))

	// Runtimes - can optionally pass a list of runtimes to test, overriding
	// the default of testing all builtin runtimes.
	// Example "FUNC_E2E_MATRIX_RUNTIMES=go,python"
	MatrixRuntimes = getEnvList("FUNC_E2E_MATRIX_RUNTIMES", "E2E_RUNTIMES", toCSV(MatrixRuntimes))

	// Templates
	MatrixTemplates = getEnvList("FUNC_E2E_MATRIX_TEMPLATES", "", toCSV(MatrixTemplates))

	// Namespace - the Kubernetes namespace where functions will be deployed
	Namespace = getEnv("FUNC_E2E_NAMESPACE", "", DefaultNamespace)

	// Plugin - if set, func is a plugin and Bin is the one plugging. The value
	// is the name of the subcommand.
	Plugin = getEnv("FUNC_E2E_PLUGIN", "E2E_USE_KN_FUNC", "")
	// Plugin Backwards compatibility:
	// If set to "true", the default value is "func" because the deprecated
	// value was literal string "true".
	if Plugin == "true" {
		Plugin = "func"
	}

	// Podman - optionally enable Podman S2I and Builder test
	Podman = getEnvBool("FUNC_E2E_PODMAN", "", false)

	// PodmanHost - the DOCKER_HOST to use specifically during Podman tests
	// If FUNC_E2E_PODMAN is enabled but FUNC_E2E_PODMAN_HOST is not set,
	// try to auto-detect the Podman socket path
	PodmanHost = getEnv("FUNC_E2E_PODMAN_HOST", "", "")
	if Podman && PodmanHost == "" {
		PodmanHost = detectPodmanSocket()
		if PodmanHost != "" {
			fmt.Fprintf(os.Stderr, "  Auto-detected Podman socket: %s\n", PodmanHost)
		}
	}

	// Registry - the registry URL including any account/repository at that
	// registry.  Example:  docker.io/alice.  Default is the local registry.
	Registry = getEnv("FUNC_E2E_REGISTRY", "E2E_REGISTRY_URL", DefaultRegistry)

	// Verbose env as a truthy boolean
	Verbose = getEnvBool("FUNC_E2E_VERBOSE", "", DefaultVerbose)

	// Tools - the path to supporting tools.
	Tools = getEnvPath("FUNC_E2E_TOOLS", "", DefaultTools)

	// Testdata - the path to supporting testdata
	Testdata = getEnvPath("FUNC_E2E_TESTDATA", "", DefaultTestdata)
}

// ---------------------------------------------------------------------------
// CORE TESTS
// Create, Read, Update Delete and List.
// Implemented as "init", "run", "deploy", "describe", "list" and "delete"
// ---------------------------------------------------------------------------

// TestCore_Init ensures that initializing a default Function with only the
// minimum of required arguments or settings succeeds without error and the
// Function created is populated with the minimal settings provided.
//
//	func init
func TestCore_Init(t *testing.T) {
	name := "func-e2e-test-core-init"
	root := fromCleanEnv(t, name)

	// Act (newCmd == "func ...")
	if err := newCmd(t, "init", "-l=go").Run(); err != nil {
		t.Fatal(err)
	}

	// Assert
	f, err := fn.NewFunction(root)
	if err != nil {
		t.Fatalf("expected an initialized function, but when reading it, got error. %v", err)
	}
	if f.Runtime != "go" {
		t.Fatalf("expected initialized function with runtime 'go' got '%v'", f.Runtime)
	}
}

// TestCore_Run ensures that running a function results in it being
// becoming available and will echo requests.
//
//	func run
func TestCore_Run(t *testing.T) {
	name := "func-e2e-test-core-run"
	_ = fromCleanEnv(t, name)

	if err := newCmd(t, "init", "-l=go").Run(); err != nil {
		t.Fatal(err)
	}

	address, err := chooseOpenAddress(t)
	if err != nil {
		t.Fatal(err)
	}
	cmd := newCmd(t, "run", "--address", address)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}

	// Wait for echo
	if !waitForEcho(t, "http://"+address) {
		t.Fatalf("service does not appear to have started correctly.")
	}

	// ^C the running function
	if err := cmd.Process.Signal(os.Interrupt); err != nil {
		fmt.Fprintf(os.Stderr, "error interrupting. %v", err)
	}

	// Wait for exit and error if anything other than 130 (^C/interrupt)
	if err := cmd.Wait(); isAbnormalExit(t, err) {
		t.Fatalf("function exited abnormally %v", err)
	}
}

// TestCore_Deploy ensures that a function can be deployed to the cluster.
//
//	func deploy
func TestCore_Deploy_Basic(t *testing.T) {
	name := "func-e2e-test-core-deploy"
	_ = fromCleanEnv(t, name)

	if err := newCmd(t, "init", "-l=go").Run(); err != nil {
		t.Fatal(err)
	}

	if err := newCmd(t, "deploy").Run(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		clean(t, name, Namespace)
	}()

	if !waitForEcho(t, fmt.Sprintf("http://%v.%s.localtest.me", name, Namespace)) {
		t.Fatalf("function did not deploy correctly")
	}
}

// TestCore_Deploy_Template ensures that the system supports creating
// functions based off templates in a remote repository.
// func deploy --repository=https://github.com/alice/myfunction
func TestCore_Deploy_Template(t *testing.T) {
	name := "func-e2e-test-core-deploy-template"
	_ = fromCleanEnv(t, name)

	// Creates a new Function from the template located in the repository at a
	// well-known path:  {repo}/{runtime}/{template} where
	//   repo: github.com/functions-dev
	//   runtime: go
	//   template: http (the default.  can be changed with --template)
	if err := newCmd(t, "init", "-l=go", "--repository=https://github.com/functions-dev/func-e2e-tests").Run(); err != nil {
		t.Fatal(err)
	}
	if err := newCmd(t, "deploy").Run(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		clean(t, name, Namespace)
	}()

	// The default implementation responds with HTTP 200 and the string
	// "testcore-deploy-template" for all requests.
	if !waitForContent(t, fmt.Sprintf("http://%v.%s.localtest.me", name, Namespace), name) {
		t.Fatalf("function did not update correctly")
	}
}

// TestCore_Deploy_Source ensures that a function can be built and deployed
// locally from source code housed in a remote source repository.
// func deploy --git-url={url}
// func deploy --git-url={url} --git-ref={ref}
// func deploy --git-url={url} --git-ref={ref} --git-dir={subdir}
func TestCore_Deploy_Source(t *testing.T) {
	t.Log("Not Implemeted: running a local deploy from source code in a remote repo is not currently an implemented feature because this can be easily accomplished with `git clone ... && func deoploy`")
	// Should this be a feature implemented in the future (mostly just a
	// convenience command), the test would be as follows:
	// resetEnv(t)
	// name := "func-e2e-test-core-deploy-source"
	// _ = cdTemp(t, name) // sets Function name obliquely, see function docs
	//
	// if err := newCmd(t, "deploy", "--git-url=https://github.com/functions-dev/func-e2e-tests").Run(); err != nil {
	// 	t.Fatal(err)
	// }
	// defer func() {
	// 	clean(t, name, Namespace)
	// }()
	// if !waitForContent(t, fmt.Sprintf("http://func-e2e-test-deploy-source.%s.localtest.me", Namespace), "func-e2e-test-deploy-source") {
	// 	t.Fatalf("function did not update correctly")
	// }
}

// TestCore_Update ensures that a running function can be updated.
//
// func deploy
func TestCore_Update(t *testing.T) {
	name := "func-e2e-test-core-update"
	root := fromCleanEnv(t, name)

	// create
	if err := newCmd(t, "init", "-l=go").Run(); err != nil {
		t.Fatal(err)
	}

	// deploy
	if err := newCmd(t, "deploy").Run(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		clean(t, name, Namespace)
	}()
	if !waitForEcho(t, fmt.Sprintf("http://%v.%s.localtest.me", name, Namespace)) {
		t.Fatalf("function did not deploy correctly")
	}

	// update
	update := `
	package function
	import "fmt"
	import "net/http"
	func Handle(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprintln(w, "UPDATED")
	}
	`
	err := os.WriteFile(filepath.Join(root, "handle.go"), []byte(update), 0644)
	if err != nil {
		t.Fatal(err)
	}
	if err := newCmd(t, "deploy").Run(); err != nil {
		t.Fatal(err)
	}
	if !waitForContent(t, fmt.Sprintf("http://%v.%s.localtest.me", name, Namespace), "UPDATED") {
		t.Fatalf("function did not update correctly")
	}
}

// TestCore_Describe ensures that describing a function accurately represents
// its expected state.
//
//	func describe
func TestCore_Describe(t *testing.T) {
	name := "func-e2e-test-core-describe"
	_ = fromCleanEnv(t, name)

	if err := newCmd(t, "init", "-l=go").Run(); err != nil {
		t.Fatal(err)
	}

	cmd := newCmd(t, "deploy")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		clean(t, name, Namespace)
	}()

	if err := cmd.Wait(); err != nil {
		t.Fatalf("deploy error. %v", err)
	}

	if !waitForEcho(t, fmt.Sprintf("http://%v.%s.localtest.me", name, Namespace)) {
		t.Fatalf("function did not deploy correctly")
	}

	// Call func describe with JSON output
	cmd = newCmd(t, "describe", "--output=json")
	out := bytes.Buffer{}
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}

	// Parse the JSON output
	var instance fn.Instance
	if err := json.Unmarshal(out.Bytes(), &instance); err != nil {
		t.Fatalf("error unmarshaling describe output: %v", err)
	}

	// Validate that the name matches what we expect
	if instance.Name != name {
		t.Errorf("Expected name %q, got %q", name, instance.Name)
	}
}

// TestCore_Invoke ensures that the invoke helper functions for both
// local and remote function instances.
//
//	func invoke
func TestCore_Invoke(t *testing.T) {
	name := "func-e2e-test-core-invoke"
	_ = fromCleanEnv(t, name)

	if err := newCmd(t, "init", "-l=go").Run(); err != nil {
		t.Fatal(err)
	}

	// Test local invocation
	// ----------------------------------------
	// Runs the function locally, which `func invoke` will invoke when
	// it detects it is running.
	address, err := chooseOpenAddress(t)
	if err != nil {
		t.Fatal(err)
	}

	cmd := newCmd(t, "run", "--address", address)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	run := cmd // for the closure
	defer func() {
		// ^C the running function
		if err := run.Process.Signal(os.Interrupt); err != nil {
			fmt.Fprintf(os.Stderr, "error interrupting. %v", err)
		}
	}()

	// TODO: complete implementation of `func run --json` structured output
	// such that we can parse it for the actual listen address in the case
	// that there is already something else running on 8080
	if !waitForEcho(t, "http://"+address) {
		t.Fatalf("service does not appear to have started correctly.")
	}

	// Check invoke
	cmd = newCmd(t, "invoke", "--data=func-e2e-test-core-invoke-local")
	out := bytes.Buffer{}
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "func-e2e-test-core-invoke-local") {
		t.Logf("out: %v", out.String())
		t.Fatal("function invocation did not echo data provided")
	}

	// Test remote invocation
	// ----------------------------------------
	// Deploys the function remotely.  `func invoke` will then invoke it
	// with preference over the (still) running local instance.
	if err := newCmd(t, "deploy").Run(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		clean(t, name, Namespace)
	}()
	if !waitForEcho(t, fmt.Sprintf("http://func-e2e-test-core-invoke.%s.localtest.me", Namespace)) {
		t.Fatalf("function did not deploy correctly")
	}
	cmd = newCmd(t, "invoke", "--data=func-e2e-test-core-invoke-remote")
	out = bytes.Buffer{}
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "func-e2e-test-core-invoke-remote") {
		t.Logf("out: %v", out.String())
		t.Fatal("function invocation did not echo data provided")
	}
}

// TestCore_Delete ensures that a function registered as deleted when deleted.
// Also tests list as a side-effect.
//
//	func delete
func TestCore_Delete(t *testing.T) {
	name := "func-e2e-test-core-delete"
	_ = fromCleanEnv(t, name)

	// Deploy a Function
	if err := newCmd(t, "init", "-l=go").Run(); err != nil {
		t.Fatal(err)
	}

	if err := newCmd(t, "deploy").Run(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		clean(t, name, Namespace)
	}()
	if !waitForEcho(t, fmt.Sprintf("http://%v.%s.localtest.me", name, Namespace)) {
		t.Fatalf("function did not deploy correctly")
	}

	// Check it appears in the list
	client := fn.New(fn.WithLister(knative.NewLister(false)))
	list, err := client.List(context.Background(), Namespace)
	if err != nil {
		t.Fatal(err)
	}

	if !containsInstance(t, list, name, Namespace) {
		t.Logf("list: %v", list)
		t.Fatal("Instance list did not contain the 'delete' test service")
	}

	// Delete the Function
	if err := newCmd(t, "delete").Run(); err != nil {
		t.Logf("Error deleting function. %v", err)
	}

	list, err = client.List(context.Background(), Namespace)
	if err != nil {
		t.Fatal(err)
	}

	// Check it no longer appears in the list
	if containsInstance(t, list, name, Namespace) {
		t.Logf("list: %v", list)
		t.Fatalf("Instance %q is still shown as available", name)
	}
}

// ---------------------------------------------------------------------------
// METADATA TESTS
// Environment Variables, Labels, Volumes, and Subscriptions
// ---------------------------------------------------------------------------

// TestMetadata_Envs_Add ensures that environment variables configured to be
// passed to the Function are available at runtime.
// - Static Value
// - Local Environment Variable
// - Config Map (single key)
// - Config Map (all keys)
// - Secret (single key)
// - Secret (all keys)
//
//	func config envs add --name={name} --value={value}
func TestMetadata_Envs_Add(t *testing.T) {
	name := "func-e2e-test-metadata-envs-add"
	root := fromCleanEnv(t, name)

	// Create the test Function
	if err := newCmd(t, "init", "-l=go").Run(); err != nil {
		t.Fatal(err)
	}

	// Set Env: fixed value passed as an argument
	if err := newCmd(t, "config", "envs", "add",
		"--name=A", "--value=a").Run(); err != nil {
		t.Fatal(err)
	}

	// Set Env: from local ENV "B"
	os.Setenv("B", "b") // From a local ENV
	if err := newCmd(t, "config", "envs", "add",
		"--name=B", "--value={{env:B}}").Run(); err != nil {
		t.Fatal(err)
	}

	// Set Env: from cluster secret (single)
	setSecret(t, "test-secret-single", Namespace, map[string][]byte{
		"C": []byte("c"),
	})
	if err := newCmd(t, "config", "envs", "add",
		"--name=C", "--value={{secret:test-secret-single:C}}").Run(); err != nil {
		t.Fatal(err)
	}

	// Set Env: from all the keys in a secret (multi)
	setSecret(t, "test-secret-multi", Namespace, map[string][]byte{
		"D": []byte("d"),
		"E": []byte("e"),
	})
	if err := newCmd(t, "config", "envs", "add",
		"--value={{secret:test-secret-multi}}").Run(); err != nil {
		t.Fatal(err)
	}

	// Set Env: from cluster config map (single)
	setConfigMap(t, "test-config-map-single", Namespace, map[string]string{
		"F": "f",
	})
	if err := newCmd(t, "config", "envs", "add",
		"--name=F", "--value={{configMap:test-config-map-single:F}}").Run(); err != nil {
		t.Fatal(err)
	}

	// Set Env: from all keys in a configMap (multi)
	setConfigMap(t, "test-config-map-multi", Namespace, map[string]string{
		"G": "g",
		"H": "h",
	})
	if err := newCmd(t, "config", "envs", "add",
		"--value={{configMap:test-config-map-multi}}").Run(); err != nil {
		t.Fatal(err)
	}

	// The test function will respond HTTP 500 unless all defined environment
	// variables exist and are populated.
	impl := `
	package function
	import (
		"fmt"
		"net/http"
		"os"
	    "strings"
	)
	func Handle(w http.ResponseWriter, _ *http.Request) {
		for c := 'A'; c <= 'H'; c++ {
			envVar := string(c)
			value, exists := os.LookupEnv(envVar)
			if exists && strings.ToLower(envVar) == value {
				continue
			} else if exists {
				msg := fmt.Sprintf("Environment variable %s exists but does not have the expected value: %s\n", envVar, value)
				http.Error(w, msg, http.StatusInternalServerError)
	            return
			} else {
				msg := fmt.Sprintf("Environment variable %s does not exist\n", envVar)
				http.Error(w, msg, http.StatusInternalServerError)
	            return
			}
		}
		fmt.Fprintln(w, "OK")
	}
	`
	err := os.WriteFile(filepath.Join(root, "handle.go"), []byte(impl), 0644)
	if err != nil {
		t.Fatal(err)
	}
	if err := newCmd(t, "deploy").Run(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		clean(t, name, Namespace)
	}()
	if !waitForContent(t, fmt.Sprintf("http://%v.%s.localtest.me", name, Namespace), "OK") {
		t.Fatalf("handler failed")
	}

	// Set a test Environment Variable
	// Add
}

// TestMetadata_Envs_Remove ensures that environment variables can be removed.
//
//	func config envs remove --name={name}
func TestMetadata_Envs_Remove(t *testing.T) {
	name := "func-e2e-test-metadata-envs-remove"
	root := fromCleanEnv(t, name)

	// Create the test Function
	if err := newCmd(t, "init", "-l=go").Run(); err != nil {
		t.Fatal(err)
	}

	// Set Env: two fixed values passed as an argument
	if err := newCmd(t, "config", "envs", "add",
		"--name=A", "--value=a").Run(); err != nil {
		t.Fatal(err)
	}
	if err := newCmd(t, "config", "envs", "add",
		"--name=B", "--value=b").Run(); err != nil {
		t.Fatal(err)
	}

	// Test that the function received both A and B
	impl := `
	package function
	import (
		"fmt"
		"net/http"
		"os"
	)
	func Handle(w http.ResponseWriter, _ *http.Request) {
		if os.Getenv("A") != "a" {
			http.Error(w, "A not set", http.StatusInternalServerError)
			return
		}
		if os.Getenv("B") != "b" {
			http.Error(w, "A not set", http.StatusInternalServerError)
			return
		}
		fmt.Fprintln(w, "OK")
	}
	`
	if err := os.WriteFile(filepath.Join(root, "handle.go"), []byte(impl), 0644); err != nil {
		t.Fatal(err)
	}
	if err := newCmd(t, "deploy").Run(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		clean(t, name, Namespace)
	}()
	if !waitForContent(t, fmt.Sprintf("http://%v.%s.localtest.me", name, Namespace), "OK") {
		t.Fatalf("handler failed")
	}

	// Remove B
	if err := newCmd(t, "config", "envs", "remove", "--name=B").Run(); err != nil {
		t.Fatal(err)
	}

	// Test that the function now only receives A
	impl = `
	package function
	import (
		"fmt"
		"net/http"
		"os"
	)
	func Handle(w http.ResponseWriter, _ *http.Request) {
		if os.Getenv("A") != "a" {
			http.Error(w, "A not set", http.StatusInternalServerError)
			return
		}
		if _, exists := os.LookupEnv("B"); exists {
			http.Error(w, "B still exists after remove", http.StatusInternalServerError)
			return
		}
		fmt.Fprintln(w, "OK")
	}
	`
	if err := os.WriteFile(filepath.Join(root, "handle.go"), []byte(impl), 0644); err != nil {
		t.Fatal(err)
	}
	if err := newCmd(t, "deploy").Run(); err != nil {
		t.Fatal(err)
	}
	if !waitForContent(t, fmt.Sprintf("http://%v.%s.localtest.me", name, Namespace), "OK") {
		t.Fatalf("handler failed")
	}
}

// TestMetadata_Labels_Add ensures that labels added via the CLI are
// carried through to the final service
//
// func config labels add
func TestMetadata_Labels_Add(t *testing.T) {
	name := "func-e2e-test-metadata-labels-add"
	_ = fromCleanEnv(t, name)

	if err := newCmd(t, "init", "-l=go").Run(); err != nil {
		t.Fatal(err)
	}

	// Add a label with a simple value
	// func config labels add --name=foo --value=bar
	if err := newCmd(t, "config", "labels", "add", "--name=foo", "--value=bar").Run(); err != nil {
		t.Fatal(err)
	}

	// Add a label which pulls its value from an environment variable
	// func config labels add --name=foo --value={{env:TESTLABEL}}
	os.Setenv("TESTLABEL", "testvalue")
	if err := newCmd(t, "config", "labels", "add", "--name=envlabel", "--value={{ env:TESTLABEL }}").Run(); err != nil {
		t.Fatal(err)
	}

	// Deploy the function
	if err := newCmd(t, "deploy").Run(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		clean(t, name, Namespace)
	}()
	if !waitForEcho(t, fmt.Sprintf("http://%v.%s.localtest.me", name, Namespace)) {
		t.Fatalf("function did not deploy correctly")
	}

	// Use the output from "func describe" (json output) to verify the
	// function contains the both the test labels as expected.
	cmd := newCmd(t, "describe", name, "--output=json", "--namespace", Namespace)
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}

	var instance fn.Instance
	if err := json.Unmarshal(out.Bytes(), &instance); err != nil {
		t.Fatalf("error unmarshaling describe output: %v", err)
	}
	if instance.Labels == nil {
		t.Fatal("No labels returned")
	}
	if instance.Labels["foo"] != "bar" {
		t.Errorf("Label 'foo' not found or has wrong value. Got: %v", instance.Labels["foo"])
	}
	if instance.Labels["envlabel"] != "testvalue" {
		t.Errorf("Label 'envlabel' not found or has wrong value. Got: %v", instance.Labels["envlabel"])
	}
}

// TestMetadata_Labels_Remove ensures that labels can be removed.
//
// func config labels remove
func TestMetadata_Labels_Remove(t *testing.T) {
	name := "func-e2e-test-metadata-labels-remove"
	_ = fromCleanEnv(t, name)

	// Create the test Function with a couple simple labels
	if err := newCmd(t, "init", "-l=go").Run(); err != nil {
		t.Fatal(err)
	}
	if err := newCmd(t, "config", "labels", "add", "--name=foo", "--value=bar").Run(); err != nil {
		t.Fatal(err)
	}
	if err := newCmd(t, "config", "labels", "add", "--name=foo2", "--value=bar2").Run(); err != nil {
		t.Fatal(err)
	}
	if err := newCmd(t, "deploy").Run(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		clean(t, name, Namespace)
	}()
	if !waitForEcho(t, fmt.Sprintf("http://%v.%s.localtest.me", name, Namespace)) {
		t.Fatalf("function did not deploy correctly")
	}

	// Verify the labels were applied
	cmd := newCmd(t, "describe", name, "--output=json", "--namespace", Namespace)
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}
	var desc fn.Instance
	if err := json.Unmarshal(out.Bytes(), &desc); err != nil {
		t.Fatalf("error unmarshaling describe output: %v", err)
	}
	if desc.Labels == nil {
		t.Fatal("No labels returned")
	}
	if desc.Labels["foo"] != "bar" {
		t.Errorf("Label 'foo' not found or has wrong value. Got: %v", desc.Labels["foo"])
	}
	if desc.Labels["foo2"] != "bar2" {
		t.Errorf("Label 'foo2' not found or has wrong value. Got: %v", desc.Labels["foo2"])
	}

	// Remove one label and redeploy
	if err := newCmd(t, "config", "labels", "remove", "--name=foo2").Run(); err != nil {
		t.Fatal(err)
	}
	if err := newCmd(t, "deploy").Run(); err != nil {
		t.Fatal(err)
	}
	if !waitForEcho(t, fmt.Sprintf("http://%v.%s.localtest.me", name, Namespace)) {
		t.Fatalf("function did not redeploy correctly")
	}

	// Verify the function no longer includes the removed label.
	cmd = newCmd(t, "describe", "--output=json")
	out = bytes.Buffer{}
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}

	var desc2 fn.Instance
	if err := json.Unmarshal(out.Bytes(), &desc2); err != nil {
		t.Fatalf("error unmarshaling describe output: %v", err)
	}
	if _, ok := desc2.Labels["foo"]; !ok {
		t.Error("Label 'foo' should still exist")
	}
	if _, ok := desc2.Labels["foo2"]; ok {
		t.Error("Label 'foo' was not removed")
	}
}

// TestMetadta_Volumes ensures that adding volumes of various types are
// made available to the running function, and can be removed.
//
// func config volumes add
// func config volumes remove
func TestMetadata_Volumes(t *testing.T) {
	name := "func-e2e-test-metadata-volumes"
	root := fromCleanEnv(t, name)

	// Create the test Function
	if err := newCmd(t, "init", "-l=go").Run(); err != nil {
		t.Fatal(err)
	}

	// Cluster Test Configuration
	// --------------------------
	// Create test resources that will be mounted as volumes

	// Create a ConfigMap with test data
	configMapName := fmt.Sprintf("%s-configmap", name)
	setConfigMap(t, configMapName, Namespace, map[string]string{
		"config.txt": "configmap-data",
	})

	// Create a Secret with test data
	secretName := fmt.Sprintf("%s-secret", name)
	setSecret(t, secretName, Namespace, map[string][]byte{
		"secret.txt": []byte("secret-data"),
	})

	// Add volumes using the new CLI commands
	// Add ConfigMap volume
	if err := newCmd(t, "config", "volumes", "add",
		"--type=configmap",
		"--source="+configMapName,
		"--mount-path=/etc/config").Run(); err != nil {
		t.Fatal(err)
	}

	// Add Secret volume
	if err := newCmd(t, "config", "volumes", "add",
		"--type=secret",
		"--source="+secretName,
		"--mount-path=/etc/secret").Run(); err != nil {
		t.Fatal(err)
	}

	// Add EmptyDir volume (for testing write capabilities)
	if err := newCmd(t, "config", "volumes", "add",
		"--type=emptydir",
		"--mount-path=/tmp/scratch").Run(); err != nil {
		t.Fatal(err)
	}

	// Create a Function implementation which validates the volumes.
	impl := `package function

import (
	"fmt"
	"net/http"
	"os"
	"strings"
)

func Handle(w http.ResponseWriter, _ *http.Request) {
	errors := []string{}

	// Check ConfigMap volume
	configData, err := os.ReadFile("/etc/config/config.txt")
	if err != nil {
		errors = append(errors, fmt.Sprintf("ConfigMap read error: %v", err))
	} else if string(configData) != "configmap-data" {
		errors = append(errors, fmt.Sprintf("ConfigMap data mismatch: got %q", string(configData)))
	}

	// Check Secret volume
	secretData, err := os.ReadFile("/etc/secret/secret.txt")
	if err != nil {
		errors = append(errors, fmt.Sprintf("Secret read error: %v", err))
	} else if string(secretData) != "secret-data" {
		errors = append(errors, fmt.Sprintf("Secret data mismatch: got %q", string(secretData)))
	}

	// Check EmptyDir volume (test write capability)
	testFile := "/tmp/scratch/test.txt"
	testData := "emptydir-test"
	if err := os.WriteFile(testFile, []byte(testData), 0644); err != nil {
		errors = append(errors, fmt.Sprintf("EmptyDir write error: %v", err))
	} else {
		// Read it back to verify
		readData, err := os.ReadFile(testFile)
		if err != nil {
			errors = append(errors, fmt.Sprintf("EmptyDir read error: %v", err))
		} else if string(readData) != testData {
			errors = append(errors, fmt.Sprintf("EmptyDir data mismatch: got %q", string(readData)))
		}
	}

	if len(errors) > 0 {
		http.Error(w, strings.Join(errors, "\n"), http.StatusInternalServerError)
		return
	}
	fmt.Fprintln(w, "OK")
}

`
	err := os.WriteFile(filepath.Join(root, "handle.go"), []byte(impl), 0644)
	if err != nil {
		t.Fatal(err)
	}

	// Deploy the function
	if err := newCmd(t, "deploy").Run(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		clean(t, name, Namespace)
	}()

	// Verify the function has access to all volumes
	if !waitForContent(t, fmt.Sprintf("http://%s.%s.localtest.me", name, Namespace), "OK") {
		t.Fatalf("function failed to access volumes correctly")
	}

	// Test volume removal
	// Remove the ConfigMap volume
	if err := newCmd(t, "config", "volumes", "remove",
		"--mount-path=/etc/config").Run(); err != nil {
		t.Fatal(err)
	}

	// Update implementation to verify ConfigMap is no longer accessible
	impl = `package function

import (
	"fmt"
	"net/http"
	"os"
	"strings"
)

func Handle(w http.ResponseWriter, _ *http.Request) {
	errors := []string{}

	// Check ConfigMap volume should NOT exist
	if _, err := os.Stat("/etc/config"); !os.IsNotExist(err) {
		errors = append(errors, "ConfigMap volume still exists after removal")
	}

	// Check Secret volume should still exist
	secretData, err := os.ReadFile("/etc/secret/secret.txt")
	if err != nil {
		errors = append(errors, fmt.Sprintf("Secret read error: %v", err))
	} else if string(secretData) != "secret-data" {
		errors = append(errors, fmt.Sprintf("Secret data mismatch: got %q", string(secretData)))
	}

	// Check EmptyDir volume should still exist
	testFile := "/tmp/scratch/test2.txt"
	if err := os.WriteFile(testFile, []byte("test2"), 0644); err != nil {
		errors = append(errors, fmt.Sprintf("EmptyDir write error: %v", err))
	}

	if len(errors) > 0 {
		http.Error(w, strings.Join(errors, "\n"), http.StatusInternalServerError)
		return
	}
	fmt.Fprintln(w, "OK")
}
`
	err = os.WriteFile(filepath.Join(root, "handle.go"), []byte(impl), 0644)
	if err != nil {
		t.Fatal(err)
	}

	// Redeploy and verify removal worked
	if err := newCmd(t, "deploy").Run(); err != nil {
		t.Fatal(err)
	}

	if !waitForContent(t, fmt.Sprintf("http://%s.%s.localtest.me", name, Namespace), "OK") {
		t.Fatalf("function failed after volume removal")
	}
}

// TODO: TestMetadata_Subscriptions ensures that function instances can be
// subscribed to events.
func TestMetadata_Subscriptions(t *testing.T) {
	// TODO
	// Create a function which emits an event with as much defaults as possible
	// Create a function which subscribes to those events
	// Succeed the test as soon as it receives the event
	t.Skip("Subscritions E2E tests not yet implemented")
}

// ---------------------------------------------------------------------------
// REMOTE TESTS
// Tests related to invoking processes remotely (in-cluster).
// All remote tests presume the cluster has Tekton installed.
// ---------------------------------------------------------------------------

// TestRemote_Deploy ensures that the default action of running a remote
// build succeeds:  uploading local souce code to the cluster for build and
// delpoy in-cluster.
//
//	func deploy --remote
func TestRemote_Deploy(t *testing.T) {
	name := "func-e2e-test-remote-deploy"
	_ = fromCleanEnv(t, name)

	if err := newCmd(t, "init", "-l=go").Run(); err != nil {
		t.Fatal(err)
	}
	if err := newCmd(t, "deploy", "--remote", "--builder=pack", "--registry=registry.default.svc.cluster.local:5000/func").Run(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		clean(t, name, Namespace)
	}()

	if !waitForEcho(t, fmt.Sprintf("http://%v.%s.localtest.me", name, Namespace)) {
		t.Fatalf("function did not deploy correctly")
	}
}

// TestRemote_Source ensures a remote build can be triggered which pulls
// source from a remote repository.
//
//	func deploy --remote --git-url={url} --registry={} --builder=pack
func TestRemote_Source(t *testing.T) {
	name := "func-e2e-test-remote-source"
	_ = fromCleanEnv(t, name)

	// This command currently requires the function source also be available
	// locally in order to use its name.
	cmd := exec.Command("git", "clone", "https://github.com/functions-dev/func-e2e-tests", ".")
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}

	// Trigger the deploy
	if err := newCmd(t, "deploy", "--remote",
		"--git-url", "https://github.com/functions-dev/func-e2e-tests",
		"--registry", "registry.default.svc.cluster.local:5000/func",
		"--builder", "pack",
	).Run(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		clean(t, name, Namespace)
	}()

	if !waitForContent(t,
		fmt.Sprintf("http://%v.%s.localtest.me", name, Namespace), name) {
		t.Fatalf("function did not deploy correctly")
	}

}

// TestRemote_Ref ensures a remote build can be triggered which pulls
// sourece from a specific reference (branch/tag) of a remote repository.
func TestRemote_Ref(t *testing.T) {
	name := "func-e2e-test-remote-ref"
	_ = fromCleanEnv(t, name)

	// This command currently requires the function source also be available
	// locally in order to use its name.
	cmd := exec.Command("git", "clone", "https://github.com/functions-dev/func-e2e-tests", ".")
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}

	// IMPORTANT: The local func.yaml must match the one in the target branch.
	// This is a current limitation where remote builds still require local
	// source to determine function metadata (name, runtime, etc).
	// TODO: Remove this checkout once the implementation supports fetching
	// function metadata from the remote repository.
	cmd = exec.Command("git", "checkout", name)
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}

	// Trigger the deploy
	if err := newCmd(t, "deploy", "--remote",
		"--git-url", "https://github.com/functions-dev/func-e2e-tests",
		"--git-branch", name,
		"--registry", "registry.default.svc.cluster.local:5000/func",
		"--builder", "pack",
		"--build",
	).Run(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		clean(t, name, Namespace)
	}()

	if !waitForContent(t,
		fmt.Sprintf("http://%v.%s.localtest.me", name, Namespace), name) {
		t.Fatalf("function did not deploy correctly")
	}
}

// TestRemote_Dir ensures that remote builds can be instructed to build and
// deploy a function located in a subdirectory.
//
//	func deploy --remote --git-dir={subdir}
//	func deploy --remote --git-dir={subdir} --git-url={url}
func TestRemote_Dir(t *testing.T) {
	name := "func-e2e-test-remote-dir"
	_ = fromCleanEnv(t, name)

	// This command currently requires the function source also be available
	// locally in order to use its name.
	cmd := exec.Command("git", "clone", "https://github.com/functions-dev/func-e2e-tests", ".")
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}

	// IMPORTANT: When using --git-dir, we need to change to that directory locally
	// to ensure the local func.yaml matches the one that will be used in the remote build.
	// This is a current limitation where remote builds still require local source to
	// determine function metadata (name, runtime, etc).
	// TODO: Remove this cd once the implementation supports fetching function metadata
	// from the remote repository subdirectory.
	if err := os.Chdir(name); err != nil {
		t.Fatalf("failed to change to subdirectory %s: %v", name, err)
	}

	// Trigger the deploy
	if err := newCmd(t, "deploy", "--remote",
		"--git-url", "https://github.com/functions-dev/func-e2e-tests",
		"--git-dir", name,
		"--registry", "registry.default.svc.cluster.local:5000/func",
		"--builder", "pack",
		"--build",
	).Run(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		clean(t, name, Namespace)
	}()

	if !waitForContent(t,
		fmt.Sprintf("http://%v.%s.localtest.me", name, Namespace), name) {
		t.Fatalf("function did not deploy correctly")
	}
}

// TestPodman_Pack ensures that the Podman container engine can be used to
// deploy functions built with Pack.
func TestPodman_Pack(t *testing.T) {
	name := "func-e2e-test-podman-pack"
	_ = fromCleanEnv(t, name)
	if err := setupPodman(t); err != nil {
		t.Fatal(err)
	}

	if !Podman {
		t.Skip("Podman tests not enabled. Enable with FUNC_E2E_PODMAN=true and set FUNC_E2E_PODMAN_HOST to the Podman socket")
	}
	if PodmanHost == "" {
		t.Skip("FUNC_E2E_PODMAN_HOST must be set to the Podman socket path")
	}

	// Create a Function
	if err := newCmd(t, "init", "-l=go").Run(); err != nil {
		t.Fatal(err)
	}

	// Deploy
	// ------
	if err := newCmd(t, "deploy", "--builder=pack").Run(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		clean(t, name, Namespace)
	}()

	if !waitForEcho(t, fmt.Sprintf("http://%v.%s.localtest.me", name, Namespace)) {
		t.Fatalf("function did not deploy correctly")
	}
}

// TestPodman_S2I ensures that the Podman container engine can be used to
// deploy functions built with S2I.
func TestPodman_S2I(t *testing.T) {
	name := "func-e2e-test-podman-s2i"
	_ = fromCleanEnv(t, name)
	if err := setupPodman(t); err != nil {
		t.Fatal(err)
	}

	if !Podman {
		t.Skip("Podman tests not enabled. Enable with FUNC_E2E_TEST_PODMAN=true and set FUNC_E2E_PODMAN_HOST to the Podman socket")
	}
	if PodmanHost == "" {
		t.Skip("FUNC_E2E_PODMAN_HOST must be set to the Podman socket path")
	}

	// Create a Function
	if err := newCmd(t, "init", "-l=go").Run(); err != nil {
		t.Fatal(err)
	}

	// Deploy
	// ------
	if err := newCmd(t, "deploy", "--builder=s2i").Run(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		clean(t, name, Namespace)
	}()

	if !waitForEcho(t, fmt.Sprintf("http://%v.%s.localtest.me", name, Namespace)) {
		t.Fatalf("function did not deploy correctly")
	}

}

// ---------------------------------------------------------------------------
// MATRIX TESTS
// Tests related to confirming functionality of different language runtimes
// and builders.
//
// For each of:
//
//		OS:       Linux, Mac, Windows (handled at the Git Action level)
//		Runtime:  Go, Python, Node, Typescript, Quarkus, Springboot, Rust
//		Builder:  Host, Pack, S2I
//		Template: http, CloudEvent
//
//	 Test it can:
//	 1.  Run locally on the host (func run)
//	 3.  Deploy and receive the default response (an echo)
//	 4.  Remote build and run via an in-cluster build
// -----------------

// TestMatrix_Run ensures that supported runtimes and builders can run both
// builtin templates locally.
func TestMatrix_Run(t *testing.T) {
	if !Matrix {
		t.Skip("Matrix tests not enabled. Enable with FUNC_E2E_MATRIX=true")
	}
	for _, runtime := range MatrixRuntimes {
		for _, builder := range MatrixBuilders {
			for _, template := range MatrixTemplates {
				name := fmt.Sprintf("func-e2e-matrix-%s-%s-%s-run", runtime, builder, template)
				// Test Running Locally
				// --------------------
				t.Run(name, func(t *testing.T) {
					doMatrixRun(t, name, runtime, builder, template)
				})
			}
		}
	}
}

// doMatrixRun implements a specific permutation of the local run matrix test.
func doMatrixRun(t *testing.T, name, runtime, builder, template string) {
	t.Helper()
	_ = fromCleanEnv(t, name)

	// Clean up container images and volumes when done
	t.Cleanup(func() {
		cleanImages(t, name)
	})

	// Choose an address ahead of time
	address, err := chooseOpenAddress(t)
	if err != nil {
		t.Fatal(err)
	}

	// func init
	init := []string{"init", "-l", runtime, "-t", template}

	// func run
	run := []string{"run", "--builder", builder, "--address", address}

	// Language and architecture special treatment
	// - Skips tests if the builder is not supported
	// - Skips tests for the pack builder if on ARM
	// - adds arguments as necessary
	init, timeout := matrixExceptionsLocal(t, init, runtime, builder, template)

	// Initialize
	// ----------
	if err := newCmd(t, init...).Run(); err != nil {
		t.Fatalf("Failed to create %s function with %s template: %v", runtime, template, err)
	}

	// Run
	// ---
	cmd := newCmd(t, run...)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}

	// Wait for the function to be ready, using the appropriate method based
	// on template
	httpAddress := "http://" + address
	var ready bool
	if template == "cloudevents" {
		ready = waitForCloudevent(t, httpAddress, withWaitTimeout(timeout))
	} else { // default is http:
		ready = waitForEcho(t, httpAddress, withWaitTimeout(timeout))
	}

	if !ready {
		t.Fatalf("service does not appear to have started correctly.")
	}

	// ^C the running function
	if err := cmd.Process.Signal(os.Interrupt); err != nil {
		fmt.Fprintf(os.Stderr, "error interrupting. %v", err)
	}

	// Wait for exit and error if anything other than 130 (^C/interrupt)
	if err := cmd.Wait(); isAbnormalExit(t, err) {
		t.Fatalf("function exited abnormally %v", err)
	}
}

// TestMatrix_Deploy ensures that supported runtimes and builders can deploy
// builtin templates successfully.
func TestMatrix_Deploy(t *testing.T) {
	if !Matrix {
		t.Skip("Matrix tests not enabled. Enable with FUNC_E2E_MATRIX=true")
	}
	for _, runtime := range MatrixRuntimes {
		for _, builder := range MatrixBuilders {
			for _, template := range MatrixTemplates {
				name := fmt.Sprintf("func-e2e-matrix-%s-%s-%s-deploy", runtime, builder, template)
				t.Run(name, func(t *testing.T) {
					doMatrixDeploy(t, name, runtime, builder, template)
				})
			}
		}
	}
}

// doMatrixDeploy implements a specific permutation of the deploy matrix tests.
func doMatrixDeploy(t *testing.T, name, runtime, builder, template string) {
	t.Helper()
	_ = fromCleanEnv(t, name)

	// Register cleanup functions (runs in LIFO order - image cleanup will run after cluster cleanup)
	t.Cleanup(func() {
		cleanImages(t, name)
	})
	t.Cleanup(func() {
		clean(t, name, Namespace)
	})

	// Initialize
	initArgs := []string{"init", "-l", runtime, "-t", template}
	initArgs, timeout := matrixExceptionsLocal(t, initArgs, runtime, builder, template)
	if err := newCmd(t, initArgs...).Run(); err != nil {
		t.Fatalf("Failed to create %s function with %s template: %v", runtime, template, err)
	}

	// Deploy
	deployArgs := []string{"deploy", "--builder", builder}
	if err := newCmd(t, deployArgs...).Run(); err != nil {
		t.Fatal(err)
	}

	// Wait for the function to be ready, using the appropriate method based
	// on template
	httpAddress := fmt.Sprintf("http://%v.%s.localtest.me", name, Namespace)
	var ready bool
	if template == "cloudevents" {
		ready = waitForCloudevent(t, httpAddress, withWaitTimeout(timeout))
	} else {
		ready = waitForEcho(t, httpAddress, withWaitTimeout(timeout))
	}

	if !ready {
		t.Fatalf("function did not deploy correctly")
	}
}

// TestMatrix_Remote ensures that supported runtimes and builders can deploy
// builtin templates remotely.
func TestMatrix_Remote(t *testing.T) {
	if !Matrix {
		t.Skip("Matrix tests not enabled. Enable with FUNC_E2E_MATRIX=true")
	}
	for _, runtime := range MatrixRuntimes {
		for _, builder := range MatrixBuilders {
			for _, template := range MatrixTemplates {
				name := fmt.Sprintf("func-e2e-matrix-%s-%s-%s-remote", runtime, builder, template)
				t.Run(name, func(t *testing.T) {
					doMatrixRemote(t, name, runtime, builder, template)
				})
			}
		}
	}
}

// doMatrixRemote implements a specific permutation of the remote deploy matrix tests.
func doMatrixRemote(t *testing.T, name, runtime, builder, template string) {
	t.Helper()
	_ = fromCleanEnv(t, name)

	// Register cleanup functions (runs in LIFO order - image cleanup will run after cluster cleanup)
	t.Cleanup(func() {
		cleanImages(t, name)
	})
	t.Cleanup(func() {
		clean(t, name, Namespace)
	})

	// Initialize
	initArgs := []string{"init", "-l", runtime, "-t", template}
	initArgs, timeout := matrixExceptionsRemote(t, initArgs, runtime, builder, template)
	if err := newCmd(t, initArgs...).Run(); err != nil {
		t.Fatalf("Failed to create %s function with %s template: %v", runtime, template, err)
	}

	// Deploy
	if err := newCmd(t, "deploy", "--builder", builder, "--remote", "--registry=registry.default.svc.cluster.local:5000/func").Run(); err != nil {
		t.Fatal(err)
	}

	// Wait for the function to be ready, using the appropriate method based on template
	functionURL := fmt.Sprintf("http://%v.%s.localtest.me", name, Namespace)
	var ready bool
	if template == "cloudevents" {
		ready = waitForCloudevent(t, functionURL, withWaitTimeout(timeout))
	} else {
		ready = waitForEcho(t, functionURL, withWaitTimeout(timeout))
	}

	if !ready {
		t.Fatalf("function did not deploy correctly")
	}
}

// matrixExceptionsLocal adds language-specific arguments or skips matrix tests as
// necessary to match current levels of supported combinations for local
// tasks such as run, build and deploy
func matrixExceptionsLocal(t *testing.T, initArgs []string, funcRuntime, builder, template string) ([]string, time.Duration) {
	t.Helper()

	// Choose a default timeout based on builder.
	// Slightly shorter for local builds
	var waitTimeout = 2 * time.Minute
	if builder == "pack" || builder == "s2i" {
		waitTimeout = 6 * time.Minute
	}

	return matrixExceptionsShared(t, initArgs, funcRuntime, builder, template, waitTimeout)
}

// matrixExceptionsRemote adds language-specific arguments or skips matrix tests as
// necessary to match current levels of supported combinations for remote
// builds
func matrixExceptionsRemote(t *testing.T, initArgs []string, funcRuntime, builder, template string) ([]string, time.Duration) {
	t.Helper()

	// Choose a default timeout based on builder.
	// Slightly longer for remote builds
	var waitTimeout = 2 * time.Minute
	if builder == "pack" || builder == "s2i" {
		waitTimeout = 5 * time.Minute
	}

	// Remote builds only support Pack and S2I
	if builder == "host" {
		t.Skip("Host builder is not supported for remote builds.")
	}

	return matrixExceptionsShared(t, initArgs, funcRuntime, builder, template, waitTimeout)
}

// matrixExceptionsShared are exceptions to the full matrix which are shared
// between both local (run, build, deploy) and remote (deploy --remote)
func matrixExceptionsShared(t *testing.T, initArgs []string, funcRuntime, builder, template string, waitTimeout time.Duration) ([]string, time.Duration) {
	t.Helper()

	// Buildpacks do not currently support ARM
	if builder == "pack" && (runtime.GOARCH == "arm64" || runtime.GOARCH == "arm") {
		t.Skip("Paketo buildpacks do not currently support ARM64 architecture. " +
			"See https://github.com/paketo-buildpacks/nodejs/issues/712")
	}

	// Python Special Treatment
	// --------------------------
	// Skip Pack builder (not supported)
	// TODO: Remove when pack support is added
	if funcRuntime == "python" && builder == "pack" {
		t.Skip("The pack builder does not currently support Python Functions")
	}

	// Echo Implementation
	// Replace the simple "OK" implementation with an echo.
	//
	// The Python HTTP template is currently not an "echo" because it's
	// annoyingly complex, and we want the default template to be as simple
	// and approachable as possible.  We'll be transitioning to having all
	// builtin templates to a simple "OK" response for this reason, and using
	// an external repository for the "echo" implementations currently the
	// default.  Python HTTP is a bit ahead of this schedule, so use an echo
	// implementation in ./testdata until then:
	if funcRuntime == "python" && template == "http" {
		initArgs = append(initArgs, "--repository", "file://"+filepath.Join(Testdata, "templates"))
	}

	// Node special treatment
	// ----------------------
	// Skip on Host builder (not supported)
	if funcRuntime == "node" && builder == "host" {
		t.Skip("The host builder does not currently support Node Functions")
	}

	// Typescript special treatment
	// ----------------------
	// Skip on Host builder (not supported)
	if funcRuntime == "typescript" && builder == "host" {
		t.Skip("The host builder does not currently support Typescript Functions")
	}

	// Rust special treatment
	// ----------------------
	// Skip on Host builder (not supported)
	if funcRuntime == "rust" && builder == "host" {
		t.Skip("The host builder does not currently support Rust Functions")
	}
	// Skip on S2I builder (not supported)
	if funcRuntime == "rust" && builder == "s2i" {
		t.Skip("The s2i builder does not currently support Rust Functions")
	}

	// Quarkus special treatment
	// ----------------------
	// Skip on Host builder (not supported)
	if funcRuntime == "quarkus" && builder == "host" {
		t.Skip("The host builder does not currently support Quarkus Functions")
	}
	// Java can take... a while
	if funcRuntime == "quarkus" {
		waitTimeout = 6 * time.Minute
	}

	// Springboot special treatment
	// ----------------------
	// Skip on Host builder (not supported)
	if funcRuntime == "springboot" && builder == "host" {
		t.Skip("The host builder does not currently support Springboot Functions")
	}
	// Skip on s2i builder (not supported)
	if funcRuntime == "springboot" && builder == "s2i" {
		t.Skip("The s2i builder does not currently support Springboot Functions")
	}
	// Java can take... a while
	if funcRuntime == "springboot" {
		waitTimeout = 10 * time.Minute
	}
	return initArgs, waitTimeout
}

// ----------------------------------------------------------------------------
// Helpers
// ----------------------------------------------------------------------------

// fromCleanEnv provides a clean environment for a function E2E test.
func fromCleanEnv(t *testing.T, name string) (root string) {
	root = cdTemp(t, name)
	// Deprecated?  We're allowing HOME to stay set for now:
	// setupHome(t)
	setupEnv(t)
	return
}

// cdTmp changes to a new temporary directory for running the test.
// Essentially equivalent to creating a new directory before beginning to
// use func.  The path created is returned.
// The "name" argument is the name of the final Function's directory.
// Note that this will be unnecessary when upcoming changes remove the logic
// which uses the current directory name by default for function name and
// instead requires an explicit name argument on build/deploy.
// Name should be unique per test to enable better test isolation.
func cdTemp(t *testing.T, name string) string {
	pwd, err := os.Getwd()
	if err != nil {
		panic(err)
	}
	tmp := filepath.Join(t.TempDir(), name)
	if err := os.MkdirAll(tmp, 0755); err != nil {
		panic(err)
	}
	if err := os.Chdir(tmp); err != nil {
		panic(err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(pwd); err != nil {
			panic(err)
		}
	})
	return tmp
}

// setupEnv before running a test to remove all environment variables and
// set the required environment variables to those specified during
// initialization.
//
// Every test must be run with a nearly completely isolated environment,
// otherwise a developer's local environment will affect the E2E tests when
// run locally outside of CI. Some environment variables, provided via
// FUNC_E2E_* or other settings, are explicitly set here.
func setupEnv(t *testing.T) {
	t.Helper()
	// Preserve HOME, PATH and some XDG paths, and PATH
	home := os.Getenv("HOME")
	path := Tools + ":" + os.Getenv("PATH") // Prepend E2E tools
	xdgConfigHome := os.Getenv("XDG_CONFIG_HOME")
	xdgRuntimeDir := os.Getenv("XDG_RUNTIME_DIR")
	// Preserve SSH environment variables for git operations (needed when
	// git configs rewrite HTTPS URLs to SSH URLs)
	sshAuthSock := os.Getenv("SSH_AUTH_SOCK")
	sshAgentPid := os.Getenv("SSH_AGENT_PID")

	// Clear everything else
	os.Clearenv()

	os.Setenv("HOME", home)
	os.Setenv("PATH", path)
	os.Setenv("XDG_CONFIG_HOME", xdgConfigHome)
	os.Setenv("XDG_RUNTIME_DIR", xdgRuntimeDir)
	if sshAuthSock != "" {
		os.Setenv("SSH_AUTH_SOCK", sshAuthSock)
	}
	if sshAgentPid != "" {
		os.Setenv("SSH_AGENT_PID", sshAgentPid)
	}
	os.Setenv("KUBECONFIG", Kubeconfig)
	os.Setenv("GOCOVERDIR", Gocoverdir)
	os.Setenv("FUNC_VERBOSE", fmt.Sprintf("%t", Verbose))

	// The Registry will be set either during first-time setup using the
	// global config, or already defaulted by the user via environment variable.
	os.Setenv("FUNC_REGISTRY", Registry)

	// If the docker host is set, it should affect any tests which perform
	// container operations except for podman-specific tests.  These use
	// the FUNC_E2E_PODMAN_HOST value during test execution directly.
	os.Setenv("DOCKER_HOST", DockerHost)

	// The following host-builder related settings will become the defaults
	// once the host builder supports the core runtimes.  Setting them here in
	// order to futureproof individual tests.
	os.Setenv("FUNC_BUILDER", "host")    // default to host builder
	os.Setenv("FUNC_CONTAINER", "false") // "run" uses host builder
}

// setupPodmanEnvs
// - configures VM to treat localhost:50000 as an insecure registry
// - proxy connections to the host if running in a VM (like on darwin)
// - creates an XDG_CONFIG_HOME and XDG_DATA_HOME
func setupPodman(t *testing.T) error {
	t.Helper()

	// Podman Socket
	os.Setenv("DOCKER_HOST", PodmanHost)

	// Podman Config
	// NOTE: the unqualified-search-registries and short-name-mode may be
	// unnecessary.
	cfg := `unqualified-search-registries = ["docker.io", "quay.io", "registry.fedoraproject.org", "registry.access.redhat.com"]
short-name-mode="permissive"

[[registry]]
location="localhost:50000"
insecure=true
`
	cfgPath := filepath.Join(t.TempDir(), "registries.conf")
	if err := os.WriteFile(cfgPath, []byte(cfg), 0644); err != nil {
		return fmt.Errorf("failed to create registries.conf: %v", err)
	}
	os.Setenv("CONTAINERS_REGISTRIES_CONF", cfgPath)

	// Podman Info
	// May be useful when debugging:
	// t.Log("podman info:")
	// infoCmd := exec.Command("podman", "info")
	// output, err := infoCmd.CombinedOutput()
	// if err != nil {
	// 	return err
	// }
	// t.Logf("%s", output)

	// Done if Linux
	if runtime.GOOS == "linux" {
		// Podman machine setup is only needed on macOS/Windows
		// On Linux, Podman runs natively without a VM
		t.Log("Running on Linux - Podman machine setup not needed")
		return nil
	}

	// Windows and Darwin must run Podman in a VM.
	// connect the pipes

	// List available machines (debug)
	t.Log("Available Podman Machines:")
	listCmd := exec.Command("podman", "machine", "list")
	output, err := listCmd.CombinedOutput()
	if err != nil {
		return err
	}
	t.Logf("%s", output)

	// Check if a Podman machine is running
	// The output contains "Currently running" or similar text when a machine is active
	if !strings.Contains(string(output), "Currently running") && !strings.Contains(string(output), "Running") {
		return fmt.Errorf("Podman machine is not running. Please start it with: podman machine start podman-machine-default")
	}

	// Kill any existing process on port 50000 in the Podman VM
	killCmd := exec.Command("podman", "machine", "ssh", "--",
		"sudo lsof -ti :50000 | sudo xargs kill -9 2>/dev/null || true")
	if output, err = killCmd.CombinedOutput(); err != nil {
		t.Logf("output: %s", output)
		return fmt.Errorf("failed killing existing registry proxy: %v", err)
	}

	// Set up socat proxy to forward localhost:50000 to host.containers.internal:50000
	// This allows containers in Podman to access the host's registry
	proxyCmd := exec.Command("podman", "machine", "ssh", "--",
		"sudo sh -c 'socat TCP-LISTEN:50000,fork,reuseaddr TCP:host.containers.internal:50000 </dev/null >/dev/null 2>&1 & echo Registry proxy started'")
	if output, err = proxyCmd.CombinedOutput(); err != nil {
		t.Logf("output: %s", output)
		return fmt.Errorf("failed to set up registry proxy: %v, output: %s", err, output)
	}
	t.Logf("Podman registry proxy enabled: %s", strings.TrimSpace(string(output)))

	return nil
}

// newCmd returns an *exec.Cmd
// bin will be FUNC_E2E_BIN, and if FUNC_E2E_PLUGIN is set, the subcommand
// will be set as well.
// arguments set to those provided.
func newCmd(t *testing.T, args ...string) *exec.Cmd {
	t.Helper()
	bin := Bin

	// If Plugin proivided, it is a subcommand so prepend it to args.
	if Plugin != "" {
		args = append([]string{Plugin}, args...)
	}

	// Add verbose flag if Verbose is set
	if Verbose {
		args = append(args, "-v")
	}

	// Debug

	b := strings.Builder{}
	for _, arg := range args {
		b.WriteString(arg + " ")
	}
	base := filepath.Base(bin)
	t.Logf("$ %v %v\n", base, b.String())

	cmd := exec.Command(bin, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd

	// TODO: create an option to only print stdout and stderr if the
	// test fails?
	//
	// var stdout bytes.Buffer
	// cmd := exec.Command(bin, args...)
	// cmd.Stdout = io.MultiWriter(os.Stdout, &stdout)
	// cmd.Stderr = os.Stderr
	// if err := cmd.Run(); err != nil {
	// 	t.Fatal(err)
	// }
	// return stdout.String()
}

type waitOption func(*waitCfg)

type waitCfg struct {
	timeout       time.Duration // total time to wait
	interval      time.Duration // time between retries
	warnThreshold time.Duration // start warning after this long
	warnInterval  time.Duration // only warn this often
}

func withWaitTimeout(t time.Duration) waitOption {
	return func(cfg *waitCfg) {
		cfg.timeout = t
	}
}

// waitForEcho returns true if there is service at the given addresss which
// echoes back the request arguments given.
func waitForEcho(t *testing.T, address string, options ...waitOption) (ok bool) {
	return waitFor(t, address+"?test-echo-param&message=test-echo-param", "test-echo-param", "does not appear to be an echo", options...)
}

// waitForCloudevent returns true if there is a service at the given address
// which accepts CloudEvents and responds with HTTP 200 when given a cloudevent
func waitForCloudevent(t *testing.T, address string, options ...waitOption) (ok bool) {
	t.Helper()

	cfg := waitCfg{
		timeout:       2 * time.Minute,
		interval:      5 * time.Second,
		warnThreshold: 30 * time.Second,
		warnInterval:  10 * time.Second,
	}
	for _, o := range options {
		o(&cfg)
	}

	client := &http.Client{}

	startTime := time.Now()
	lastWarnTime := time.Time{}
	attemptCount := 0

	for time.Since(startTime) < cfg.timeout {
		attemptCount++
		time.Sleep(cfg.interval)

		req, err := http.NewRequest("POST", address, strings.NewReader(`{"message": "test"}`))
		if err != nil {
			t.Fatalf("error creating request: %v", err)
			return false
		}

		// Set CloudEvents headers
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Ce-Id", "test-event-1")
		req.Header.Set("Ce-Type", "test.event.type")
		req.Header.Set("Ce-Source", "e2e-test")
		req.Header.Set("Ce-Specversion", "1.0")

		res, err := client.Do(req)
		if err != nil {
			elapsed := time.Since(startTime)
			if elapsed > cfg.warnThreshold && time.Since(lastWarnTime) >= cfg.warnInterval {
				t.Logf("unable to contact function (attempt %v, elapsed %v). %v", attemptCount, elapsed.Round(time.Second), err)
				lastWarnTime = time.Now()
			}
			continue
		}
		defer res.Body.Close()

		if res.StatusCode == 200 {
			// Validate that the response is a proper CloudEvent
			// CloudEvents can be in either binary or structured mode

			// Read the response body first
			body, err := io.ReadAll(res.Body)
			if err != nil {
				t.Logf("Error reading response body: %v", err)
				continue
			}

			// Ensure body is valid JSON
			var jsonBody any
			if err := json.Unmarshal(body, &jsonBody); err != nil {
				elapsed := time.Since(startTime)
				if elapsed > cfg.warnThreshold && time.Since(lastWarnTime) >= cfg.warnInterval {
					t.Logf("Function returned 200 but invalid JSON body (attempt %v, elapsed %v): %v", attemptCount, elapsed.Round(time.Second), err)
					lastWarnTime = time.Now()
				}
				continue
			}

			// Check for CloudEvent response (either mode)
			contentType := res.Header.Get("Content-Type")
			ceSpecVersion := res.Header.Get("Ce-Specversion")

			if contentType == "application/cloudevents+json" {
				// Valid structured CloudEvent
				t.Logf("Received valid structured CloudEvent response")
				return true
			} else if ceSpecVersion != "" {
				// Valid binary CloudEvent
				t.Logf("Received valid binary CloudEvent response")
				return true
			}

			// Neither structured nor binary CloudEvent
			elapsed := time.Since(startTime)
			if elapsed > cfg.warnThreshold && time.Since(lastWarnTime) >= cfg.warnInterval {
				t.Logf("Function returned 200 but response is not a CloudEvent (attempt %v, elapsed %v)", attemptCount, elapsed.Round(time.Second))
				t.Logf("Content-Type: %s, Ce-Specversion: %s", contentType, ceSpecVersion)
				lastWarnTime = time.Now()
			}
			continue
		}

		if res.StatusCode == 500 {
			body, _ := io.ReadAll(res.Body)
			t.Log("500 response received; canceling retries.")
			t.Logf("Response: %s\n", body)
			return false
		}

		elapsed := time.Since(startTime)
		if elapsed > cfg.warnThreshold && time.Since(lastWarnTime) >= cfg.warnInterval {
			t.Logf("Function responded with status %d (attempt %v, elapsed %v)", res.StatusCode, attemptCount, elapsed.Round(time.Second))
			lastWarnTime = time.Now()
		}
	}

	t.Logf("Could not validate CloudEvents function after %v (timeout %v)", time.Since(startTime).Round(time.Second), cfg.timeout)
	return false
}

// waitForContent returns true if there is a service listening at the
// given addresss which responds HTTP OK with the given string in its body.
// returns false if the.
func waitForContent(t *testing.T, address, content string, options ...waitOption) (ok bool) {
	return waitFor(t, address, content, "expected content not found", options...)
}

// waitFor an endpoint to return an OK response which includes the given
// content.
//
// If the Function returns a 500, it is considered a positive test failure
// by the implementation and retries are discontinued.
//
// TODO:  Implement a --output=json flag on `func run` and update all
// callers currently passing localhost:8080 with this calculated value.
//
// Reasoning: This will be a false negative if port 8080 is being used
// by another process.  This will fail because, if a service is already running
// on port 8080, Functions will automatically choose to run the next higher
// unused port.  And this will be a false positive if there happens to be
// a service not already running on the port which happens to implement an
// echo.  For example there is another process outside the E2Es which is
// currently executing a `func run`
// Note that until this is implemented, this temporary implementation also
// forces single-threaded test execution.
func waitFor(t *testing.T, address, content, errMsg string, options ...waitOption) (ok bool) {
	t.Helper()
	cfg := waitCfg{
		timeout:       2 * time.Minute,
		interval:      5 * time.Second,
		warnThreshold: 30 * time.Second,
		warnInterval:  10 * time.Second,
	}
	for _, o := range options {
		o(&cfg)
	}

	var (
		mismatchLast     = ""    // cache the last content for squelching purposes.
		mismatchReported = false // note that the given content was reported
	)

	startTime := time.Now()
	lastWarnTime := time.Time{}
	attemptCount := 0

	for time.Since(startTime) < cfg.timeout {
		attemptCount++
		time.Sleep(cfg.interval)
		// t.Logf("GET %v\n", address)
		res, err := http.Get(address)
		if err != nil {
			elapsed := time.Since(startTime)
			if elapsed > cfg.warnThreshold && time.Since(lastWarnTime) >= cfg.warnInterval {
				t.Logf("unable to contact function (attempt %v, elapsed %v). %v", attemptCount, elapsed.Round(time.Second), err)
				lastWarnTime = time.Now()
			}
			continue
		}
		body, err := io.ReadAll(res.Body)
		if err != nil {
			t.Logf("error reading function response. %v", err)
			continue
		}
		defer res.Body.Close()
		if res.StatusCode == 500 {
			t.Log("500 response received; canceling retries.")
			t.Logf("Response: %s\n", body)
			return false
		}
		if !strings.Contains(string(body), content) {
			if string(body) != mismatchLast || !mismatchReported {
				if errMsg == "" {
					errMsg = "expected content not found"
				}
				t.Log("Response received, but " + errMsg)
				t.Logf("Response: %s\n", body)
				mismatchLast = string(body)
				mismatchReported = true
			}
			continue
		}
		return true
	}
	t.Logf("Could not validate function after %v (timeout %v)", time.Since(startTime).Round(time.Second), cfg.timeout)
	return
}

// isAbnormalExit checks an error returned from a cmd.Wait and returns true
// if the error is something other than a known exit 130 from a SIGINT.
func isAbnormalExit(t *testing.T, err error) bool {
	if err == nil {
		return false // no error is not an abnormal error.
	}
	t.Helper()
	if exitErr, ok := err.(*exec.ExitError); ok {
		exitCode := exitErr.ExitCode()
		// When interrupted, the exit will exit with an ExitError, but
		// should be exit code 130 (the code for SIGINT)
		if exitCode != 0 && exitCode != 130 {
			t.Fatalf("Function exited code %v", exitErr.ExitCode())
			return true
		}
	} else {
		t.Fatalf("Function errored during execution. %v", err)
		return true
	}
	return false
}

// setSecret creates or replaces a secret.
func setSecret(t *testing.T, name, ns string, data map[string][]byte) {
	ctx := context.Background()
	config, err := k8s.GetClientConfig().ClientConfig()
	if err != nil {
		t.Fatal(err)
	}
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		t.Fatal(err)
	}
	_ = clientset.CoreV1().Secrets(ns).Delete(ctx, name, metav1.DeleteOptions{})
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Data:       data,
	}
	_, err = clientset.CoreV1().Secrets(ns).Create(ctx, secret, metav1.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}
}

// setConfigMap creates or replaces a configMap
func setConfigMap(t *testing.T, name, ns string, data map[string]string) {
	ctx := context.Background()
	config, err := k8s.GetClientConfig().ClientConfig()
	if err != nil {
		t.Fatal(err)
	}
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		t.Fatal(err)
	}
	_ = clientset.CoreV1().ConfigMaps(ns).Delete(ctx, name, metav1.DeleteOptions{})
	configMap := corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Data:       data,
	}
	_, err = clientset.CoreV1().ConfigMaps(ns).Create(ctx, &configMap, metav1.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}
}

// containsInstance checks if the list includes the given instance.
func containsInstance(t *testing.T, list []fn.ListItem, name, namespace string) bool {
	t.Helper()
	for _, v := range list {
		if v.Name == name && v.Namespace == namespace {
			return true
		}
	}
	return false
}

// clean up by deleting the named function (via defers)
func clean(t *testing.T, name, ns string) {
	t.Helper()
	if !Clean {
		return
	}
	// There is currently a bug in delete which hangs for several seconds
	// when deleting a Function. This adds considerably to the test suite
	// execution time.  Tests are written such that they are not dependent
	// on a clean exit/cleanup, so this step is skipped for speed.
	if err := newCmd(t, "delete", name, "--namespace", ns).Run(); err != nil {
		t.Logf("Error deleting function. %v", err)
	}
}

// cleanImages removes container images and volumes created for a function during testing.
// This is separate from clean() which handles cluster resources.
func cleanImages(t *testing.T, name string) {
	t.Helper()

	if !CleanImages {
		return
	}

	// Log stats before cleanup
	logImageStats(t, name, "pre-cleanup")

	// Build the image name pattern used by the tests
	imageName := fmt.Sprintf("%s/%s", Registry, name)

	// Track what we cleaned
	var imagesRemoved, volumesRemoved int

	// Try to remove with Docker first
	dockerCmd := exec.Command("docker", "rmi", imageName)
	if output, err := dockerCmd.CombinedOutput(); err != nil {
		// Log but don't fail - image might not exist or Docker might not be available
		if strings.Contains(string(output), "No such image") {
			t.Logf("Docker image %s not found (already cleaned or never created)", imageName)
		} else {
			t.Logf("Docker image cleanup for %s failed: %v", imageName, err)
		}
	} else {
		imagesRemoved++
	}

	// Clean up any pack build cache volumes associated with this function
	// Format: pack-cache-func_{function-name}_latest-*
	volumePattern := fmt.Sprintf("pack-cache-func_%s_", name)

	// List and remove matching Docker volumes
	listCmd := exec.Command("docker", "volume", "ls", "--format", "{{.Name}}")
	if output, err := listCmd.Output(); err == nil {
		volumes := strings.Split(string(output), "\n")
		for _, vol := range volumes {
			vol = strings.TrimSpace(vol)
			if strings.HasPrefix(vol, volumePattern) {
				rmCmd := exec.Command("docker", "volume", "rm", vol)
				if err := rmCmd.Run(); err != nil {
					t.Logf("Failed to remove Docker volume %s: %v", vol, err)
				} else {
					volumesRemoved++
				}
			}
		}
	}

	// Log cleanup summary
	if imagesRemoved > 0 || volumesRemoved > 0 {
		t.Logf("Cleanup complete for %s: removed %d image(s) and %d volume(s)", name, imagesRemoved, volumesRemoved)
	} else {
		t.Logf("No images or volumes to clean for %s", name)
	}

	// Log stats after cleanup (but only if we actually cleaned something)
	if imagesRemoved > 0 {
		logImageStats(t, name, "post-cleanup")
	}
}

// logImageStats logs Docker/Podman storage statistics for debugging disk usage
func logImageStats(t *testing.T, name string, phase string) {
	t.Helper()

	// Get image size for this specific function
	imageName := fmt.Sprintf("%s/%s", Registry, name)

	// Check Docker image size
	dockerSizeCmd := exec.Command("docker", "images", "--format", "{{.Size}}", imageName)
	if output, err := dockerSizeCmd.Output(); err == nil && len(output) > 0 {
		t.Logf("[%s] Docker image %s size: %s", phase, imageName, strings.TrimSpace(string(output)))
	}

	// Log overall Docker storage usage (only once at the beginning to avoid noise)
	if phase == "pre-cleanup" {
		dockerDfCmd := exec.Command("docker", "system", "df", "--format", "{{.Type}}\t{{.Size}}\t{{.Reclaimable}}")
		if output, err := dockerDfCmd.Output(); err == nil {
			t.Logf("[%s] Docker storage usage:\n%s", phase, string(output))
		}
	}
}

// detectPodmanSocket attempts to auto-detect the Podman socket path.
// This replicates the logic from hack/set-podman-host.sh.
// Returns the socket path with unix:// prefix, or empty string if not found.
func detectPodmanSocket() string {
	// Check if podman command is available
	if _, err := exec.LookPath("podman"); err != nil {
		fmt.Fprintf(os.Stderr, "  Warning: podman command not found\n")
		return ""
	}

	// Check if Podman service is running
	if err := exec.Command("podman", "info").Run(); err != nil {
		fmt.Fprintf(os.Stderr, "  Warning: Podman service is not running: %v\n", err)
		return ""
	}

	var socketPath string

	// OS-specific socket detection
	if runtime.GOOS == "darwin" || runtime.GOOS == "windows" {
		// On macOS/Windows, try to get the socket from podman machine inspect
		machineName := os.Getenv("PODMAN_MACHINE")
		if machineName == "" {
			machineName = "podman-machine-default"
		}

		// Try podman machine inspect
		inspectCmd := exec.Command("podman", "machine", "inspect", machineName, "--format", "{{.ConnectionInfo.PodmanSocket.Path}}")
		if output, err := inspectCmd.Output(); err == nil {
			socketPath = strings.TrimSpace(string(output))
		}

		// fallback: use podman info
		if socketPath == "" {
			infoCmd := exec.Command("podman", "info", "-f", "{{.Host.RemoteSocket.Path}}")
			if output, err := infoCmd.Output(); err == nil {
				socketPath = strings.TrimSpace(string(output))
			}
		}
	} else {
		// Linux: directly use podman info
		infoCmd := exec.Command("podman", "info", "-f", "{{.Host.RemoteSocket.Path}}")
		output, err := infoCmd.Output()
		if err != nil {
			fmt.Fprintf(os.Stderr, "  Warning: podman info failed: %v\n", err)
			return ""
		}
		socketPath = strings.TrimSpace(string(output))
	}

	if socketPath == "" {
		fmt.Fprintf(os.Stderr, "  Warning: Could not determine Podman socket path\n")
		return ""
	}

	// Check if it already has unix:// prefix
	if strings.HasPrefix(socketPath, "unix://") {
		return socketPath
	}

	// Add unix:// prefix if needed
	return "unix://" + socketPath
}

// getEnvPath converts the value returned from getEnv to an absolute path.
// See getEnv docs for details.
func getEnvPath(env, deprecated, dflt string) (val string) {
	val = getEnv(env, deprecated, dflt)
	if !filepath.IsAbs(val) { // convert to abs
		var err error
		if val, err = filepath.Abs(val); err != nil {
			panic(fmt.Sprintf("error converting path to absolute. %v", err))
		}
	}
	return
}

// getEnvPath converts the value returned from getEnv into a string slice.
func getEnvList(env, deprecated, dflt string) (vals []string) {
	return fromCSV(getEnv(env, deprecated, dflt))
}

// getEnvBool converts the value returned from getEnv into a boolean.
func getEnvBool(env, deprecated string, dfltBool bool) bool {
	dflt := fmt.Sprintf("%t", dfltBool)
	val, err := strconv.ParseBool(getEnv(env, deprecated, dflt))
	if err != nil {
		panic(fmt.Sprintf("value for %v %v expected to be boolean. %v", env, deprecated, err))
	}
	return val
}

// getEnv gets the value of the given environment variable, or the default.
// If the optional deprecated environment variable name is passed, it will be used
// as a fallback with a warning about its deprecation status being printed.
// The final value will be converted to an absolute path.
func getEnv(env, deprecated, dflt string) (val string) {
	// First check deprecated if provided
	if deprecated != "" {
		if val = os.Getenv(deprecated); val != "" {
			fmt.Fprintf(os.Stderr, "warning:  the env var %v is deprecated and support will be removed in a future release.   please use %v.", deprecated, env)
		}
	}
	// Current env takes precedence
	if v := os.Getenv(env); v != "" {
		val = v
	}
	// Default
	if val == "" {
		val = dflt
	}
	return
}

// printVersion of func which is being used, taking into account if
// we're running as a plugin.
func printVersion() {
	args := []string{"version", "--verbose"}
	bin := Bin
	if Plugin != "" {
		args = append([]string{Plugin}, args...)
	}
	os.Setenv("GOCOVERDIR", Gocoverdir)
	cmd := exec.Command(bin, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		os.Exit(1)
	}
}

func fromCSV(s string) (result []string) {
	result = []string{}
	ss := strings.Split(s, ",")
	for _, s := range ss {
		result = append(result, strings.TrimSpace(s))
	}
	return
}

func toCSV(ss []string) string {
	return strings.Join(ss, ",")
}

// chooseOpenAddress for use with things like running local functions.
// Always uses the looback interface; OS-chosen port.
func chooseOpenAddress(t *testing.T) (address string, err error) {
	t.Helper()
	var l net.Listener
	if l, err = net.Listen("tcp", "127.0.0.1:"); err != nil {
		return "", fmt.Errorf("cannot bind tcp: %w", err)
	}
	defer l.Close()
	return l.Addr().String(), nil
}
