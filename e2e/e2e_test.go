//go:build e2e
// +build e2e

/*
Package e2e provides an end-to-end test suite for the Functions CLI "func".

See README.md for more details.
*/
package e2e

import (
	"bufio"
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
	"text/template"
	"time"

	fn "knative.dev/func/pkg/functions"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"knative.dev/func/pkg/k8s"
)

const (
	// DefaultBin is the default binary to run, relative to this test file.
	// This is the binary built by default when running 'make'.
	// This can be customized with FUNC_E2E_BIN.
	// Note this is always relative to this test file.
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

	// DefaultDomain for E2E tests. Defaults to "localtest.me" but can be
	// overridden using FUNC_E2E_DOMAIN environment variable. This domain
	// must be properly configured in the cluster's DNS and Knative serving.
	DefaultDomain = "localtest.me"

	// DefaultRegistry to use when running the e2e tests.  This is the URL
	// of the registry created by default when using the cluster.sh script
	// to set up a local testing cluster, but can be customized with
	// FUNC_E2E_REGISTRY.
	DefaultRegistry = "localhost:50000/func"

	// DefaultClusterRegistry to use for in-cluster (remote) builds.
	// This is the cluster-internal registry URL accessible from within
	// the cluster for Tekton builds. Can be customized with
	// FUNC_E2E_CLUSTER_REGISTRY.
	DefaultClusterRegistry = "registry.default.svc.cluster.local:5000/func"

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

	// Domain is the DNS domain suffix used for constructing function URLs
	// during tests. Defaults to "localtest.me". When using a custom domain,
	// ensure it is configured in the cluster's CoreDNS and Knative serving
	// config-domain. The pattern is: http://{function}.{namespace}.{domain}
	// Can be set with FUNC_E2E_DOMAIN
	Domain string

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
	// ensure DNS is configured for {function}.{namespace}.{domain} patterns.
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

	// ClusterRegistry is the cluster-internal container registry to use
	// for in-cluster (remote) builds with Tekton. This is the registry
	// accessible from within the cluster.
	// Can be set with FUNC_E2E_CLUSTER_REGISTRY
	ClusterRegistry string

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

	// Dump full environment when CI_DEBUGGING is set
	// (useful primarily when debugging a CI environment)
	if os.Getenv("CI_DEBUGGING") != "" {
		fmt.Fprintln(os.Stderr, "--  Initial Environment: ")
		for _, env := range os.Environ() {
			fmt.Fprintln(os.Stderr, "  ", env)
		}
		fmt.Fprintln(os.Stderr, "---------------------")
	}

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
	fmt.Fprintf(os.Stderr, "  FUNC_E2E_DOMAIN=%v\n", os.Getenv("FUNC_E2E_DOMAIN"))
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
	fmt.Fprintf(os.Stderr, "  Domain=%v\n", Domain)
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
	fmt.Fprintf(os.Stderr, "  ClusterRegistry=%v\n", ClusterRegistry)
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
	printVersion() // TODO: `version` outputs a superfluous linebreak
	fmt.Fprintln(os.Stderr, "--- init complete ---")
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

	// Domain - the DNS domain suffix for function URLs
	Domain = getEnv("FUNC_E2E_DOMAIN", "", DefaultDomain)

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

	// ClusterRegistry - the cluster-internal registry URL for in-cluster builds
	ClusterRegistry = getEnv("FUNC_E2E_CLUSTER_REGISTRY", "", DefaultClusterRegistry)

	// Verbose env as a truthy boolean
	Verbose = getEnvBool("FUNC_E2E_VERBOSE", "", DefaultVerbose)

	// Tools - the path to supporting tools.
	Tools = getEnvPath("FUNC_E2E_TOOLS", "", DefaultTools)

	// Testdata - the path to supporting testdata
	Testdata = getEnvPath("FUNC_E2E_TESTDATA", "", DefaultTestdata)
}

// ----------------------------------------------------------------------------
// Helpers
// ----------------------------------------------------------------------------

// fromCleanEnv provides a clean environment for a function E2E test.
func fromCleanEnv(t *testing.T, name string) (root string) {
	root = cdTemp(t, name)
	setupEnv(t)
	return
}

// cdTmp changes to a new temporary directory for running the test.
// Essentially equivalent to creating a new directory before beginning to
// use func.  The path created is returned.
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
// will be set as well. Arguments are set to those provided.
func newCmd(t *testing.T, args ...string) *exec.Cmd {
	t.Helper()
	bin := Bin

	// If Plugin provided, it is a subcommand so prepend it to args.
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
}

