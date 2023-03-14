package cmd

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	fn "knative.dev/func/pkg/functions"
)

// TestVolumes_DefaultEmpty ensures that the default is a list which responds
// correctly when no volumes are specified, in both default and json output
func TestVolumes_DefaultEmpty(t *testing.T) {
	root := fromTempDirectory(t)
	if err := fn.New().Init(fn.Function{Runtime: "go", Root: root}); err != nil {
		t.Fatal(err)
	}

	// Empty list, default output (human legible)
	cmd := NewVolumesCmd(NewTestClient())
	cmd.SetArgs([]string{})
	buff := bytes.Buffer{}
	cmd.SetOut(&buff)
	cmd.SetErr(&buff)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	out := strings.TrimSpace(buff.String())
	if out != "No volumes" {
		t.Fatalf("Unexpected result from an empty volumes list:\n%v\n", out)
	}

	// Empty list, json output
	cmd = NewVolumesCmd(NewTestClient())
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

// TestVolumes_DefaultPopulated ensures that volumes on a function are
// listed in both default and json formats.
func TestVolumes_DefaultPopulated(t *testing.T) {
	root := fromTempDirectory(t)

	secret := "secret"
	path := "path"
	volumes := []fn.Volume{{Secret: &secret, Path: &path}} // TODO: pointers unnecessary
	if err := fn.New().Init(fn.Function{Runtime: "go", Root: root, Run: fn.RunSpec{Volumes: volumes}}); err != nil {
		t.Fatal(err)
	}

	// Populated list, default formatting
	cmd := NewVolumesCmd(NewTestClient())
	cmd.SetArgs([]string{})
	buff := bytes.Buffer{}
	cmd.SetOut(&buff)
	cmd.SetErr(&buff)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	out := strings.TrimSpace(buff.String())
	if out != "Volumes:\n  Secret \"secret\" mounted at path: \"path\"" {
		t.Fatalf("Unexpected volumes list:\n%v\n", out)
	}

	// Populated list, json output
	cmd = NewVolumesCmd(NewTestClient())
	cmd.SetArgs([]string{"-o=json"})
	buff = bytes.Buffer{}
	cmd.SetOut(&buff)
	cmd.SetErr(&buff)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	var data []fn.Volume
	bb := buff.Bytes()
	if err := json.Unmarshal(bb, &data); err != nil {
		t.Fatal(err) // fail if not proper json
	}
	if !reflect.DeepEqual(data, volumes) {
		t.Fatalf("expected output: %s got: %s\n", volumes, data)
	}
}

// TestVolumes_Add ensures adding volumes works as expected.
func TestVolumes_Add(t *testing.T) {
	root := fromTempDirectory(t)

	if err := fn.New().Init(fn.Function{Runtime: "go", Root: root}); err != nil {
		t.Fatal(err)
	}

	// Add a configMap
	cmd := NewVolumesCmd(NewTestClient())
	cmd.SetArgs([]string{"add", "--configmap=cm", "--mount=/cm"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	f, err := fn.NewFunction(root)
	if err != nil {
		t.Fatal(err)
	}
	key := "cm" // TODO: pointers unnecessary
	mount := "/cm"
	volumes := []fn.Volume{{ConfigMap: &key, Path: &mount}}
	if !reflect.DeepEqual(f.Run.Volumes, volumes) {
		t.Fatalf("unexpected volumes: %v\n", f.Run.Volumes)
	}

	// Add a secret
	cmd = NewVolumesCmd(NewTestClient())
	cmd.SetArgs([]string{"add", "--secret=secret", "--mount=/secret"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	f, err = fn.NewFunction(root)
	if err != nil {
		t.Fatal(err)
	}
	key2 := "secret"
	mount2 := "/secret"
	volumes = []fn.Volume{
		{ConfigMap: &key, Path: &mount},
		{Secret: &key2, Path: &mount2},
	}
	if !reflect.DeepEqual(f.Run.Volumes, volumes) {
		t.Fatalf("unexpected volumes: %v\n", f.Run.Volumes)
	}

	// TODO: check errors: both --secret and --configmap specified, or
	// invalid value formats
}

// TODO: TestVolumes_Remove

// TODO: TestVolumes_Interactive
