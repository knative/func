//go:build !integration
// +build !integration

package function_test

import (
	"reflect"
	"testing"

	fn "knative.dev/kn-plugin-func"
	. "knative.dev/kn-plugin-func/testing"
)

// TestWriteIdempotency ensures that a Function can be written repeatedly
// without change.
func TestWriteIdempotency(t *testing.T) {
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
	if !reflect.DeepEqual(f1, f2) {
		t.Fatalf("function differs after reload.")
	}
}
