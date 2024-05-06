/*
Package e2e provides an end-to-end test suite for the Functions CLI "func".

See README.md for more details.
*/
package e2e

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"knative.dev/func/cmd"
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

	// DefaultNamespace for E2E tests is that used by deafult in the
	// CLI being tested.
	DefaultNamespace = cmd.DefaultNamespace
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

	// Matrix indicates a full matrix test should be run.  Defaults to false.
	// Enable with FUNC_E2E_MATRIX=true
	Matrix bool

	// MatrixRuntimes for which runtime-specific tests should be run.  Defaults
	// to all core language runtimes.  Can be set with FUNC_E2E_MATRIX_RUNTIMES
	MatrixRuntimes = []string{"go", "python", "node", "rust", "typescript", "quarkus", "springboot"}

	// MatrixBuilders specifies builders to check in addition to the "host"
	// builder which is used
	// by default.  Used for Builder-specific tests.  Can be set with
	// FUNC_E2E_MATRIX_BUILDERS.
	MatrixBuilders = []string{"host", "pack", "s2i"}

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

	// Clean
	Clean bool // wait for each test function to be removed before continuing

	// Verbose mode for all command runs.
	Verbose bool
)

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
	// Assemble
	resetEnv()
	name := "func-e2e-test-core-init"
	root := cdTemp(t, name)

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
	resetEnv()
	name := "func-e2e-test-core-run"
	_ = cdTemp(t, name) // sets Function name obliquely, see docs

	if err := newCmd(t, "init", "-l=go").Run(); err != nil {
		t.Fatal(err)
	}

	cmd := newCmd(t, "run")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}

	// TODO: implement structured command output (ex --json or --output=json),
	// parse it, and use that to find the listen address.
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

// TestCore_Deploy ensures that a function can be deployed to the cluster.
//
//	func deploy
func TestCore_Deploy(t *testing.T) {
	resetEnv()
	name := "func-e2e-test-core-deploy"
	_ = cdTemp(t, name) // sets Function name obliquely, see function docs

	if err := newCmd(t, "init", "-l=go").Run(); err != nil {
		t.Fatal(err)
	}

	cmd := newCmd(t, "deploy")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		clean(t, name, DefaultNamespace)
	}()

	if err := cmd.Wait(); err != nil {
		t.Fatalf("deploy error. %v", err)
	}

	if !waitFor(t, "http://func-e2e-test-deploy.default.127.0.0.1.sslip.io") {
		t.Fatalf("function did not deploy correctly")
	}
}

// TestCore_Deploy_Template ensures that the system supports creating
// functions based off templates in a remote repository.
func TestCore_Deploy_Template(t *testing.T) {
	resetEnv()
	name := "func-e2e-test-core-deploy-template"
	_ = cdTemp(t, name) // sets Function name obliquely, see function docs

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
		clean(t, name, DefaultNamespace)
	}()

	// The default implementation responds with HTTP 200 and the string
	// "testcore-deploy-template" for all requests.
	if !waitForContent(t, "http://func-e2e-test-deploy-template.default.127.0.0.1.sslip.io", "func-e2e-test-deploy-template") {
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
	// resetEnv()
	// name := "func-e2e-test-deploy-source"
	// _ = cdTemp(t, name) // sets Function name obliquely, see function docs
	//
	// if err := newCmd(t, "deploy", "--git-url=https://github.com/functions-dev/func-e2e-tests").Run(); err != nil {
	// 	t.Fatal(err)
	// }
	// defer func() {
	// 	clean(t, name, DefaultNamespace)
	// }()
	// if !waitForContent(t, "http://func-e2e-test-deploy-source.default.127.0.0.1.sslip.io", "func-e2e-test-deploy-source") {
	// 	t.Fatalf("function did not update correctly")
	// }
}

