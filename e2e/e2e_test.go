/*
Package e2e provides an end-to-end test suite for the Functions CLI "func".

tl;dr:

	./hack/registry.sh         Configure system for insecure local registrires
	./hack/install-binaries.sh Fetch binaries into ./hack/bin
	./hack/allocate.sh         Create a cluster and kube config in ./hack/bin
	go test ./e2e -tags=e2e    Run all tests using these bins and cluster
	./hack/delete.sh           Nuke the cluster

Test Suite Status:

This package is a work-in-progress, and is not being executed in
CI.  For the active e2e tests, see the "test" directory.

Purpose:

This set of e2e tests are meant to allow for easy local-execution of e2e tests
in as lightweight of an implementation as possible for developers to quickly
isolate problems found in the comprehensive CI-run e2e workflows, which
are currently prohibitively complex to set up and run locally.

Overview:

The tests themselves are separated into four categories:  Core, Metadata,
Repository, and Runtimes.

Core tests include checking the basic CRUDL operations of Create, Read, Update,
Delete and List.  Creation involves initialization with "func init",
running the function locally with "func run", and in the cluster with "func
deploy".  Reading is implemented as "func describe".  Updating ensures that
an updated function replaces the old on a subsequent "func deploy".  Finally,
"func list" implements a standard list operation, listing deployed Functions.

Metadata tests ensure that manipulation of a Function's metadata is correctly
carried to the final Function.  Metadata includes environment variables,
labels, volumes and event subscriptions.

Repository tests confirm features which involve working with git repositories.
This includes operations such as building locally from source code located in
a remote repository, building a specific revision, etc.

Runtime tests are a larger matrix of tests which check operations which differ
in implementation between language runtimes.  The primary operations which
differ and must be checked for each runtime are creation and running locally.
Therefore, the runtime tests execute for each language, for each template, for
each builder.  As a side-effect of the test implementation, "func invoke" is
also tested.

Prerequisites:

These tests expect a compiled binary, which will be executed utilizing a
cluster configured to run Functions, as well as an available and authenticated
container registry.  These can be configured locally for testing by using
scripts in `../hack`:

  - install-binaries.sh: Installs executables needed for cluster setup and
    configuration into hack/bin.

  - allocate.sh: Allocates and configures a Function-ready cluster locally, and
    starts a local container registry on port :50000.

  - regsitry.sh: Configures the local podman or docker to allow unencrypted
    communication with the local registry.

  - delete.sh: Removes the cluster and registry.  Using this to recreate the
    cluster between test runs will ensure the cluster is in a clean initial state.

Options:

These tests accept environment variables which alter the default behavior:

FUNC_E2E_BIN: sets the path to the binary to use for the E2E tests.  This is
by default the binary created when "make" is run in the repository root.
Note that if providing a relative path, this path is relative to this test
package, not the directory from which `go test` was run.

FUNC_E2E_PLUGIN: if set, the command run by the tests will be
"${FUNC_E2E_BIN} func", allowing for running all tests when func is installed
as a plugin; such as when used as a plugin for the Knative cluster admin
tool "kn".  The value should be set to the name of the subcommand for the
func plugin (usually "func").  For example to run E2E tests on 'kn' with
the 'kn-func' plugged in use `FUNC_E2E_BIN=/path/to/kn FUNC_E2E_PLUGIN=func`

FUNC_E2E_REGISTRY: if provided, tests will use this registry (in form
"registry.example.com/user") instead of the test suite default of
"localhost:50000/func".

FUNC_E2E_RUNTIMES: Overrides the language runtimes to test when executing the
runtime-specific tests.  Set to empty to effectively skip the (lengthy) runtimes
tests.  By default tests all core supported runtimes.

FUNC_E2E_BUILDERS: Overrides which builders will be used during the builder and
runtime matrix.  By default is set to all builders ("host", "pack" and "s2i").

FUNC_E2E_KUBECONFIG: The path to the kubeconfig to be used by tests.  This
defaults to ../hack/bin/kubeconfig.yaml, which is created when using the
../hack/allocate.sh script to set up a test cluster.

FUNC_E2E_GOCOVERDIR: The path to use for Go coverage data reported by these
tests.  This defaults to ../.coverage

FUNC_E2E_GO: the path to the 'go' binary tests should use when running
outside of a container (host builder, or runner with --container=false).  This
can be used to test against specific go versions.  Defaults to the go binary
found in the current session's PATH.

FUNC_E2E_GIT: the path to the 'git' binary tests should provide to the commands
being tested for git-related activities.   Defaults to the git binary
found in the current session's PATH.

FUNC_E2E_VERBOSE: instructs the test suite to run all commands in
verbose mode.

Running:

From the root of the repository, run "make test-e2e-quick".  This will compile
the current source, creating the binary "./func" if it does not already exist.
It will then run "go test ./e2e".  By default the tests will use the locally
compiled "func" binary unless FUNC_E2E_BIN is provided.

Tests follow a naming convention to allow for manually testing subsets.  For
example, To run only "core" tests, run "make" to update the binary to test,
then "go test -run TestCore ./e2e".

Cleanup:

The tests do attempt to clean up after themselves, but since a test failure is
definitionally the entering of an unknown state, it is suggested to delete
the cluster between full test runs. To remove the local cluster, use the
"delete.sh" script described above.

Upgrades:
  - Now supports testing func when a plugin of a different name
  - Now supports running specific runtimes rather than the prior version which
    supported one or all.
  - Uses sensible defaults for environment variables to reduce setup when
    running locally.
  - Removes redundant `go test` flags
  - Now supports specifying builders
  - Subsets of test can be specified using name prefixes --run=TestCore etc.
*/
package e2e

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httputil"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	fn "knative.dev/func/pkg/functions"
)

