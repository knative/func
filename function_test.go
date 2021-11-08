//go:build !integration
// +build !integration

package function_test

import (
	"testing"

	fn "knative.dev/kn-plugin-func"
	. "knative.dev/kn-plugin-func/testing"
)

// TestFunctionNameDefault ensures that a Function's name is defaulted to that
// which can be derived from the last part of its path.
func TestFunctionNameDefault(t *testing.T) {
	root := "testdata/testFunctionNameDefault"
	defer Using(t, root)()
	_, err := fn.NewFunction(root)
	if err != nil {
		t.Fatal(err)
	}
	// TODO
	// Test that the name was defaulted
}
