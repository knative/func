//go:build e2e
// +build e2e

package e2e

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestCore_PythonUserDepsRun verifies that user code and its local dependencies
// survive the scaffolding process during a local pack build. The test template
// includes a local mylib package inside function/ that func.py imports;
func TestCore_PythonUserDepsRun(t *testing.T) {
	name := "func-e2e-python-userdeps-run"
	_ = fromCleanEnv(t, name)
	t.Cleanup(func() { cleanImages(t, name) })

	// Init with testdata Python HTTP template (includes function/mylib/)
	initArgs := []string{"init", "-l", "python", "-t", "http",
		"--repository", "file://" + filepath.Join(Testdata, "templates-userdeps")}
	if err := newCmd(t, initArgs...).Run(); err != nil {
		t.Fatalf("init failed: %v", err)
	}

	// Run with pack builder
	cmd := newCmd(t, "run", "--builder", "pack", "--json")
	address := parseRunJSON(t, cmd)

	if !waitFor(t, address,
		withWaitTimeout(6*time.Minute),
		withContentMatch("hello from mylib")) {
		t.Fatal("function did not return mylib greeting — user code not preserved")
	}

	if err := cmd.Process.Signal(os.Interrupt); err != nil {
		fmt.Fprintf(os.Stderr, "error interrupting: %v", err)
	}
}

// TestCore_PythonUserDepsRemote verifies that user code and its local
// dependencies survive a remote (Tekton) build.
func TestCore_PythonUserDepsRemote(t *testing.T) {
	name := "func-e2e-python-userdeps-remote"
	_ = fromCleanEnv(t, name)
	t.Cleanup(func() { cleanImages(t, name) })
	t.Cleanup(func() { clean(t, name, Namespace) })

	// Init with testdata Python HTTP template (includes function/mylib/)
	initArgs := []string{"init", "-l", "python", "-t", "http",
		"--repository", "file://" + filepath.Join(Testdata, "templates-userdeps")}
	if err := newCmd(t, initArgs...).Run(); err != nil {
		t.Fatalf("init failed: %v", err)
	}

	// Deploy remotely via Tekton
	if err := newCmd(t, "deploy", "--builder", "pack", "--remote",
		fmt.Sprintf("--registry=%s", ClusterRegistry)).Run(); err != nil {
		t.Fatal(err)
	}

	if !waitFor(t, ksvcUrl(name),
		withWaitTimeout(5*time.Minute),
		withContentMatch("hello from mylib")) {
		t.Fatal("function did not return mylib greeting — user code not preserved in remote build")
	}
}
