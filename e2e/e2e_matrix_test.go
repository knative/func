//go:build e2e
// +build e2e

package e2e

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

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
// ---------------------------------------------------------------------------

// TestMatrix_Run ensures that supported runtimes and builders can run both
// builtin templates locally.
func TestMatrix_Run(t *testing.T) {
	forEachPermutation(t, "run", func(t *testing.T, name, runtime, builder, template string) {
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

		// Ensure the Function comes up

		if !waitFor(t, "http://"+address,
			withWaitTimeout(timeout),
			withTemplate(template)) {
			t.Fatal("service does not appear to have started correctly.")
		}

		// ^C the running function
		if err := cmd.Process.Signal(os.Interrupt); err != nil {
			fmt.Fprintf(os.Stderr, "error interrupting. %v", err)
		}

		// Wait for exit and error if anything other than 130 (^C/interrupt)
		if err := cmd.Wait(); isAbnormalExit(t, err) {
			t.Fatalf("function exited abnormally %v", err)
		}
	})
}

// TestMatrix_Deploy ensures that supported runtimes and builders can deploy
// builtin templates successfully.
func TestMatrix_Deploy(t *testing.T) {
	forEachPermutation(t, "deploy", func(t *testing.T, name, runtime, builder, template string) {
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

		// Ensure the Function comes up
		if !waitFor(t, fmt.Sprintf("http://%v.%s.%s", name, Namespace, Domain),
			withWaitTimeout(timeout),
			withTemplate(template)) {
			t.Fatal("function did not deploy correctly")
		}
	})
}

// TestMatrix_Remote ensures that supported runtimes and builders can deploy
// builtin templates remotely.
func TestMatrix_Remote(t *testing.T) {
	forEachPermutation(t, "remote", func(t *testing.T, name, runtime, builder, template string) {
		_ = fromCleanEnv(t, name)

		// Register cleanup functions (runs in LIFO order - image cleanup will
		// run after cluster cleanup)
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
		if err := newCmd(t, "deploy", "--builder", builder, "--remote", fmt.Sprintf("--registry=%s", ClusterRegistry)).Run(); err != nil {
			t.Fatal(err)
		}

		// Ensure the Function comes up
		if !waitFor(t, fmt.Sprintf("http://%v.%s.%s", name, Namespace, Domain),
			withWaitTimeout(timeout),
			withTemplate(template)) {
			t.Fatal("function did not deploy correctly")
		}
	})
}

// forEachPermutation of runtime, builder, and template, run the given test.
func forEachPermutation(t *testing.T, group string, do func(t *testing.T, name, runtime, builder, template string)) {
	t.Helper()
	if !Matrix {
		t.Skip("Matrix tests not enabled. Enable with FUNC_E2E_MATRIX=true")
	}
	for _, runtime := range MatrixRuntimes {
		for _, builder := range MatrixBuilders {
			for _, template := range MatrixTemplates {
				name := fmt.Sprintf("func-e2e-matrix-%s-%s-%s-%s", group, runtime, builder, template)
				fn := func(t *testing.T) { do(t, name, runtime, builder, template) }
				t.Run(name, fn)
			}
		}
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