type waitOption func(*waitCfg)

type waitCfg struct {
	timeout       time.Duration // total time to wait
	interval      time.Duration // time between retries
	warnThreshold time.Duration // start warning after this long
	warnInterval  time.Duration // only warn this often
	content       string        // match content vs default HTTP OK
	template      string        // template name
}

func withWaitTimeout(t time.Duration) waitOption {
	return func(cfg *waitCfg) {
		cfg.timeout = t
	}
}

func withTemplate(t string) waitOption {
	return func(cfg *waitCfg) {
		cfg.template = t
	}
}

func withContentMatch(c string) waitOption {
	return func(cfg *waitCfg) {
		cfg.content = c
	}
}

func waitFor(t *testing.T, address string, options ...waitOption) (ok bool) {
	t.Helper()
	cfg := waitCfg{}
	for _, o := range options {
		o(&cfg)
	}
	if cfg.template == "cloudevents" {
		return waitForCloudevent(t, address, options...)
	} else if cfg.content != "" {
		return waitForContent(t, address, cfg.content, "expected content not found", options...)
	} else {
		return waitForContent(t, address, "OK", "expected OK response", options...)
	}
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

// waitFor an endpoint to return an OK response which includes the given
// content.
//
// If the Function returns a 500, it is considered a positive test failure
// by the implementation and retries are discontinued.
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
func waitForContent(t *testing.T, address, content, errMsg string, options ...waitOption) (ok bool) {
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
	t.Helper()
	if err == nil {
		return false // no error is not an abnormal error.
	}
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
	t.Helper()
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
	t.Helper()
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

	// Get image name for this specific function
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

// parseRunJSON runs the command and extracts the function address from JSON output.
// We scan line-by-line because container builders (s2i/pack) may print build logs
// to stdout before the JSON, so we need to skip those lines to find valid JSON.
func parseRunJSON(t *testing.T, cmd *exec.Cmd) string {
	t.Helper()

	stdoutReader, stdoutWriter := io.Pipe()
	stderr := &bytes.Buffer{}
	cmd.Stdout = stdoutWriter
	cmd.Stderr = stderr

	type runOutput struct {
		Address string `json:"address"`
		Host    string `json:"host"`
		Port    string `json:"port"`
	}

	addressChan := make(chan string, 1)
	errChan := make(chan error, 1)

	// Must spawn reader before starting the command, otherwise we may miss output
	go func() {
		scanner := bufio.NewScanner(stdoutReader)
		for scanner.Scan() {
			line := scanner.Text()
			if strings.HasPrefix(strings.TrimSpace(line), "{") {
				var result runOutput
				if err := json.Unmarshal([]byte(line), &result); err == nil && result.Address != "" {
					addressChan <- result.Address
					io.Copy(io.Discard, stdoutReader) // Prevent command from blocking on full pipe
					return
				}
			}
		}
		if err := scanner.Err(); err != nil {
			errChan <- fmt.Errorf("error reading stdout: %w", err)
		} else {
			errChan <- fmt.Errorf("no JSON output found in stdout")
		}
	}()

	// Start the command
	if err := cmd.Start(); err != nil {
		t.Fatalf("failed to start command: %v", err)
	}

	var address string
	select {
	case address = <-addressChan:
		t.Logf("Function running on %s (from JSON output)", address)
	case err := <-errChan:
		t.Fatalf("JSON parsing error: %v\nstderr: %s", err, stderr.String())
	case <-time.After(5 * time.Minute):
		t.Fatalf("timeout waiting for func run JSON output. stderr: %s", stderr.String())
	}

	t.Cleanup(func() { stdoutWriter.Close() })
	return address
}

func ksvcUrl(name string) string {
	// TODO get `default-external-scheme` from the config in cluster
	const ksvcDefaultExternalScheme = `http`
	// TODO get `domain-template` from the config in cluster
	const ksvcDomainTemplate = `{{.Name}}-{{.Namespace}}-ksvc.{{.Domain}}`
	t, err := template.New("").Parse(ksvcDomainTemplate)
	if err != nil {
		panic(err)
	}
	var buf bytes.Buffer
	err = t.Execute(&buf, struct {
		Name, Namespace, Domain string
	}{name, Namespace, Domain})
	if err != nil {
		panic(err)
	}
	return ksvcDefaultExternalScheme + `://` + buf.String()
}
