//go:build !integration
// +build !integration

package function_test

import (
	"testing"

	"github.com/google/go-cmp/cmp"

	fn "knative.dev/kn-plugin-func"
	. "knative.dev/kn-plugin-func/testing"
)

// TestFunction_WriteIdempotency ensures that a Function can be written repeatedly
// without change.
func TestFunction_WriteIdempotency(t *testing.T) {
	root, rm := Mktemp(t)
	defer rm()
	client := fn.New(fn.WithRegistry(TestRegistry))

	// Create a function
	f := fn.Function{
		Runtime: TestRuntime,
		Root:    root,
	}
	if err := client.Create(f); err != nil {
		t.Fatal(err)
	}

	// Load the function and write it again
	f1, err := fn.NewFunction(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := f1.Write(); err != nil {
		t.Fatal(err)
	}

	// Load it again and compare
	f2, err := fn.NewFunction(root)
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(f1, f2); diff != "" {
		t.Error("function differs after reload (-before, +after):", diff)
	}
}

// TestFunction_NameDefault ensures that a Function's name is defaulted to that
// which can be derived from the last part of its path.
// Creating a new Function from a path will error if there is no Function at
// that path.  Creating using the client initializes the default.
func TestFunction_NameDefault(t *testing.T) {
	// A path at which there is no Function currently
	root := "testdata/testFunctionNameDefault"
	defer Using(t, root)()
	f, err := fn.NewFunction(root)
	if err == nil {
		t.Fatal("expected error instantiating a nonexistant Function")
	}

	// Create the Function at the path
	client := fn.New(fn.WithRegistry(TestRegistry))
	f = fn.Function{
		Runtime: TestRuntime,
		Root:    root,
	}
	if err := client.Create(f); err != nil {
		t.Fatal(err)
	}

	// Load the (now extant) Function
	f, err = fn.NewFunction(root)
	if err != nil {
		t.Fatal(err)
	}

	// Verify the name was defaulted as expected
	if f.Name != "testFunctionNameDefault" {
		t.Fatalf("expected name 'testFunctionNameDefault', got '%v'", f.Name)
	}
}
