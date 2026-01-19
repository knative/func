//go:build e2e
// +build e2e

package e2e

import (
	"fmt"
	"os"
	"os/exec"
	"testing"
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
	if err := newCmd(t, "deploy", "--remote", "--builder=pack", fmt.Sprintf("--registry=%s", ClusterRegistry)).Run(); err != nil {
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
		"--registry", ClusterRegistry,
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
	// https://github.com/knative/func/issues/3203
	cmd = exec.Command("git", "checkout", name)
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}

	// Trigger the deploy
	if err := newCmd(t, "deploy", "--remote",
		"--git-url", "https://github.com/functions-dev/func-e2e-tests",
		"--git-branch", name,
		"--registry", ClusterRegistry,
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
	// https://github.com/knative/func/issues/3203
	if err := os.Chdir(name); err != nil {
		t.Fatalf("failed to change to subdirectory %s: %v", name, err)
	}

	// Trigger the deploy
	if err := newCmd(t, "deploy", "--remote",
		"--git-url", "https://github.com/functions-dev/func-e2e-tests",
		"--git-dir", name,
		"--registry", ClusterRegistry,
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