// Static Defaults

const (
	// DefaultBin is the default binary to run, relative to this test file.
	// This is the binary built by default when running 'make'.
	// This can be customized with FUNC_E2E_BIN.
	// NOte this is always relative to this test file.
	DefaultBin = "../func"

	// DefaultGocoverdir defines the default path to use for the GOCOVERDIR
	// while executing tests.  This value can be altered using
	// FUNC_E2E_GOCOVERDIR. While this value could be passed through using
	// its original environment variable name "GOCOVERDIR", to keep with the
	// isolation of environment provided for all other values, this one is
	// likewise also isolated using the "FUNC_E2E_" prefix.
	DefaultGocoverdir = "../.coverage"

	// DefaultHome to use for all commands which are not explicitly setting
	// a home of a given state.  This will be removed as there is work being
	// undertaken at this time to remove the dependency on a home directory
	// in the Docker credentials system.  When complete, most commands will
	// not require HOME.
	DefaultHome = "./testdata/default_home"

	// DefaultKubeconfig is the default path (relative to this test file) at
	// which the kubeconfig can be found which was created when setting up
	// a local test cluster using the allocate.sh script.  This can be
	// overridden using FUNC_E2E_KUBECONFIG.
	DefaultKubeconfig = "../hack/bin/kubeconfig.yaml"

	// DefaultRegistry to use when running the e2e tests.  This is the URL
	// of the registry created by default when using the allocate.sh script
	// to set up a local testing cluster, but can be customized with
	// FUNC_E2E_REGISTRY.
	DefaultRegistry = "localhost:50000/func"

	// DefaultVerbose sets the default for the --verbose flag of all commands.
	DefaultVerbose = false
)

var ( // static-ish

	// DefaultBuilders which we want THESE e2e tests to consider.
	// This is currently equivalent to all known builders; host, s2i and pack.
	// Note this only affects tests which are explicitly intended to check
	// runtimes and builder compatibility.  Core tests all use the Go+host builder
	// combination.
	DefaultBuilders = []string{"host", "pack", "s2i"}

	// DefaultRuntimes which we want THESE e2e tests to consider
	// This is currently a subset but will be expanded to be all core runtimes
	// as they become supported by the Go builder.
	DefaultRuntimes = []string{"go", "python"}
)

