/*
Package e2e provides an end-to-end test suite for the Functions CLI "func".

Status:

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
- Now supports running specific runtimes rathern than the prior version which supported one or all.
- Uses sensible defaults for environment variables to reduce setup when running locally.
- Removes redundant `go test` flags
- Now supports specifying builders
- Subsets of test can be specified using name prefixes --run=TestCore etc.
*/
package e2e

import (
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

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

	// DefaultName for Functions created when no special name is necessary
	// to set up test conditions.
	DefaultName = "testfunc"

	// DefaultRegistry to use when running the e2e tests.  This is the URL
	// of the registry created by default when using the allocate.sh script
	// to set up a local testing cluster, but can be customized with
	// FUNC_E2E_REGISTRY.
	DefaultRegistry = "localhost:50000/func"
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

	// Home is the final path to the default Home directory used for tests
	// which do not set it explicitly.
	Home string
)

// ---------------------------------------------------------------------------
// CORE TESTS
// Create, Read, Update Delete and List.
// Implemented as "init", "run", "deploy", "describe", "list" and "delete"
// ---------------------------------------------------------------------------

// TestCore_init ensures that initializing a default Function with only the
// minimum of required arguments or settings succeeds without error and the
// Function created is populated with the minimal settings provided.
func TestCore_init(t *testing.T) {
	// Assemble
	resetEnv()
	root := cdTemp(t)
	for _, env := range os.Environ() {
		t.Log(env)
	}

	// Act
	if err := newCmd(t, "init", "-l=go").Run(); err != nil {
		t.Fatal(err)
	}

	// Assert
	f, err := fn.NewFunction(root)
	if err != nil {
		t.Fatal(err)
	}
	if f.Runtime != "go" {
		t.Fatalf("expected runtime 'go' got '%v'", f.Runtime)
	}
}

// TestCore_run ensures that running a function results in it being
// becoming available and will echo requests.
func TestCore_run(t *testing.T) {
	resetEnv()
	_ = cdTemp(t)

	if err := newCmd(t, "init", "-l=go").Run(); err != nil {
		t.Fatal(err)
	}
	cmd := newCmd(t, "run", "--container=false")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}

	// TODO: parse output and find final port in the case of a port collision
	// the system will use successively higher ports until it finds one
	// unoccupied.

	res, err := http.Get("localhost:8080")
	if err != nil {
		t.Fatal(err)
	}

	cmd.Wait()

	// note that --container=false will become the default once the scaffolding
	// and host builder is supported by most/all core languages.

}

// Removed
// Tests removed from E2Es that need to have an equivalent unit or integration
// test implemented:
// - Interactive Terminal (prompt) tests

// TODO:
// Add "run" both containerised and not for all languages as it is
// implemented.

// ----------------------------------------------------------------------------
// Helpers
// ----------------------------------------------------------------------------

