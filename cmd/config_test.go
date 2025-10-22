package cmd_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"testing"

	"github.com/ory/viper"
	"github.com/spf13/cobra"
	fnCmd "knative.dev/func/cmd"
	"knative.dev/func/cmd/ci"
	"knative.dev/func/cmd/common"
	fn "knative.dev/func/pkg/functions"
)

func TestListEnvs(t *testing.T) {
	mock := common.NewMockLoaderSaver()
	foo := "foo"
	bar := "bar"
	envs := []fn.Env{{Name: &foo, Value: &bar}}
	mock.LoadFn = func(path string) (fn.Function, error) {
		if path != "<path>" {
			t.Fatalf("bad path, got %q but expected <path>", path)
		}
		return fn.Function{Run: fn.RunSpec{Envs: envs}}, nil
	}

	cmd := setupConfigEnvCmd(mock, "-o=json", "--path=<path>")

	var buff bytes.Buffer
	cmd.SetOut(&buff)
	cmd.SetErr(&buff)

	err := cmd.Execute()
	if err != nil {
		t.Fatal(err)
	}

	var data []fn.Env
	err = json.Unmarshal(buff.Bytes(), &data)
	if err != nil {
		t.Fatal(err)
	}
	if !envsEqual(envs, data) {
		t.Errorf("env mismatch, expected %v but got %v", envs, data)
	}
}

func setupConfigEnvCmd(mock common.FunctionLoaderSaver, args ...string) *cobra.Command {
	cmd := fnCmd.NewConfigCmd(mock, ci.NewBufferWriter(), fnCmd.NewClient)
	cmd.SetArgs(append([]string{"envs"}, args...))
	return cmd
}

func TestListEnvAdd(t *testing.T) {
	// strings as vars so we can take address of them
	foo := "foo"
	bar := "bar"
	answer := "answer"
	fortyTwo := "42"
	configMapExpression := "{{ configMap:myMap }}"

	mock := common.NewMockLoaderSaver()
	mock.LoadFn = func(path string) (fn.Function, error) {
		return fn.Function{Run: fn.RunSpec{Envs: []fn.Env{{Name: &foo, Value: &bar}}}}, nil
	}
	var expectedEnvs []fn.Env
	mock.SaveFn = func(f fn.Function) error {
		if !envsEqual(expectedEnvs, f.Run.Envs) {
			return fmt.Errorf("unexpected envs: got %v but %v was expected", f.Run.Envs, expectedEnvs)
		}
		return nil
	}

	expectedEnvs = []fn.Env{{Name: &foo, Value: &bar}, {Name: &answer, Value: &fortyTwo}}
	cmd := setupConfigEnvCmd(mock, "add", "--name=answer", "--value=42")
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)

	err := cmd.Execute()
	if err != nil {
		t.Error(err)
	}

	viper.Reset()
	expectedEnvs = []fn.Env{{Name: &foo, Value: &bar}, {Name: nil, Value: &configMapExpression}}
	cmd = setupConfigEnvCmd(mock, "add", "--value={{ configMap:myMap }}")
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)

	err = cmd.Execute()
	if err != nil {
		t.Error(err)
	}

	viper.Reset()
	cmd = setupConfigEnvCmd(mock, "add", "--name=1", "--value=abc")
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)

	err = cmd.Execute()
	if err == nil {
		t.Error("expected variable name error but got nil")
	}
}

func envsEqual(a, b []fn.Env) bool {
	if len(a) != len(b) {
		return false
	}

	strPtrEq := func(x, y *string) bool {
		switch {
		case x == nil && y == nil:
			return true
		case x != nil && y != nil:
			return *x == *y
		default:
			return false
		}
	}

	strPtrLess := func(x, y *string) bool {
		switch {
		case x == nil && y == nil:
			return false
		case x != nil && y != nil:
			return *x < *y
		case x == nil:
			return true
		default:
			return false
		}

	}

	lessForSlice := func(s []fn.Env) func(i, j int) bool {
		return func(i, j int) bool {
			x := s[i]
			y := s[j]
			if strPtrLess(x.Name, y.Name) {
				return true
			}
			return strPtrLess(x.Value, y.Value)
		}
	}

	sort.Slice(a, lessForSlice(a))
	sort.Slice(b, lessForSlice(b))

	for i := range a {
		x := a[i]
		y := b[i]
		if !strPtrEq(x.Name, y.Name) || !strPtrEq(x.Value, y.Value) {
			return false
		}
	}
	return true
}