// Final Settings
// Populated during init phase (see init func in Helpers below)
var (

	// Bin is the absolute path to the binary to use when testing.
	// Can be set with FUNC_E2E_BIN.
	Bin string

	// Plugin indicates func is being run as a plugin within Bin, and
	// the value of this argument is the subcommand.  For example, when
	// running e2e tests as a plugin to `kn`, Bin will be /path/to/kn and
	// 'Plugin' would be 'func'.  The resultant commands would then be
	//  /path/to/kn func {command}
	// Can be set with FUNC_E2E_PLUGIN
	Plugin string

	// Registry is the container registry to use by default for tests;
	// defaulting to the local container registry set up by the allocation
	// scripts running on localhost:5000.
	// Can be set with FUNC_E2E_REGISTRY
	Registry string

	// Runtimes for which runtime-specific tests should be run.  Defaults
	// to all core language runtimes.  Can be set with FUNC_E2E_RUNTIMES
	Runtimes = []string{}

	// Builders to check in addition to the "host" builder which is used
	// by default.  Used for Builder-specific tests.  Can be set with
	// FUNC_E2E_BUILDERS.
	Builders = []string{}

	// Kubeconfig is the path at which a kubeconfig suitable for running
	// E2E tests can be found.  By default the config located in
	// hack/bin/kubeconfig.yaml will be used.  This is created when running
	// hack/allocate.sh to set up a local test cluster.
	// To avoid confusion, the current user's KUBECONFIG will not be used.
	// Instead, this can be set explicitly using FUNC_E2E_KUBECONFIG.
	Kubeconfig string

	// Gocoverdir is the path to the directory which will be used for Go's
	// coverage reporting, provided to the test binary as GOCOVERDIR.  By
	// default the current user's environment is not used, and by default this
	// is set to ../.coverage (as relative to this test file).  This value
	// can be overridden with FUNC_E2E_GOCOVERDIR.
	Gocoverdir string

	// Go is the path to the go binary to instruct commands to use when
	// completing tasks which require the go toolchain.  Will be set by
	// default to the Go found in PATH, but can be overridden with
	// FUNC_E2E_GO.
	Go string

	// Git is the path to the git binary to be provided to commands to use
	// which utilize git features.  For example when building containers,
	// the current git version is provided to the running function as an
	// environment variable.  This will default to the git found in PATH, but
	// can be overridden with FUNC_E2E_GIT.
	Git string

	// Home is the final path to the default Home directory used for tests
	// which do not set it explicitly.
	Home string

	// Verbose mode for all command runs.
	Verbose bool
)

// ---------------------------------------------------------------------------
// CORE TESTS
// Create, Read, Update Delete and List.
// Implemented as "create", "run", "deploy", "describe", "list" and "delete"
// ---------------------------------------------------------------------------

// TestCore_init ensures that initializing a default Function with only the
// minimum of required arguments or settings succeeds without error and the
// Function created is populated with the minimal settings provided.
func TestCore_init(t *testing.T) {
	// Assemble
	resetEnv()
	root := cdTemp(t, "create")

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

// TestCore_run ensures that running a function results in it being
// becoming available and will echo requests.
func TestCore_run(t *testing.T) {
	resetEnv()
	_ = cdTemp(t, "run") // sets Function name obliquely, see docs

	if err := newCmd(t, "init", "-l=go").Run(); err != nil {
		t.Fatal(err)
	}

	cmd := newCmd(t, "run")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}

	if !waitFor(t, "http://localhost:8080") {
		t.Fatalf("service does not appear to have started correctly.")
	}

	// ^C the running function
	if err := cmd.Process.Signal(os.Interrupt); err != nil {
		fmt.Fprintf(os.Stderr, "error interrupting. %v", err)
	}

	// Wait for exit and error if anything other than 130 (^C/interrupt)
	if err := cmd.Wait(); isAbnormalExit(t, err) {
		t.Fatalf("funciton exited abnormally %v", err)
	}
}

// TestCore_deploy ensures that a function can be deployed to the cluster.
func TestCore_deploy(t *testing.T) {
	resetEnv()
	_ = cdTemp(t, "deploy") // sets Function name obliquely, see function docs

	if err := newCmd(t, "init", "-l=go").Run(); err != nil {
		t.Fatal(err)
	}

	cmd := newCmd(t, "deploy")

	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := newCmd(t, "delete").Run(); err != nil {
			t.Logf("Error deleting function. %v", err)
		}
	}()

	if err := cmd.Wait(); err != nil {
		t.Fatalf("deploy error. %v", err)
	}

	if !waitFor(t, "http://deploy.default.127.0.0.1.sslip.io") {
		t.Fatalf("function did not deploy correctly")
	}
}

