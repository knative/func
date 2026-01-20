//go:build e2e
// +build e2e

package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	fn "knative.dev/func/pkg/functions"
	"knative.dev/func/pkg/knative"
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
	name := "func-e2e-test-core-init"
	root := fromCleanEnv(t, name)

	// Act (newCmd == "func ...")
	if err := newCmd(t, "init", "-l=go").Run(); err != nil {
		t.Fatal(err)
	}

	// Assert we got an initialized Function (language, name, root and spec)
	f, err := fn.NewFunction(root)
	if err != nil {
		t.Fatalf("expected an initialized function, but when reading it, got error. %v", err)
	}
	if f.Runtime != "go" {
		t.Fatalf("expected initialized function with runtime 'go' got '%v'", f.Runtime)
	}
	if f.Name != name {
		t.Fatalf("expected initialized function with name '%v' got '%v'", name, f.Name)
	}
	if f.Root != root {
		t.Fatalf("expected initialized function with root '%v' got '%v'", root, f.Root)
	}
	if f.SpecVersion == "" {
		t.Fatal("expected initialized function to have a spec version set")
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

	cmd := newCmd(t, "run", "--json")
	address := parseRunJSON(t, cmd)

	if !waitFor(t, address) {
		t.Fatal("service does not appear to have started correctly.")
	}

	// ^C the running function
	if err := cmd.Process.Signal(os.Interrupt); err != nil {
		fmt.Fprintf(os.Stderr, "error interrupting. %v", err)
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

	if !waitFor(t, ksvcUrl(name)) {
		t.Fatal("function did not deploy correctly")
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
	if !waitFor(t, ksvcUrl(name),
		withContentMatch(name)) {
		t.Fatal("function did not update correctly")
	}
}

// TestCore_Deploy_Source ensures that a function can be built and deployed
// locally from source code housed in a remote source repository.
// func deploy --git-url={url}
// func deploy --git-url={url} --git-ref={ref}
// func deploy --git-url={url} --git-ref={ref} --git-dir={subdir}
func TestCore_Deploy_Source(t *testing.T) {
	t.Log("Not Implemeted: running a local deploy from source code in a remote repo is not currently an implemented feature because this can be easily accomplished with `git clone ... && func deploy`")
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
	// if !waitForContent(t, fmt.Sprintf("http://func-e2e-test-deploy-source.%s.%s", Namespace, Domain), "func-e2e-test-deploy-source") {
	// 	t.Fatal("function did not update correctly")
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
	if !waitFor(t, ksvcUrl(name)) {
		t.Fatal("function did not deploy correctly")
	}

	// update
	update := `
	package function
	import "fmt"
	import "net/http"
	type Function struct{}
	func New() *Function { return &Function{} }
	func (f *Function) Handle(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprintln(w, "UPDATED")
	}
	`
	err := os.WriteFile(filepath.Join(root, "function.go"), []byte(update), 0644)
	if err != nil {
		t.Fatal(err)
	}
	if err := newCmd(t, "deploy").Run(); err != nil {
		t.Fatal(err)
	}
	if !waitFor(t, ksvcUrl(name),
		withContentMatch("UPDATED")) {
		t.Fatal("function did not update correctly")
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

	if !waitFor(t, ksvcUrl(name)) {
		t.Fatal("function did not deploy correctly")
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

	if err := newCmd(t, "init", "-l=go",
		"--repository", "https://github.com/functions-dev/templates",
		"-t", "echo").Run(); err != nil {
		t.Fatal(err)
	}

	// Test local invocation
	// ----------------------------------------
	// Runs the function locally, which `func invoke` will invoke when
	// it detects it is running.
	cmd := newCmd(t, "run", "--json")
	address := parseRunJSON(t, cmd)

	run := cmd // for the closure
	defer func() {
		// ^C the running function
		if err := run.Process.Signal(os.Interrupt); err != nil {
			fmt.Fprintf(os.Stderr, "error interrupting. %v", err)
		}
	}()

	if !waitFor(t, address+"?test-echo-param&message=test-echo-param",
		withContentMatch("test-echo-param")) {
		t.Fatal("service does not appear to have started correctly.")
	}

	// Check invoke
	checkInvoke := func(data string) {
		cmd = newCmd(t, "invoke", "--data="+data)
		out := bytes.Buffer{}
		cmd.Stdout = &out
		if err := cmd.Run(); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(out.String(), data) {
			t.Logf("out: %v", out.String())
			t.Fatal("function invocation did not echo data")
		}
	}
	checkInvoke("func-e2e-test-core-invoke-local")

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

	if !waitFor(t, ksvcUrl(name)+"?test-echo-param&message=test-echo-param",
		withContentMatch("test-echo-param")) {
		t.Fatal("function did not deploy correctly")
	}

	checkInvoke("func-e2e-test-core-invoke-remote")
}

// TestCore_StaticSignature ensures backward compatibility with the static
// (non-instanced) function signature. Functions can use either:
// - Instanced: type MyFunction struct{} + New() + Handle method (default)
// - Static: package-level func Handle(...) in handle.go (legacy, still supported)
func TestCore_StaticSignature(t *testing.T) {
	name := "func-e2e-test-core-static"
	root := fromCleanEnv(t, name)

	// Create func.yaml
	funcYaml := fmt.Sprintf(`specVersion: %s
name: %s
runtime: go
created: 2025-01-01T00:00:00Z
`, fn.LastSpecVersion(), name)
	if err := os.WriteFile(filepath.Join(root, "func.yaml"), []byte(funcYaml), 0644); err != nil {
		t.Fatal(err)
	}

	// Create go.mod
	goMod := "module function\n\ngo 1.23\n"
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte(goMod), 0644); err != nil {
		t.Fatal(err)
	}

	// Create handle.go with static signature
	handleGo := `package function

import (
	"fmt"
	"net/http"
)

func Handle(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintln(w, "OK")
}
`
	if err := os.WriteFile(filepath.Join(root, "handle.go"), []byte(handleGo), 0644); err != nil {
		t.Fatal(err)
	}

	// Deploy
	if err := newCmd(t, "deploy").Run(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		clean(t, name, Namespace)
	}()

	// Verify - waitFor defaults to checking for "OK"
	if !waitFor(t, ksvcUrl(name)) {
		t.Fatal("static signature function did not deploy correctly")
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
	if !waitFor(t, ksvcUrl(name)) {
		t.Fatal("function did not deploy correctly")
	}

	// Check it appears in the list
	client := fn.New(fn.WithListers(knative.NewLister(false)))
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
