//go:build e2e

package e2e

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// PYTHON MATRIX SCENARIOS
// Language-specific cases that belong on E2E - Runtimes (python), not Core.
// Named TestMatrix_Python_* so they ride make test-e2e-matrix / -run TestMatrix_
// and can be subset with -run TestMatrix_Python.
// ---------------------------------------------------------------------------

// TestMatrix_Python_StaticSignature ensures backward compatibility with the
// static (non-instanced) Python function signature. Python functions can
// export either a `new()` constructor (instanced) or a plain `handle()`
// function (static). The scaffolding imports `new` first and falls back
// to `handle` on ImportError.
func TestMatrix_Python_StaticSignature(t *testing.T) {
	requireMatrixRuntime(t, "python")

	name := "func-e2e-matrix-py-static"
	root := fromCleanEnv(t, name)

	if err := newCmd(t, "init", "-l=python", "-t=http").Run(); err != nil {
		t.Fatal(err)
	}

	staticPy := `async def handle(scope, receive, send):
    await send({
        'type': 'http.response.start',
        'status': 200,
        'headers': [[b'content-type', b'text/plain']],
    })
    await send({
        'type': 'http.response.body',
        'body': b'OK',
    })
`
	if err := os.WriteFile(filepath.Join(root, "function", "func.py"), []byte(staticPy), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "function", "__init__.py"), []byte("from .func import handle\n"), 0644); err != nil {
		t.Fatal(err)
	}

	if err := newCmd(t, "deploy", "--builder", "host").Run(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		clean(t, name, Namespace)
	}()

	if !waitFor(t, ksvcUrl(name)) {
		t.Fatal("static Python function did not deploy correctly")
	}
}

// TestMatrix_Python_LegacyParliament: old parliament functions (func.py
// def main(context) + parliament Procfile) must build and serve via pack
// and s2i, built locally and in-cluster.
func TestMatrix_Python_LegacyParliament(t *testing.T) {
	requireMatrixRuntime(t, "python")

	// The fixtures in testdata/legacy-parliament were generated with func
	// v1.17.0 (the last parliament-era release):
	//
	//	func create -l python -t http parliament-http
	//	func create -l python -t cloudevents parliament-cloudevents
	//
	// The cells pair builders, flavors and localities so that each builder
	// builds both locally and remotely (in-cluster, where scaffolding runs
	// inside the Tekton pipeline) and each flavor passes through both builders.
	for _, tc := range []struct {
		builder  string
		template string
		remote   bool
	}{
		{"pack", "http", false},
		{"s2i", "cloudevents", false},
		{"pack", "cloudevents", true},
		{"s2i", "http", true},
	} {
		cell := tc.builder
		if tc.remote {
			cell += "-remote"
		}
		t.Run(cell, func(t *testing.T) {
			name := "func-e2e-parliament-" + cell
			root := fromCleanEnv(t, name)

			// Copy in the v1.17.0-generated fixture, renaming it for this run;
			// the rest of the era func.yaml stays as generated.
			fixture := "parliament-" + tc.template
			if err := os.CopyFS(root, os.DirFS(filepath.Join(Testdata, "legacy-parliament", fixture))); err != nil {
				t.Fatal(err)
			}
			yamlPath := filepath.Join(root, "func.yaml")
			y, err := os.ReadFile(yamlPath)
			if err != nil {
				t.Fatal(err)
			}
			y = bytes.Replace(y, []byte("name: "+fixture), []byte("name: "+name), 1)
			if err := os.WriteFile(yamlPath, y, 0644); err != nil {
				t.Fatal(err)
			}

			args := []string{"deploy", "--builder", tc.builder}
			if tc.remote {
				args = append(args, "--remote")
			}
			if err := newCmd(t, args...).Run(); err != nil {
				t.Fatal(err)
			}
			defer func() {
				clean(t, name, Namespace)
			}()

			if tc.template == "cloudevents" {
				if !waitFor(t, ksvcUrl(name), withTemplate("cloudevents")) {
					t.Fatalf("legacy parliament cloudevents function did not deploy correctly with %s builder", tc.builder)
				}
			} else {
				// The parliament http template echoes the query as JSON.
				if !waitFor(t, ksvcUrl(name)+"?foo=bar", withContentMatch(`{"foo": "bar"}`)) {
					t.Fatalf("legacy parliament http function did not deploy correctly with %s builder", tc.builder)
				}
			}
		})
	}
}