// init global settings for the current run from environment
// we readd E2E config settings passed via the FUNC_E2E_* environment
// variables.  These globals are used when creating test cases.
// Some tests pass these values as flags, sometimes as environment variables,
// sometimes not at all; hence why the actual environment setup is deferred
// into each test, merely reading them in here during E2E process init.
func init() {
	fmt.Fprintln(os.Stderr, "Initializing E2E Tests")
	//  Deprecated           Available Settings     Final
	//  ---------------------------------------------------
	//  E2E_FUNC_BIN      => FUNC_E2E_BIN        => Bin
	//  E2E_USE_KN_FUNC   => FUNC_E2E_PLUGIN     => Plugin
	//  E2E_REGISTRY_URL  => FUNC_E2E_REGISTRY   => Registry
	//  E2E_RUNTIMES      => FUNC_E2E_RUNTIMES   => Runtimes
	//                       FUNC_E2E_BUILDERS   => Builders
	//                       FUNC_E2E_KUBECONFIG => Kubeconfig
	//                       FUNC_E2E_GOCOVERDIR => Gocoverdir
	fmt.Fprintln(os.Stderr, "----------------------")
	fmt.Fprintln(os.Stderr, "Config Provided:")
	fmt.Fprintf(os.Stderr, "  FUNC_E2E_BIN=%v\n", os.Getenv("FUNC_E2E_BIN"))
	fmt.Fprintf(os.Stderr, "  FUNC_E2E_BUILDERS=%v\n", os.Getenv("FUNC_E2E_BUILDERS"))
	fmt.Fprintf(os.Stderr, "  FUNC_E2E_GOCOVERDIR=%v\n", os.Getenv("FUNC_E2E_GOCOVERDIR"))
	fmt.Fprintf(os.Stderr, "  FUNC_E2E_KUBECONFIG=%v\n", os.Getenv("FUNC_E2E_KUBECONFIG"))
	fmt.Fprintf(os.Stderr, "  FUNC_E2E_PLUGIN=%v\n", os.Getenv("FUNC_E2E_PLUGIN"))
	fmt.Fprintf(os.Stderr, "  FUNC_E2E_REGISTRY=%v\n", os.Getenv("FUNC_E2E_REGISTRY"))
	fmt.Fprintf(os.Stderr, "  FUNC_E2E_RUNTIMES=%v\n", os.Getenv("FUNC_E2E_RUNTIMES"))
	fmt.Fprintf(os.Stderr, "  (deprecated) E2E_FUNC_BIN=%v\n", os.Getenv("E2E_FUNC_BIN"))
	fmt.Fprintf(os.Stderr, "  (deprecated) E2E_REGISTRY_URL=%v\n", os.Getenv("E2E_REGISTRY_URL"))
	fmt.Fprintf(os.Stderr, "  (deprecated) E2E_RUNTIMES=%v\n", os.Getenv("E2E_RUNTIMES"))
	fmt.Fprintf(os.Stderr, "  (deprecated) E2E_USE_KN_FUNC=%v\n", os.Getenv("E2E_USE_KN_FUNC"))

	fmt.Fprintln(os.Stderr, "---------------------")
	fmt.Fprintln(os.Stderr, "Final Config:")
	// Bin - path to binary which will be used when running the tests.
	Bin = os.Getenv("E2E_FUNC_BIN") // Read in deprecated env first
	if Bin != "" {                  // warn if found
		fmt.Fprintln(os.Stderr, "WARNING:  The env var E2E_FUNC_BIN is deprecated and support will be removed in a future release.   Please use FUNC_E2E_BIN.")
	}
	if v := os.Getenv("FUNC_E2E_BIN"); v != "" { // overwrite with current env
		Bin = v
	}
	if Bin == "" { // Default
		Bin = DefaultBin
	}
	if !filepath.IsAbs(Bin) { // convert to abs
		v, err := filepath.Abs(Bin)
		if err != nil {
			panic(fmt.Sprintf("error converting path to absolute. %v", err))
		}
		Bin = v
	}
	fmt.Fprintf(os.Stderr, "  Bin=%v\n", Bin) // echo for verification

	// Plugin - if set, func is a plugin and Bin is the one plugging. The value
	// is the name of the subcommand.  If set to "true", for backwards compat
	// the default value is "func"
	Plugin = os.Getenv("E2E_USE_KN_FUNC") // read in the deprecated env
	if Plugin == "true" {
		fmt.Fprintln(os.Stderr, "WARNING:  The env var E2E_USE_KN_FUNC is deprecated and support will be removed in a future release.   Please use FUNC_E2E_PLUGIN and set to the value 'func'.")
		// "true" is for backwards compatibility.
		// The new env var is a string indicating the name of the plugin's
		// subcommand, which for that case was always "func"
		Plugin = "func"
	}
	if v := os.Getenv("FUNC_E2E_PLUGIN"); v != "" { // override with new
		Plugin = v
	}
	fmt.Fprintf(os.Stderr, "  Plugin=%v\n", Plugin) // echo

	// Registry - the registry URL including any account/repository at that
	// registry.  Example:  docker.io/alice.  Default is the local registry.
	Registry = os.Getenv("E2E_REGISTRY_URL") // read in the deprecated env
	if Registry != "" {
		fmt.Fprintln(os.Stderr, "WARNING: the env var E2E_REGISTRY_URL is deprecated and support will be removed in a future release.  Please use FUNC_E2E_REGISTRY.")
	}
	if v := os.Getenv("FUNC_E2E_REGISTRY"); v != "" { // overwrite with new
		Registry = v
	}
	if Registry == "" { // default
		Registry = DefaultRegistry
	}
	fmt.Fprintf(os.Stderr, "  Registry=%v\n", Registry) // echo

	// Runtimes - can optionally pass a list of runtimes to test, overriding
	// the default of testing all builtin runtimes.
	// Example "FUNC_E2E_RUNTIMES=go,python"
	runtimes := os.Getenv("E2E_RUNTIMES")
	if runtimes != "" {
		fmt.Fprintln(os.Stderr, "WARNING: the env var E2E_RUNTIMES is deprecated and support will be removed in a future release.  Please use FUNC_E2E_RUNTIMES and set to a comma-delimited list.")
		Runtimes = fromCSV(runtimes)
	}
	if runtimes = os.Getenv("FUNC_E2E_RUNTIMES"); runtimes != "" {
		Runtimes = fromCSV(runtimes)
	}
	if len(Runtimes) == 0 {
		Runtimes = DefaultRuntimes
	}
	fmt.Fprintf(os.Stderr, "  Runtimes=%v\n", toCSV(Runtimes))

	// Builders - can optionally pass a list of builders to test, overriding
	// the default of testing all. Example "FUNC_E2E_BUILDERS=pack,s2i"
	if builders := os.Getenv("FUNC_E2E_BUILDERS"); builders != "" {
		Builders = fromCSV(builders)
	}
	if len(Builders) == 0 {
		Builders = DefaultBuilders
	}
	fmt.Fprintf(os.Stderr, "  Builders=%v\n", toCSV(Builders))

	// Kubeconfig - the kubeconfig to pass ass KUBECONFIG env to test
	// environments.
	Kubeconfig = os.Getenv("FUNC_E2E_KUBECONFIG")
	if Kubeconfig == "" {
		Kubeconfig = DefaultKubeconfig
	}
	if !filepath.IsAbs(Kubeconfig) { // convert to abs
		v, err := filepath.Abs(Kubeconfig)
		if err != nil {
			panic(fmt.Sprintf("error converting path to absolute. %v", err))
		}
		Kubeconfig = v
	}
	fmt.Fprintf(os.Stderr, "  Kubeconfig=%v\n", Kubeconfig) // echo

	// Gocoverdir - the coverage directory to use while testing the go binary.
	Gocoverdir = os.Getenv("FUNC_E2E_GOCOVERDIR")
	if Gocoverdir == "" {
		Gocoverdir = DefaultGocoverdir
	}
	if !filepath.IsAbs(Gocoverdir) { // convert to abs
		v, err := filepath.Abs(Gocoverdir)
		if err != nil {
			panic(fmt.Sprintf("error converting path to absolute. %v", err))
		}
		Gocoverdir = v
	}
	fmt.Fprintf(os.Stderr, "  Gocoverdir=%v\n", Gocoverdir) // echo

	// Home is the default home directory, is not configurable (tests override
	// it on a case-by-case basis) and is merely set here to the absolute path
	// to DefaultHome (./testdata/default_home)
	var err error
	if Home, err = filepath.Abs(DefaultHome); err != nil {
		panic(fmt.Sprintf("error converting the relative default home value to absolute. %v", err))
	}

	Go = os.Getenv("FUNC_E2E_GO")
	if Go == "" {
		goBin, err := exec.LookPath("go")
		if err != nil {
			panic(fmt.Sprintf("error locating to 'go' executable. %v", err))
		}
		Go = goBin
	}
	if !filepath.IsAbs(Go) { // convert to abs
		v, err := filepath.Abs(Go)
		if err != nil {
			panic(fmt.Sprintf("error converting path to absolute. %v", err))
		}
		Go = v
	}
	fmt.Fprintf(os.Stderr, "  Go=%v\n", Go) // echo

	// Go binary path

	// Coverage
	// --------
	// Create Gocoverdir if it does not already exist
	// FIXME

	// Version
	// -------
	// Print version of func which is being used, taking into account if
	// we're running as a plugin.
	fmt.Fprintln(os.Stderr, "---------------------")
	fmt.Fprintln(os.Stderr, "Testing Func Version:")
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

	fmt.Fprintln(os.Stderr, "--- init complete ---")
	fmt.Fprintln(os.Stderr, "") // TODO: there is a superfluous linebreak on "func version".  This balances the whitespace.
}