// TestCore_update ensures that a running funciton can be updated.
func TestCore_update(t *testing.T) {
	resetEnv()
	root := cdTemp(t, "update") // sets Function name obliquely, see function docs

	// create
	if err := newCmd(t, "init", "-l=go").Run(); err != nil {
		t.Fatal(err)
	}

	// deploy
	if err := newCmd(t, "deploy").Run(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := newCmd(t, "delete").Run(); err != nil {
			t.Logf("Error deleting function. %v", err)
		}
	}()
	if !waitFor(t, "http://update.default.127.0.0.1.sslip.io") {
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

	// TODO: change to wait for echo of something in particular that
	// ensures the above update took.
	if !waitForContent(t, "http://update.default.127.0.0.1.sslip.io", "UPDATED") {
		t.Fatalf("function did not update correctly")
	}
}

// ----------------------------------------------------------------------------
// Initialization
// ----------------------------------------------------------------------------
// Deprecated           Available Settings     Final
// ---------------------------------------------------
// E2E_FUNC_BIN      => FUNC_E2E_BIN        => Bin
// E2E_USE_KN_FUNC   => FUNC_E2E_PLUGIN     => Plugin
// E2E_REGISTRY_URL  => FUNC_E2E_REGISTRY   => Registry
// E2E_RUNTIMES      => FUNC_E2E_RUNTIMES   => Runtimes
//                      FUNC_E2E_BUILDERS   => Builders
//                      FUNC_E2E_KUBECONFIG => Kubeconfig
//                      FUNC_E2E_GOCOVERDIR => Gocoverdir
//                      FUNC_E2E_GO         => Go
//                      FUNC_E2E_GIT        => Git

// init global settings for the current run from environment
// we read E2E config settings passed via the FUNC_E2E_* environment
// variables.  These globals are used when creating test cases.
// Some tests pass these values as flags, sometimes as environment variables,
// sometimes not at all; hence why the actual environment setup is deferred
// into each test, merely reading them in here during E2E process init.
func init() {
	fmt.Fprintln(os.Stderr, "Initializing E2E Tests")

	fmt.Fprintln(os.Stderr, "----------------------")
	fmt.Fprintln(os.Stderr, "Config Provided:")
	fmt.Fprintf(os.Stderr, "  FUNC_E2E_BIN=%v\n", os.Getenv("FUNC_E2E_BIN"))
	fmt.Fprintf(os.Stderr, "  FUNC_E2E_BUILDERS=%v\n", os.Getenv("FUNC_E2E_BUILDERS"))
	fmt.Fprintf(os.Stderr, "  FUNC_E2E_GOCOVERDIR=%v\n", os.Getenv("FUNC_E2E_GOCOVERDIR"))
	fmt.Fprintf(os.Stderr, "  FUNC_E2E_KUBECONFIG=%v\n", os.Getenv("FUNC_E2E_KUBECONFIG"))
	fmt.Fprintf(os.Stderr, "  FUNC_E2E_PLUGIN=%v\n", os.Getenv("FUNC_E2E_PLUGIN"))
	fmt.Fprintf(os.Stderr, "  FUNC_E2E_REGISTRY=%v\n", os.Getenv("FUNC_E2E_REGISTRY"))
	fmt.Fprintf(os.Stderr, "  FUNC_E2E_RUNTIMES=%v\n", os.Getenv("FUNC_E2E_RUNTIMES"))
	fmt.Fprintf(os.Stderr, "  FUNC_E2E_GO=%v\n", os.Getenv("FUNC_E2E_GO"))
	fmt.Fprintf(os.Stderr, "  FUNC_E2E_GIT=%v\n", os.Getenv("FUNC_E2E_GIT"))
	fmt.Fprintf(os.Stderr, "  FUNC_E2E_VERBOSE=%v\n", os.Getenv("FUNC_E2E_VERBOSE"))
	fmt.Fprintf(os.Stderr, "  (deprecated) E2E_FUNC_BIN=%v\n", os.Getenv("E2E_FUNC_BIN"))
	fmt.Fprintf(os.Stderr, "  (deprecated) E2E_REGISTRY_URL=%v\n", os.Getenv("E2E_REGISTRY_URL"))
	fmt.Fprintf(os.Stderr, "  (deprecated) E2E_RUNTIMES=%v\n", os.Getenv("E2E_RUNTIMES"))
	fmt.Fprintf(os.Stderr, "  (deprecated) E2E_USE_KN_FUNC=%v\n", os.Getenv("E2E_USE_KN_FUNC"))

	fmt.Fprintln(os.Stderr, "---------------------")
	readEnvs()

	fmt.Fprintln(os.Stderr, "Final Config:")
	fmt.Fprintf(os.Stderr, "  Bin=%v\n", Bin)
	fmt.Fprintf(os.Stderr, "  Plugin=%v\n", Plugin)
	fmt.Fprintf(os.Stderr, "  Registry=%v\n", Registry)
	fmt.Fprintf(os.Stderr, "  Runtimes=%v\n", toCSV(Runtimes))
	fmt.Fprintf(os.Stderr, "  Builders=%v\n", toCSV(Builders))
	fmt.Fprintf(os.Stderr, "  Kubeconfig=%v\n", Kubeconfig)
	fmt.Fprintf(os.Stderr, "  Go=%v\n", Go)
	fmt.Fprintf(os.Stderr, "  Git=%v\n", Git)
	fmt.Fprintf(os.Stderr, "  Verbose=%v\n", Verbose)

	// Coverage
	// --------
	// Create Gocoverdir if it does not already exist
	// FIXME

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
	// Args:  current ENV, deprecated ENV, default.
	Bin = getEnvPath("FUNC_E2E_BIN", "E2E_FUNC_BIN", DefaultBin)

	// Plugin - if set, func is a plugin and Bin is the one plugging. The value
	// is the name of the subcommand.  If set to "true", for backwards compat
	// the default value is "func"
	Plugin = getEnv("FUNC_E2E_PLUGIN", "E2E_USE_KN_FUNC", "")
	if Plugin == "true" { // backwards compatibility
		Plugin = "func" // deprecated value was literal string "true"
	}

	// Registry - the registry URL including any account/repository at that
	// registry.  Example:  docker.io/alice.  Default is the local registry.
	Registry = getEnv("FUNC_E2E_REGISTRY", "E2E_REGISTRY_URL", DefaultRegistry)

	// Runtimes - can optionally pass a list of runtimes to test, overriding
	// the default of testing all builtin runtimes.
	// Example "FUNC_E2E_RUNTIMES=go,python"
	Runtimes = getEnvList("FUNC_E2E_RUNTIMES", "E2E_RUNTIMES", "")

	// Builders - can optionally pass a list of builders to test, overriding
	// the default of testing all. Example "FUNC_E2E_BUILDERS=pack,s2i"
	Builders = getEnvList("FUNC_E2E_BUILDERS", "", "")

	// Kubeconfig - the kubeconfig to pass ass KUBECONFIG env to test
	// environments.
	Kubeconfig = getEnvPath("FUNC_E2E_KUBECONFIG", "", DefaultKubeconfig)

	// Gocoverdir - the coverage directory to use while testing the go binary.
	Gocoverdir = getEnvPath("FUNC_E2E_GOCOVERDIR", "", DefaultGocoverdir)

	// Go binary path
	Go = getEnvBin("FUNC_E2E_GO", "", "go")

	// Git binary path
	Git = getEnvBin("FUNC_E2E_GIT", "", "git")

	// Verbose env as a truthy boolean
	Verbose = getEnvBool("FUNC_E2E_VERBOSE", "", DefaultVerbose)

	// Home is a bit of a special case.  It is the default home directory, is
	// not configurable (tests override it on a case-by-case basis) and is
	// merely set here to the absolute path of DefaultHome
	var err error
	if Home, err = filepath.Abs(DefaultHome); err != nil {
		panic(fmt.Sprintf("error converting the relative default home value to absolute. %v", err))
	}
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

// getEnvBin converts the value returned from getEnv into an absolute path.
// and if not provided checks the current PATH for a matching binary name,
// and returns the absolute path to that.
func getEnvBin(env, deprecated, dflt string) string {
	val, err := exec.LookPath(getEnv(env, deprecated, dflt))
	if err != nil {
		fmt.Fprintf(os.Stderr, "error locating command %q. %v", val, err)
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
	// Current env takes precidence
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

// ----------------------------------------------------------------------------
// Test Helpers
// ----------------------------------------------------------------------------

// resetEnv before running a test to remove all environment variables and
// set the required environment variables to those specified during
// initialization.
//
// Every test must be run with a nearly completely isolated environment,
// otherwise a developer's local environment will affect the E2E tests when
// run locally outside of CI. Some environment variables, provided via
// FUNC_E2E_* or other settings, are explicitly set here.
func resetEnv() {
	os.Clearenv()
	os.Setenv("HOME", Home)
	os.Setenv("KUBECONFIG", Kubeconfig)
	os.Setenv("FUNC_GO", Go)
	os.Setenv("FUNC_GIT", Git)
	os.Setenv("GOCOVERDIR", Gocoverdir)
	os.Setenv("FUNC_VERBOSE", fmt.Sprintf("%t", Verbose))

	// The Registry will be set either during first-time setup using the
	// global config, or already defaulted by the user via environment variable.
	os.Setenv("FUNC_REGISTRY", Registry)

	// The following host-builder related settings will become the defaults
	// once the host builder supports the core runtimes.  Setting them here in
	// order to futureproof individual tests.
	os.Setenv("FUNC_ENABLE_HOST_BUILDER", "true") // Enable the host builder
	os.Setenv("FUNC_BUILDER", "host")             // default to host builder
	os.Setenv("FUNC_CONTAINER", "false")          // "run" uses host builder
}

// cdTmp changes to a new temporary directory for running the test.
// Essentially equivalent to creating a new directory before beginning to
// use func.  The path created is returned.
// The "name" argument is the name of the final Function's directory.
// Note that this will be unnecessary when upcoming changes remove the logic
// which uses the current directory name by default for funciton name and
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
		os.Chdir(pwd)
	})
	return tmp
}

// newCmd returns an *exec.Cmd
// bin will be FUNC_E2E_BIN, and if FUNC_E2E_PLUGIN is set, the subcommand
// will be set as well.
// arguments set to those provided.
func newCmd(t *testing.T, args ...string) *exec.Cmd {
	bin := Bin

	// If Plugin proivided, it is a subcommand so prepend it to args.
	if Plugin != "" {
		args = append([]string{Plugin}, args...)
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

// waitFor returns true if there is service at the given addresss which
// echoes back the request arguments given.
//
// TODO:  Implement a --output=json flag on `func run` and update all
// callers currently passing localhost:8080 with this calculated value.
//
// Reasoning: This will be a false negative if port 8080 is being used
// by another proces.  This will fail because, if a service is already running
// on port 8080, Functions will automatically choose to run the next higher
// unused port.  And this will be a false positive if there happens to be
// a service not already running on the port which happens to implement an
// echo.  For example there is another process outside the E2Es which is
// currently executing a `func run`
// Note that until this is implemented, this temporary implementation also
// forces single-threaded test execution.
func waitFor(t *testing.T, address string) (ok bool) {
	t.Helper()
	retries := 50       // Set fairly high for slow environments such as free-tier CI
	warnThreshold := 30 // start warning after 30
	warnModulo := 5     // but only warn every 5 attemtps
	delay := 500 * time.Millisecond
	for i := 0; i < retries; i++ {
		time.Sleep(delay)
		res, err := http.Get(address + "?test-echo-param")
		if err != nil {
			if i > warnThreshold && i%warnModulo == 0 {
				t.Logf("unable to contact function (attempt %v/%v). %v", i, retries, err)
			}
			continue
		}
		body, err := io.ReadAll(res.Body)
		if err != nil {
			t.Logf("error reading function response. %v", err)
			continue
		}
		defer res.Body.Close()
		if strings.Index(string(body), "test-echo-param") == -1 {
			t.Log("Response received, but it does not appear to be an echo.")
			t.Log("Full dump:")
			dump, _ := httputil.DumpResponse(res, true)
			t.Log(string(dump))
			continue
		}
		return true
	}
	t.Logf("Could not contact function after %v tries", retries)
	return
}

// waitForContent returns true if there is a service listening at the
// given addresss which responds HTTP OK with the given string in its body.
func waitForContent(t *testing.T, address, content string) (ok bool) {
	t.Helper()
	retries := 50       // Set fairly high for slow environments such as free-tier CI
	warnThreshold := 30 // start warning after 30
	warnModulo := 5     // but only warn every 5 attemtps
	delay := 500 * time.Millisecond
	for i := 0; i < retries; i++ {
		time.Sleep(delay)
		res, err := http.Get(address)
		if err != nil {
			if i > warnThreshold && i%warnModulo == 0 {
				t.Logf("unable to contact function (attempt %v/%v). %v", i, retries, err)
			}
			continue
		}
		body, err := io.ReadAll(res.Body)
		if err != nil {
			t.Logf("error reading function response. %v", err)
			continue
		}
		defer res.Body.Close()
		if !strings.Contains(string(body), content) {
			t.Log("Response received, but it did not contain the expected content.")
			t.Log("Full dump:")
			dump, _ := httputil.DumpResponse(res, true)
			t.Log(string(dump))
			continue
		}
		return true
	}
	t.Logf("Could not validate function returns expected content after %v tries", retries)
	return

}

// isAbnormalExit checks an erro returned from a cmd.Wait and returns true
// Removed
// if the error is something other than a known exit 130 from a SIGINT.
func isAbnormalExit(t *testing.T, err error) bool {
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