// TestMatrix_Python_Update verifies that redeploying a Python function after
// changing its source code actually serves the new code.
// Regression test for issue #3079.
func TestMatrix_Python_Update(t *testing.T) {
	requireMatrixRuntime(t, "python")

	name := "func-e2e-test-python-update"
	root := fromCleanEnv(t, name)

	// create
	if err := newCmd(t, "init", "-l=python", "-t=http").Run(); err != nil {
		t.Fatal(err)
	}

	// deploy
	if err := newCmd(t, "deploy", "--builder", "pack").Run(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		clean(t, name, Namespace)
	}()
	if !waitFor(t, ksvcUrl(name),
		withWaitTimeout(5*time.Minute)) {
		t.Fatal("function did not deploy correctly")
	}

	// update: rewrite func.py with a new response body
	updated := `import logging

def new():
    return Function()

class Function:
    async def handle(self, scope, receive, send):
        await send({
            'type': 'http.response.start',
            'status': 200,
            'headers': [
                [b'content-type', b'text/plain'],
            ],
        })
        await send({
            'type': 'http.response.body',
            'body': b'UPDATED',
        })
`
	err := os.WriteFile(filepath.Join(root, "function", "func.py"), []byte(updated), 0644)
	if err != nil {
		t.Fatal(err)
	}

	// redeploy
	if err := newCmd(t, "deploy", "--builder", "pack").Run(); err != nil {
		t.Fatal(err)
	}
	if !waitFor(t, ksvcUrl(name),
		withWaitTimeout(5*time.Minute),
		withContentMatch("UPDATED")) {
		t.Fatal("function did not update correctly (issue #3079: poetry-venv cache not invalidated on source change)")
	}
}

// TestMatrix_Python_InstancedHTTP deploys a Python HTTP function using the
// instanced signature and verifies constructor state persists across requests.
func TestMatrix_Python_InstancedHTTP(t *testing.T) {
	requireMatrixRuntime(t, "python")

	name := "func-e2e-instanced-py-http"
	root := fromCleanEnv(t, name)

	if err := newCmd(t, "init", "-l=python", "-t=http").Run(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "function", "func.py"), []byte(pythonHTTPInstancedSource), 0644); err != nil {
		t.Fatal(err)
	}

	if err := newCmd(t, "deploy", "--builder", "host").Run(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		clean(t, name, Namespace)
	}()

	if !waitFor(t, ksvcUrl(name), withContentMatch("request:")) {
		t.Fatal("instanced HTTP function did not become ready")
	}

	first := requestCount(t, ksvcUrl(name))
	second := requestCount(t, ksvcUrl(name))
	if second <= first {
		t.Fatalf("instanced counter did not increase across requests (state should persist): first=%d second=%d", first, second)
	}
}

// TestMatrix_Python_UserDepsRun verifies that user code and its local
// dependencies survive the scaffolding process during a local pack build.
// The test template includes a local mylib package inside function/ that
// func.py imports.
func TestMatrix_Python_UserDepsRun(t *testing.T) {
	requireMatrixRuntime(t, "python")

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

// TestMatrix_Python_UserDepsRemote verifies that user code and its local
// dependencies survive a remote (Tekton) build.
func TestMatrix_Python_UserDepsRemote(t *testing.T) {
	requireMatrixRuntime(t, "python")

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
		fmt.Sprintf("--registry=%s", Registry)).Run(); err != nil {
		t.Fatal(err)
	}

	if !waitFor(t, ksvcUrl(name),
		withWaitTimeout(5*time.Minute),
		withContentMatch("hello from mylib")) {
		t.Fatal("function did not return mylib greeting — user code not preserved in remote build")
	}
}
