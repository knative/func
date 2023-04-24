//go:build !integration
// +build !integration

package functions_test

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	fn "knative.dev/func/pkg/functions"
	"knative.dev/func/pkg/oci"
	. "knative.dev/func/pkg/testing"
)

// TestRunner ensures that the default internal runner correctly executes
// a scaffolded function.
func TestRunner(t *testing.T) {
	// This integration test explicitly requires the "host" builder due to its
	// lack of a dependency on a container runtime, and the other builders not
	// taking advantage of Scaffolding (expected by this runner).
	// See E2E tests for testing of running functions built using Pack or S2I and
	// which are dependent on Podman or Docker.
	// Currently only a Go function is tested because other runtimes do not yet
	// have scaffolding.

	root, cleanup := Mktemp(t)
	defer cleanup()
	ctx, cancel := context.WithCancel(context.Background())
	client := fn.New(fn.WithBuilder(oci.NewBuilder("", true)), fn.WithVerbose(true))

	// Initialize
	f, err := client.Init(fn.Function{Root: root, Runtime: "go", Registry: TestRegistry})
	if err != nil {
		t.Fatal(err)
	}

	// Build
	if f, err = client.Build(ctx, f); err != nil {
		t.Fatal(err)
	}

	// Run
	job, err := client.Run(ctx, f)
	if err != nil {
		t.Fatal(err)
	}

	// Invoke
	resp, err := http.Get(fmt.Sprintf("http://%s:%s", job.Host, job.Port))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Fatalf("unexpected response code: %v", resp.StatusCode)
	}

	cancel()
}