// TestCore_Update ensures that a running function can be updated.
//
// func deploy
// TODO: merge with TestCore_Deploy and note it is an "upsert"
func TestCore_Update(t *testing.T) {
	resetEnv()
	name := "func-e2e-test-core-update"
	root := cdTemp(t, name) // sets Function name obliquely, see function docs

	// create
	if err := newCmd(t, "init", "-l=go").Run(); err != nil {
		t.Fatal(err)
	}

	// deploy
	if err := newCmd(t, "deploy").Run(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		clean(t, name, DefaultNamespace)
	}()
	if !waitFor(t, "http://func-e2e-test-core-update.default.127.0.0.1.sslip.io") {
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
	if !waitForContent(t, "http://func-e2e-test-core-update.default.127.0.0.1.sslip.io", "UPDATED") {
		t.Fatalf("function did not update correctly")
	}
}

// TestCore_Describe ensures that describing a function accurately represents
// its expected state.
//
//	func describe
func TestCore_Describe(t *testing.T) {
	// TODO
	t.Log("Not Implemented")
}

// TestCore_Invoke ensures that the invoke helper functions for both
// local and remote function instances.
//
//	func invoke
func TestCore_Invoke(t *testing.T) {
	t.Log("Not Implemented")
	resetEnv()
	name := "func-e2e-test-core-invoke"
	_ = cdTemp(t, name) // sets Function name obliquely, see function docs

	if err := newCmd(t, "init", "-l=go").Run(); err != nil {
		t.Fatal(err)
	}

	// Test local invocation
	// ----------------------------------------
	// Runs the funciton locally, which `func invoke` will invoke when
	// it detects it is running.
	cmd := newCmd(t, "run")
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
	if !waitFor(t, "http://localhost:8080") {
		t.Fatalf("service does not appear to have started correctly.")
	}
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
		clean(t, name, DefaultNamespace)
	}()
	if !waitFor(t, "http://func-e2e-test-core-invoke.default.127.0.0.1.sslip.io") {
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

// TestCore_delete ensures that a function registered as deleted when deleted.
// Also tests list as a side-effect.
//
//	func delete
func TestCore_delete(t *testing.T) {
	name := "func-e2e-test-core-delete"
	_ = cdTemp(t, name) // sets Function name obliquely, see function docs

	// create
	if err := newCmd(t, "init", "-l=go").Run(); err != nil {
		t.Fatal(err)
	}

	// deploy
	if err := newCmd(t, "deploy").Run(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		clean(t, name, DefaultNamespace)
	}()
	if !waitFor(t, "http://func-e2e-test-core-delete.default.127.0.0.1.sslip.io") {
		t.Fatalf("function did not deploy correctly")
	}

	client := fn.New(fn.WithLister(knative.NewLister(false)))
	list, err := client.List(context.Background(), DefaultNamespace)
	if err != nil {
		t.Fatal(err)
	}

	if !containsInstance(t, list, name, DefaultNamespace) {
		t.Logf("list: %v", list)
		t.Fatal("Instance list did not contain the 'delete' test service")
	}

	if err := newCmd(t, "delete").Run(); err != nil {
		t.Logf("Error deleting function. %v", err)
	}

	list, err = client.List(context.Background(), DefaultNamespace)
	if err != nil {
		t.Fatal(err)
	}

	if containsInstance(t, list, name, DefaultNamespace) {
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
	resetEnv()
	name := "func-e2e-test-metadata-envs-add"
	root := cdTemp(t, name)

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
	setSecret(t, "test-secret-single", DefaultNamespace, map[string][]byte{
		"C": []byte("c"),
	})
	if err := newCmd(t, "config", "envs", "add",
		"--name=C", "--value={{secret:test-secret-single:C}}").Run(); err != nil {
		t.Fatal(err)
	}

	// Set Env: from all the keys in a secret (multi)
	setSecret(t, "test-secret-multi", DefaultNamespace, map[string][]byte{
		"D": []byte("d"),
		"E": []byte("e"),
	})
	if err := newCmd(t, "config", "envs", "add",
		"--value={{secret:test-secret-multi}}").Run(); err != nil {
		t.Fatal(err)
	}

	// Set Env: from cluster config map (single)
	setConfigMap(t, "test-config-map-single", DefaultNamespace, map[string]string{
		"F": "f",
	})
	if err := newCmd(t, "config", "envs", "add",
		"--name=F", "--value={{configMap:test-config-map-single:F}}").Run(); err != nil {
		t.Fatal(err)
	}

	// Set Env: from all keys in a configMap (multi)
	setConfigMap(t, "test-config-map-multi", DefaultNamespace, map[string]string{
		"G": "g",
		"H": "h",
	})
	if err := newCmd(t, "config", "envs", "add",
		"--value={{configMap:test-config-map-multi}}").Run(); err != nil {
		t.Fatal(err)
	}

	// The test funciton will respond HTTP 500 unless all defined environment
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
		clean(t, name, DefaultNamespace)
	}()
	if !waitForContent(t, "http://func-e2e-test-metadata-envs-add.default.127.0.0.1.sslip.io", "OK") {
		t.Fatalf("handler failed")
	}

	// Set a test Environment Variable
	// Add
}

// TestMetadata_Envs_Remove ensures that environment variables can be removed.
//
//	func config envs remove --name={name}
func TestMetadata_Envs_Remove(t *testing.T) {
	// The ability to remove an env via a command appears to never have been
	// implemented (`func envs remove --name=B`).
	// t.Skip("This feature is not yet implemented")
	resetEnv()
	name := "func-e2e-test-metadata-envs-remove"
	root := cdTemp(t, name)

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
		clean(t, name, DefaultNamespace)
	}()
	if !waitForContent(t, "http://func-e2e-test-metadata-envs-remove.default.127.0.0.1.sslip.io", "OK") {
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
	if !waitForContent(t, "http://func-e2e-test-metadata-envs-remove.default.127.0.0.1.sslip.io", "OK") {
		t.Fatalf("handler failed")
	}
}

// TestMetadata_Labels ensures that labels added via the CLI are
// carried through to the final service, and can subsequently be removed.
//
// func config labels add
// func config labels remove
func TestMetadata_Labels(t *testing.T) {
	// Note: As with the environment variable's "remove" feature, this was
	// also not implemented (interactive only).  The following commands
	// need to be implemented, and then E2E tested here:

	// By static value
	// func config labels add --name=A --value="a"
	t.Log("Not Implemented: static label value")

	// From environment variable
	// func config labels add --name=B --value="{{env:B}}"
	t.Log("Not Implemented: label value from env")
}

// TestMetadta_Volumes ensures that adding volumes of various types are made
// available to the running function, and can subsequently be removed
//
// func config volumes add
// func config volumes remove
func TestMetadata_Volumes(t *testing.T) {
	// Note: as with both environment variable "remove" functionality and
	// labels, volumes are also missing a way to set them via a command
	// (noninteractively); and through the interactive prompts, only the
	// "emptyDir" type.  The following commands (or similar) need to be
	// implemented and tested:

	// ConfigMap as a Volume
	// func config volume add --type=configmap --name={map name} --path={path}
	t.Log("Not Implemented: test of configMap as volume")

	// Secret as a Volume
	// func config volume add --type=secret --name={secret name} --path={path}
	t.Log("Not Implemented: test of secret as volume")

	// PersistentVolumeClaim
	// func config volume add --type=pvc|claim --name={pvc name} --path={path}
	t.Log("Not Implemented: test of pvc volume")

	// EmptyDir
	// func config volumes add --path={path}
	t.Log("Not Implemented: test of emptyDir volume")
}

// TestMetadata_Subscriptions ensures that function instances can be
// subscribed to events.
func TestMetadata_Subscriptions(t *testing.T) {
	// Create a function which emits an event with as much defaults as possible
	// Create a function which subscribes to those events
	// Succeed the test as soon as it receives the event
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
	resetEnv()
	name := "func-e2e-test-remote-deploy"
	_ = cdTemp(t, name) // sets Function name obliquely, see function docs

	if err := newCmd(t, "init", "-l=go").Run(); err != nil {
		t.Fatal(err)
	}
	if err := newCmd(t, "deploy", "--remote", "--builder=pack", "--registry=func-registry:50000/func").Run(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		clean(t, name, DefaultNamespace)
	}()

	if !waitFor(t, "http://func-e2e-test-remote-deploy.default.127.0.0.1.sslip.io") {
		t.Fatalf("function did not deploy correctly")
	}

}

// TestRemote_Sourece ensures a remote build can be triggered which pulls
// source from a remote repository.
//
//	func deploy --remote --git-url={url}
func TestRemote_Source(t *testing.T) {

}

// TestRemote_Ref ensures a remote build can be triggered which pulls sourece
// from a specific reference (branch/tag) of a remote repository.
func TestRemote_Ref(t *testing.T) {

}

// TestRemote_Dir ensures that remote builds can be instructed to build and
// deploy a funciton located in a subdirectory.
//
//	func deploy --remote --git-dir={subdir}
//	func deploy --remote --git-dir={subdir} --git-url={url}
func TestRemote_Dir(t *testing.T) {

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
//		Template: http, CloudEvent
//		Builder:  Host, Pack, S2I
//		Source:   Local, Remote HEAD, Remote REF
//
//	 Test it can:
//	 1.  Run locally on the host
//	 2.  Run locally within a container
//	 3.  Deploy and run
//	 4.  Deply and run via a remote build

var unsupported = []struct {
	Runtime  string // go, python, node, typescript rust,
	Builder  string // host, pack, s2i
	Template string // http, cloudevent
	Source   string // local, remote, remote-ref
	Test     string // run, deploy, remote
}{
	{Runtime: "go", Builder: "s2i", Test: "remote"},
}

// ---------------------------------------------------------------------------
func TestMatrix(t *testing.T) {
	t.Log("Not Implemented")
	// For each runtime
	//  for each builder
	//    for both templates
	//      - run locally on host
	//      - run locally in container
	//      - Deploy local code
	//      - Deploy using on-cluster builds (--remote)
	//      - Deploy using on-cluster builds referring to a remote repo
	//      - Deploy using on-cluster builds referring to a remote repo w/ ref
}

// ----------------------------------------------------------------------------
// Test Helpers
// ----------------------------------------------------------------------------

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
			t.Logf("Response: %s\n", body)
			continue
		}
		return true
	}
	t.Logf("Could not contact function after %v tries", retries)
	return
}

