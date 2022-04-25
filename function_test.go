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

// Test_Interpolate ensures environment variable interpolation processes
// environment variables by interpolating properly formatted references to
// local environment variables, returning a final simple map structure.
// Also ensures that nil value references are interpreted as meaning the
// environment is to be disincluded from the resultant map, rather than included
// with an empty value.
// TODO: Perhaps referring to a nonexistent local env var should be treated
// as a "leave as is" (do not set) rather than "required" resulting in error?
// TODO: What use case does a nil pointer in the Env struct serve?  Add it
// explicitly here ore get rid of the nils.
func Test_Interpolate(t *testing.T) {
	defer WithEnvVar(t, "INTERPOLATE", "interpolated")()
	cases := []struct {
		Value    string
		Expected string
		Error    bool
	}{
		// Simple values are kept unchanged
		{Value: "simple value", Expected: "simple value"},
		// Properly referenced environment variables are interpolated
		{Value: "{{ env:INTERPOLATE }}", Expected: "interpolated"},
		// Other interpolation types other than "env" are left unchanged
		{Value: "{{ other:TYPE }}", Expected: "{{ other:TYPE }}", Error: false},
		// Properly formatted references to missing variables error
		{Value: "{{ env:MISSING }}", Expected: "", Error: true},
	}

	name := "NAME" // default name for all tests
	for _, c := range cases {
		t.Logf("Value: %v\n", c.Value)
		var (
			envs    = []fn.Env{{Name: &name, Value: &c.Value}} // pre-interpolated
			vv, err = fn.Interpolate(envs)                     // interpolated
			v       = vv[name]                                 // final value
		)
		if c.Error && err == nil {
			t.Fatal("expected error in Envs interpolation not received")
		}
		if v != c.Expected {
			t.Fatalf("expected env value '%v' to be interpolated as '%v', but got '%v'", c.Value, c.Expected, v)
		}
	}

	// Nil value should be treated as being disincluded from the resultant map.
	envs := []fn.Env{{Name: &name}} // has a nil *Value ptr
	vv, err := fn.Interpolate(envs)
	if err != nil {
		t.Fatal(err)
	}
	if len(vv) != 0 {
		t.Fatalf("expected envs with a nil value to not be included in interpolation result")
	}
}
