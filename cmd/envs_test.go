package cmd

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	fn "knative.dev/func/pkg/functions"
)

// TestEnvs_DefaultEmpty ensures that the default is a list which responds correctly
// when no environment variables specified on the function, in both default and
// json output.
func TestEnvs_DefaultEmpty(t *testing.T) {
	root := fromTempDirectory(t)
	if err := fn.New().Init(fn.Function{Runtime: "go", Root: root}); err != nil {
		t.Fatal(err)
	}

	// Empty list, default output (human legible)
	cmd := NewEnvsCmd(NewTestClient())
	cmd.SetArgs([]string{})
	buff := bytes.Buffer{}
	cmd.SetOut(&buff)
	cmd.SetErr(&buff)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	out := strings.TrimSpace(buff.String())
	if out != "No environment variables" {
		t.Fatalf("Unexpected result from an empty envs list:\n%v\n", out)
	}

	// Empty list, json output
	cmd = NewEnvsCmd(NewTestClient())
	cmd.SetArgs([]string{"-o=json"})
	buff = bytes.Buffer{}
	cmd.SetOut(&buff)
	cmd.SetErr(&buff)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	var data []fn.Env
	bb := buff.Bytes()
	if err := json.Unmarshal(bb, &data); err != nil {
		t.Fatal(err) // fail if not proper json
	}
	if !reflect.DeepEqual(data, []fn.Env{}) {
		t.Fatalf("unexpected data from an empty function: %s\n", bb)
	}
}

// TestEnvs_DefaultPopulated ensures that environment variables on a function are
// listed in both default and json formats.
func TestEnvs_DefaultPopulated(t *testing.T) {
	root := fromTempDirectory(t)

	name := "name" // TODO: pointers unnecessary
	value := "value"
	envs := []fn.Env{{Name: &name, Value: &value}}
	if err := fn.New().Init(fn.Function{Runtime: "go", Root: root, Run: fn.RunSpec{Envs: envs}}); err != nil {
		t.Fatal(err)
	}

	// Populated list, default formatting
	cmd := NewEnvsCmd(NewTestClient())
	cmd.SetArgs([]string{})
	buff := bytes.Buffer{}
	cmd.SetOut(&buff)
	cmd.SetErr(&buff)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	out := strings.TrimSpace(buff.String())
	if out != "Environment variables:\n  Env \"name\" with value \"value\"" {
		t.Fatalf("Unexpected envs list:\n%v\n", out)
	}

	// Populated list, json output
	cmd = NewEnvsCmd(NewTestClient())
	cmd.SetArgs([]string{"-o=json"})
	buff = bytes.Buffer{}
	cmd.SetOut(&buff)
	cmd.SetErr(&buff)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	var data []fn.Env
	bb := buff.Bytes()
	if err := json.Unmarshal(bb, &data); err != nil {
		t.Fatal(err) // fail if not proper json
	}
	if !reflect.DeepEqual(data, envs) {
		t.Fatalf("unexpected output: %s\n", bb)
	}
}

// TestEnvs_Add ensures that adding an environment variable in all available
// ways succeeds or fails as expected:
// - simple key/value succeeds
// - as a configMap value succeeds
// - with an invalid key fails
func TestEnvs_Add(t *testing.T) {
	root := fromTempDirectory(t)

	if err := fn.New().Init(fn.Function{Runtime: "go", Root: root}); err != nil {
		t.Fatal(err)
	}

	// Add a simple key/value
	cmd := NewEnvsCmd(NewTestClient())
	cmd.SetArgs([]string{"add", "--name=name", "--value=value"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	f, err := fn.NewFunction(root)
	if err != nil {
		t.Fatal(err)
	}
	name := "name"
	value := "value"
	envs := []fn.Env{{Name: &name, Value: &value}} // TODO: pointers unnecessary
	if !reflect.DeepEqual(f.Run.Envs, envs) {
		t.Fatalf("unexpected envs: %v\n", f.Run.Envs)
	}

	// Add a configMap value reference should not fail
	cmd = NewEnvsCmd(NewTestClient())
	cmd.SetArgs([]string{"add", "--value={{ configMap:myMap }}"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	f, err = fn.NewFunction(root)
	if err != nil {
		t.Fatal(err)
	}
	name = "name"
	value = "value"
	configMapValue := "{{ configMap:myMap }}"
	envs = []fn.Env{{Name: &name, Value: &value}, {Name: nil, Value: &configMapValue}} // TODO: pointers unnecessary
	if !reflect.DeepEqual(f.Run.Envs, envs) {
		t.Fatalf("unexpected envs: %v\n", f.Run.Envs)
	}

	// Add with an invalid (numeric first character) name should fail
	cmd = NewEnvsCmd(NewTestClient())
	cmd.SetArgs([]string{"add", "--name=1", "--value=value"})
	if err := cmd.Execute(); err == nil {
		t.Fatal("did not receive expected error adding env with invalid name")
	}
}

// TestEnvs_Remove ensures that removing environment variables succeeds
// TODO: Not Implemented
//  Thee is currently no way other than interactive prompts to remove
//  a declared environment variable.  Note this is a bit tricky because some
//  have no key (no name) indicating "all variables from the given reference".
//  These entries could be removed instead by specifying an exact match of.
//  their value string, or their via their index in the list.

// TODO: TestEnvs_Interactive