// resetEnv removes environment variables from the process.
//
// Every test must be run with a nearly completely isolated environment,
// otherwise a developer's local environment will affect the E2E tests when run
// locally outside of CI.
//
// Some environment variables, provided via FUNC_E2E_* or other settings,
// are explicitly set here.
//
// For example, the system requires HOME to be set (WIP to remove requirement)
// so HOME is explicitly set to ./testdata/default_home, to be overridden
// as needed by tests which require specific home configuraitons for their
// execution.
func resetEnv() {
	// // Clear all except for those whitelisted
	// options := []string{
	// 	"FUNC_E2E_EXAMPLE_SETTING", // TODO: remove if not used
	// }
	// for _, env := range os.Environ() {
	// 	pair := strings.SplitN(env, "=", 2)
	// 	// t.Logf("unsetenv %v\n", pair)
	// 	if slices.Contains(options, pair[0]) {
	// 		continue
	// 	}
	// 	os.Unsetenv(pair[0])
	// 	t.Cleanup(func() {
	// 		if len(pair) == 2 { // t.Logf("setenv %v=%v\n", pair[0], pair[1])
	// 			os.Setenv(pair[0], pair[1])
	// 		} else if len(pair) == 1 {
	// 			// t.Logf("setenv %v\n", pair[0])
	// 			os.Setenv(pair[0], "")
	// 		} else {
	// 			panic(fmt.Sprintf("unexpected env length %v for env %v.", len(pair), env))
	// 		}
	// 	})
	// }
	os.Clearenv()
	os.Setenv("KUBECONFIG", Kubeconfig)
	os.Setenv("GOCOVERDIR", Gocoverdir)
	os.Setenv("HOME", Home)
	// Host builder is currently behind a feature flag.  The core tests rely
	// on it completely.
	os.Setenv("FUNC_ENABLE_HOST_BUILDER", "true")
	os.Setenv("FUNC_GO", Go)
}

// cdTmp changes to a new temporary directory for running the test.
// Essentially equvalent to creating a new directory before beginning to
// use func.  The path created is returned.
func cdTemp(t *testing.T) string {
	pwd, err := os.Getwd()
	if err != nil {
		panic(err)
	}
	tmp := filepath.Join(t.TempDir(), DefaultName)
	if err := os.MkdirAll(tmp, 0744); err != nil {
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