// waitForContent returns true if there is a service listening at the
// given addresss which responds HTTP OK with the given string in its body.
// returns false if the.
// If the Function returns a 500, it is considered a positive test failure
// by the implementation and retries are discontinued.
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
		if res.StatusCode == 500 {
			t.Log("500 response received; canceling retries.")
			t.Logf("Response: %s\n", body)
			return false
		}
		if !strings.Contains(string(body), content) {
			t.Log("Response received, but it did not contain the expected content.")
			t.Logf("Response: %s\n", body)
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

func clean(t *testing.T, name, ns string) {
	// There is currently a bug in delete which hangs for several seconds
	// when deleting a Function. This adds considerably to the test suite
	// execution time.  Tests are written such that they are not dependent
	// on a clean exit/cleanup, so this step is skipped for speed.
	if Clean {
		if err := newCmd(t, "delete", name, "--namespace", ns).Run(); err != nil {
			t.Logf("Error deleting function. %v", err)
		}
	}
}

// ----------------------------------------------------------------------------
// Test Initialization
// ----------------------------------------------------------------------------
// Deprecated ENV       Current ENV                   Final Variable
// ---------------------------------------------------
// E2E_FUNC_BIN      => FUNC_E2E_BIN               => Bin
// E2E_USE_KN_FUNC   => FUNC_E2E_PLUGIN            => Plugin
// E2E_REGISTRY_URL  => FUNC_E2E_REGISTRY          => Registry
// E2E_RUNTIMES      => FUNC_E2E_MATRIX_RUNTIMES   => MatrixRuntimes
//                      FUNC_E2E_MATRIX_BUILDERS   => MatrixBuilders
//                      FUNC_E2E_MATRIX            => Matrix
//                      FUNC_E2E_KUBECONFIG        => Kubeconfig
//                      FUNC_E2E_GOCOVERDIR        => Gocoverdir
//                      FUNC_E2E_GO                => Go
//                      FUNC_E2E_GIT               => Git

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
	fmt.Fprintf(os.Stderr, "  FUNC_E2E_GIT=%v\n", os.Getenv("FUNC_E2E_GIT"))
	fmt.Fprintf(os.Stderr, "  FUNC_E2E_GO=%v\n", os.Getenv("FUNC_E2E_GO"))
	fmt.Fprintf(os.Stderr, "  FUNC_E2E_GOCOVERDIR=%v\n", os.Getenv("FUNC_E2E_GOCOVERDIR"))
	fmt.Fprintf(os.Stderr, "  FUNC_E2E_KUBECONFIG=%v\n", os.Getenv("FUNC_E2E_KUBECONFIG"))
	fmt.Fprintf(os.Stderr, "  FUNC_E2E_MATRIX=%v\n", os.Getenv("FUNC_E2E_MATRIX"))
	fmt.Fprintf(os.Stderr, "  FUNC_E2E_MATRIX_BUILDERS=%v\n", os.Getenv("FUNC_E2E_MATRIX_BUILDERS"))
	fmt.Fprintf(os.Stderr, "  FUNC_E2E_MATRIX_RUNTIMES=%v\n", os.Getenv("FUNC_E2E_MATRIX_RUNTIMES"))
	fmt.Fprintf(os.Stderr, "  FUNC_E2E_PLUGIN=%v\n", os.Getenv("FUNC_E2E_PLUGIN"))
	fmt.Fprintf(os.Stderr, "  FUNC_E2E_REGISTRY=%v\n", os.Getenv("FUNC_E2E_REGISTRY"))
	fmt.Fprintf(os.Stderr, "  FUNC_E2E_VERBOSE=%v\n", os.Getenv("FUNC_E2E_VERBOSE"))
	fmt.Fprintf(os.Stderr, "  FUNC_E2E_CLEAN=%v\n", os.Getenv("FUNC_E2E_CLEAN"))
	fmt.Fprintf(os.Stderr, "  (deprecated) E2E_FUNC_BIN=%v\n", os.Getenv("E2E_FUNC_BIN"))
	fmt.Fprintf(os.Stderr, "  (deprecated) E2E_REGISTRY_URL=%v\n", os.Getenv("E2E_REGISTRY_URL"))
	fmt.Fprintf(os.Stderr, "  (deprecated) E2E_RUNTIMES=%v\n", os.Getenv("E2E_RUNTIMES"))
	fmt.Fprintf(os.Stderr, "  (deprecated) E2E_USE_KN_FUNC=%v\n", os.Getenv("E2E_USE_KN_FUNC"))

	fmt.Fprintln(os.Stderr, "---------------------")
	// Read them all into their final variables
	readEnvs()

	fmt.Fprintln(os.Stderr, "Final Config:")
	fmt.Fprintf(os.Stderr, "  Bin=%v\n", Bin)
	fmt.Fprintf(os.Stderr, "  Git=%v\n", Git)
	fmt.Fprintf(os.Stderr, "  Go=%v\n", Go)
	fmt.Fprintf(os.Stderr, "  Kubeconfig=%v\n", Kubeconfig)
	fmt.Fprintf(os.Stderr, "  Matrix=%v\n", Matrix)
	fmt.Fprintf(os.Stderr, "  MatrixBuilders=%v\n", toCSV(MatrixBuilders))
	fmt.Fprintf(os.Stderr, "  MatrixRuntimes=%v\n", toCSV(MatrixRuntimes))
	fmt.Fprintf(os.Stderr, "  Plugin=%v\n", Plugin)
	fmt.Fprintf(os.Stderr, "  Registry=%v\n", Registry)
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
	Bin = getEnvPath("FUNC_E2E_BIN", "E2E_FUNC_BIN", DefaultBin)
	// Final =          current ENV, deprecated ENV, default

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

	// Matrix - optionally enable matrix test
	Verbose = getEnvBool("FUNC_E2E_MATRIX", "", false)

	// Runtimes - can optionally pass a list of runtimes to test, overriding
	// the default of testing all builtin runtimes.
	// Example "FUNC_E2E_MATRIX_RUNTIMES=go,python"
	MatrixRuntimes = getEnvList("FUNC_E2E_MATRIX_RUNTIMES", "E2E_RUNTIMES", toCSV(MatrixRuntimes))

	// Builders - can optionally pass a list of builders to test, overriding
	// the default of testing all. Example "FUNC_E2E_MATRIX_BUILDERS=pack,s2i"
	MatrixBuilders = getEnvList("FUNC_E2E_MATRIX_BUILDERS", "", toCSV(MatrixBuilders))

	// Kubeconfig - the kubeconfig to pass ass KUBECONFIG env to test
	// environments.
	Kubeconfig = getEnvPath("FUNC_E2E_KUBECONFIG", "", DefaultKubeconfig)

	// Gocoverdir - the coverage directory to use while testing the go binary.
	Gocoverdir = getEnvPath("FUNC_E2E_GOCOVERDIR", "", DefaultGocoverdir)

	// Go binary path
	Go = getEnvBin("FUNC_E2E_GO", "", "go")

	// Git binary path
	Git = getEnvBin("FUNC_E2E_GIT", "", "git")

	// Clean up deployed functions before starting next test
	Clean = getEnvBool("FUNC_E2E_CLEAN", "", false)

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
