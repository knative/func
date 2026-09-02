//go:build e2e

package e2e

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// REMOTE TESTS
// Tests related to invoking processes remotely (in-cluster).
// All remote tests presume the cluster has Tekton installed.
// ---------------------------------------------------------------------------

// TestRemote_Deploy ensures that the default action of running a remote
// build succeeds:  uploading local source code to the cluster for build and
// deploy in-cluster.
//
//	func deploy --remote
func TestRemote_Deploy(t *testing.T) {
	name := "func-e2e-test-remote-deploy"
	_ = fromCleanEnv(t, name)

	if err := newCmd(t, "init", "-l=go").Run(); err != nil {
		t.Fatal(err)
	}
	if err := newCmd(t, "deploy", "--remote", "--builder=pack", fmt.Sprintf("--registry=%s", Registry)).Run(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		clean(t, name, Namespace)
	}()

	if !waitFor(t, ksvcUrl(name)) {
		t.Fatal("function did not deploy correctly")
	}
}

// TestRemote_Source ensures a remote build can be triggered which pulls
// source from a remote repository, with no local copy of the function.
//
//	func deploy --remote --git-url={url} --registry={} --builder=pack
func TestRemote_Source(t *testing.T) {
	name := "func-e2e-test-remote-source"
	_ = fromCleanEnv(t, name)

	// Trigger the deploy from an empty directory: the function is read from
	// the repository.
	if err := newCmd(t, "deploy", "--remote",
		"--git-url", "https://github.com/functions-dev/func-e2e-tests",
		"--registry", Registry,
		"--builder", "pack",
	).Run(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		clean(t, name, Namespace)
	}()

	if !waitFor(t, ksvcUrl(name),
		withContentMatch(name)) {
		t.Fatal("function did not deploy correctly")
	}

}

// TestRemote_Ref ensures a remote build can be triggered which pulls
// source from a specific reference (branch/tag) of a remote repository.
// The function's metadata (name, runtime, etc) is read from that reference,
// so no local checkout is involved.
func TestRemote_Ref(t *testing.T) {
	name := "func-e2e-test-remote-ref"
	_ = fromCleanEnv(t, name)

	// Trigger the deploy
	if err := newCmd(t, "deploy", "--remote",
		"--git-url", "https://github.com/functions-dev/func-e2e-tests",
		"--git-branch", name,
		"--registry", Registry,
		"--builder", "pack",
		"--build",
	).Run(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		clean(t, name, Namespace)
	}()

	if !waitFor(t, ksvcUrl(name),
		withContentMatch(name)) {
		t.Fatal("function did not deploy correctly")
	}
}

// TestRemote_Dir ensures that remote builds can be instructed to build and
// deploy a function located in a subdirectory of the repository. The
// function's metadata is read from that subdirectory.
//
//	func deploy --remote --git-dir={subdir} --git-url={url}
func TestRemote_Dir(t *testing.T) {
	name := "func-e2e-test-remote-dir"
	_ = fromCleanEnv(t, name)

	// Trigger the deploy
	if err := newCmd(t, "deploy", "--remote",
		"--git-url", "https://github.com/functions-dev/func-e2e-tests",
		"--git-dir", name,
		"--registry", Registry,
		"--builder", "pack",
		"--build",
	).Run(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		clean(t, name, Namespace)
	}()

	if !waitFor(t, ksvcUrl(name),
		withContentMatch(name)) {
		t.Fatal("function did not deploy correctly")
	}
}

// TestRemote_Deploy_InClusterRegistry ensures that the in-cluster dialer
// tunneling path works correctly by using the cluster-internal registry URL.
//
//	func deploy --remote --registry=registry.default.svc.cluster.local:5000/func
func TestRemote_Deploy_InClusterRegistry(t *testing.T) {
	name := "func-e2e-test-remote-incluster"
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

	if !waitFor(t, ksvcUrl(name)) {
		t.Fatal("function did not deploy correctly")
	}
}

// TestRemote_Update ensures that redeploying via the remote/Tekton path after
// changing the function's source code actually serves the new code. This is the
// remote analogue of TestCore_Update / TestMatrix_Python_Update, which only cover
// the local build+deploy path.
//
//	func deploy --remote --builder=pack
func TestRemote_Update(t *testing.T) {
	name := "func-e2e-test-remote-update"
	root := fromCleanEnv(t, name)

	// create
	if err := newCmd(t, "init", "-l=go").Run(); err != nil {
		t.Fatal(err)
	}

	// initial remote build + deploy
	if err := newCmd(t, "deploy", "--remote", "--builder=pack", fmt.Sprintf("--registry=%s", Registry)).Run(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		clean(t, name, Namespace)
	}()
	if !waitFor(t, ksvcUrl(name), withWaitTimeout(5*time.Minute)) {
		t.Fatal("function did not deploy correctly")
	}

	// update: rewrite the handler to return a new response body
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
	if err := os.WriteFile(filepath.Join(root, "function.go"), []byte(update), 0644); err != nil {
		t.Fatal(err)
	}

	// redeploy via the remote path
	if err := newCmd(t, "deploy", "--remote", "--builder=pack", fmt.Sprintf("--registry=%s", Registry)).Run(); err != nil {
		t.Fatal(err)
	}
	if !waitFor(t, ksvcUrl(name),
		withContentMatch("UPDATED"),
		withWaitTimeout(5*time.Minute)) {
		t.Fatal("function did not update correctly via remote redeploy")
	}
}
