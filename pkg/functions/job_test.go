package functions

import (
	"context"
	"errors"
	"testing"

	. "knative.dev/func/pkg/testing"
)

// TestJob_New ensures that creating a new Job creates the expected errors if
// incomplete and the client registers the job as being available for the
// function when created.
//
// This is ver much a unit test mostly confirming implementation details, the
// more complete test is the integration test which invokes "run".  Presuming
// this works for both containerized and noncontainerized funcitions, the
// correctness of the Job implementation is inferred (with the possible
// exception of not cleaning up after itself, which is an implementatoin best
// left to unit tests here).
func TestJob_New(t *testing.T) {
	root, rm := Mktemp(t)
	defer rm()
	client := New()

	// create a new function
	f, err := client.Init(Function{Runtime: "go", Root: root})
	if err != nil {
		t.Fatal(err)
	}

	// Assert that an initialized function and port are required
	onStop := func() error { return nil }
	if _, err := NewJob(Function{}, "127.0.0.1", "8080", nil, onStop, false); err == nil {
		t.Fatal("expected NewJob to require an initialized functoin")
	}
	if _, err := NewJob(f, "127.0.0.1", "", nil, onStop, false); err == nil {
		t.Fatal("expected NewJob to require a port")
	}

	// Assert creating a Job with the required arguments succeeds.
	_, err = NewJob(f, "127.0.0.1", "8080", nil, onStop, false)
	if err != nil {
		t.Fatalf("creating job failed. %s", err)
	}

	// Assert that the client recognizes a job is running for the given function
	// NOTE: the Instances API will be updated to return []Instance to reflect
	// that the system supports multiple instances running simultaneously.
	_, err = client.Instances().Local(context.Background(), f)
	if err != nil {
		if errors.Is(err, ErrNotRunning) {
			t.Fatalf("client does not recognize job as running. %s", err)
		} else {
			t.Fatalf("unexpected error checking client for instance's existence. %s", err)
		}
	}

}

// TestJob_Stop ensures that stopping a local job results in the API no longer
// recognizing it as running, and invokes the onStop function
func TestJob_Stop(t *testing.T) {
	root, rm := Mktemp(t)
	defer rm()
	client := New()

	f, err := client.Init(Function{Runtime: "go", Root: root})
	if err != nil {
		t.Fatal(err)
	}

	// Assert that an initialized function and port are required
	var onStopInvoked bool
	onStop := func() error { onStopInvoked = true; return nil }

	// Assert creating a Job with the required arguments succeeds.
	j, err := NewJob(f, "127.0.0.1", "8080", nil, onStop, false)
	if err != nil {
		t.Fatalf("creating job failed. %s", err)
	}
	_, err = client.Instances().Local(context.Background(), f)
	if err != nil {
		if errors.Is(err, ErrNotRunning) {
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
