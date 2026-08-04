package functions_test

import (
	"errors"
	"net"
	"os"
	"path/filepath"
	"testing"

	fn "knative.dev/func/pkg/functions"
	. "knative.dev/func/pkg/testing"
)

// TestJob_New ensures that creating a new Job creates the expected errors if
// incomplete and the client registers the job as being available for the
// function when created.
//
// This is ver much a unit test mostly confirming implementation details, the
// more complete test is the integration test which invokes "run".  Presuming
// this works for both containerized and noncontainerized functions, the
// correctness of the Job implementation is inferred (with the possible
// exception of not cleaning up after itself, which is an implementation best
// left to unit tests here).
func TestJob_New(t *testing.T) {
	root, rm := Mktemp(t)
	defer rm()
	client := fn.New()

	// create a new function
	f, err := client.Init(fn.Function{Runtime: "go", Root: root})
	if err != nil {
		t.Fatal(err)
	}

	// Assert that an initialized function and port are required
	onStop := func() error { return nil }
	if _, err := fn.NewJob(fn.Function{}, "127.0.0.1", "8080", nil, onStop, false); err == nil {
		t.Fatal("expected NewJob to require an initialized functoin")
	}
	if _, err := fn.NewJob(f, "127.0.0.1", "", nil, onStop, false); err == nil {
		t.Fatal("expected NewJob to require a port")
	}

	// Assert creating a Job with the required arguments succeeds.
	_, err = fn.NewJob(f, "127.0.0.1", "8080", nil, onStop, false)
	if err != nil {
		t.Fatalf("creating job failed. %s", err)
	}

	// Assert that the client recognizes a job is running for the given function
	// NOTE: the Instances API will be updated to return []Instance to reflect
	// that the system supports multiple instances running simultaneously.
	_, err = client.Instances().Local(t.Context(), f)
	if err != nil {
		if errors.Is(err, fn.ErrNotRunning) {
			t.Fatalf("client does not recognize job as running. %s", err)
		} else {
			t.Fatalf("unexpected error checking client for instance's existence. %s", err)
		}
	}

}

func TestJob_NewCleansAllOrphanedDirectories(t *testing.T) {
	root, rm := Mktemp(t)
	t.Cleanup(rm)
	client := fn.New()

	f, err := client.Init(fn.Function{Runtime: "go", Root: root})
	if err != nil {
		t.Fatal(err)
	}

	listeners := make([]net.Listener, 4)
	ports := make([]string, len(listeners))
	seen := make(map[string]struct{}, len(listeners))
	for i := range listeners {
		listener, err := net.Listen("tcp", ":0")
		if err != nil {
			t.Fatal(err)
		}
		listeners[i] = listener
		t.Cleanup(func() { _ = listener.Close() })

		_, port, err := net.SplitHostPort(listener.Addr().String())
		if err != nil {
			t.Fatal(err)
		}
		if _, ok := seen[port]; ok {
			t.Fatalf("listener reused port %s", port)
		}
		seen[port] = struct{}{}
		ports[i] = port
	}

	runsDir := filepath.Join(root, fn.RunDataDir, "runs")
	for _, port := range ports[:3] {
		if err := os.MkdirAll(filepath.Join(runsDir, port), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, listener := range listeners[1:] {
		if err := listener.Close(); err != nil {
			t.Fatal(err)
		}
	}

	j, err := fn.NewJob(f, "127.0.0.1", ports[3], nil, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := j.Stop(); err != nil {
			t.Error(err)
		}
	})

	for _, port := range ports[1:3] {
		dir := filepath.Join(runsDir, port)
		if _, err := os.Stat(dir); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("stale job directory %s was not removed: %v", dir, err)
		}
	}
	if _, err := os.Stat(filepath.Join(runsDir, ports[0])); err != nil {
		t.Errorf("active job directory was removed: %v", err)
	}
	if _, err := os.Stat(j.Dir()); err != nil {
		t.Errorf("new job directory was not created: %v", err)
	}
}

// TestJob_Stop ensures that stopping a local job results in the API no longer
// recognizing it as running, and invokes the onStop function
func TestJob_Stop(t *testing.T) {
	root, rm := Mktemp(t)
	defer rm()
	client := fn.New()

	f, err := client.Init(fn.Function{Runtime: "go", Root: root})
	if err != nil {
		t.Fatal(err)
	}

	// Assert that an initialized function and port are required
	var onStopInvoked bool
	onStop := func() error { onStopInvoked = true; return nil }

	// Assert creating a Job with the required arguments succeeds.
	j, err := fn.NewJob(f, "127.0.0.1", "8080", nil, onStop, false)
	if err != nil {
		t.Fatalf("creating job failed. %s", err)
	}
	_, err = client.Instances().Local(t.Context(), f)
	if err != nil {
		if errors.Is(err, fn.ErrNotRunning) {
			t.Fatalf("client does not recognize job as running. %s", err)
		} else {
			t.Fatalf("unexpected error checking client for instance's existence. %s", err)
		}
	}
	if err := j.Stop(); err != nil {
		t.Fatal(err)
	}
	if !onStopInvoked {
		t.Fatal("the job stopped but did not invoke the onStop handler")
	}
}
