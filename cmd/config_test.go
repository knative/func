package cmd_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"testing"

	"github.com/ory/viper"
	"github.com/spf13/cobra"
	fnCmd "knative.dev/func/cmd"
	"knative.dev/func/cmd/common"
	"knative.dev/func/pkg/ci/github"
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
	cmd := fnCmd.NewConfigCmd(
		mock,
		github.NewBufferWriter(),
		common.CurrentBranchStub("", nil),
		common.WorkDirStub("", nil),
		fnCmd.NewTestCIGeneratorFactory(&github.WorkflowGeneratorMock{}),
		fnCmd.NewClient,
	)
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

// TestEnvsAddNameWithoutValue ensures that providing --name without --value returns an error
// rather than silently writing a broken fn.Env{Name: &key, Value: nil} to func.yaml.
func TestEnvsAddNameWithoutValue(t *testing.T) {
	mock := common.NewMockLoaderSaver()
	mock.LoadFn = func(path string) (fn.Function, error) {
		return fn.Function{}, nil
	}
	mock.SaveFn = func(f fn.Function) error {
		t.Error("Save must not be called when validation fails")
		return nil
	}

	cmd := setupConfigEnvCmd(mock, "add", "--name=API_KEY")
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error when --name is provided without --value, got nil")
	}
}

func TestEnvsAdd_Duplicate(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)

	foo := "foo"
	bar := "bar"
	mock := common.NewMockLoaderSaver()
	mock.LoadFn = func(path string) (fn.Function, error) {
		return fn.Function{Run: fn.RunSpec{Envs: []fn.Env{{Name: &foo, Value: &bar}}}}, nil
	}
	mock.SaveFn = func(f fn.Function) error {
		t.Error("Save must not be called when validation fails")
		return nil
	}

	cmd := setupConfigEnvCmd(mock, "add", "--name=foo", "--value=baz")
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error when adding duplicate env name, got nil")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("expected error to contain 'already exists', got: %v", err)
	}
}

func TestEnvsAdd_ValueWithoutNameNotWholeResource(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)

	mock := common.NewMockLoaderSaver()
	mock.LoadFn = func(path string) (fn.Function, error) {
		return fn.Function{}, nil
	}
	mock.SaveFn = func(f fn.Function) error {
		t.Error("Save must not be called when validation fails")
		return nil
	}

	cmd := setupConfigEnvCmd(mock, "add", "--value=plainValue")
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error when --value is provided without --name, got nil")
	}
}

func TestEnvsRemove_Success(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)

	foo := "foo"
	bar := "bar"
	var saved fn.Function
	mock := common.NewMockLoaderSaver()
	mock.LoadFn = func(path string) (fn.Function, error) {
		return fn.Function{Run: fn.RunSpec{Envs: []fn.Env{{Name: &foo, Value: &bar}}}}, nil
	}
	mock.SaveFn = func(f fn.Function) error {
		saved = f
		return nil
	}

	cmd := setupConfigEnvCmd(mock, "remove", "--name=foo")
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(saved.Run.Envs) != 0 {
		t.Fatalf("expected 0 envs after removal, got %d", len(saved.Run.Envs))
	}
}

func TestEnvsRemove_UnnamedEnvPresent(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)

	secret := "{{ secret:mysecret }}"
	foo := "foo"
	bar := "bar"
	var saved fn.Function
	mock := common.NewMockLoaderSaver()
	mock.LoadFn = func(path string) (fn.Function, error) {
		return fn.Function{Run: fn.RunSpec{Envs: []fn.Env{
			{Value: &secret},
			{Name: &foo, Value: &bar},
		}}}, nil
	}
	mock.SaveFn = func(f fn.Function) error {
		saved = f
		return nil
	}

	cmd := setupConfigEnvCmd(mock, "remove", "--name=foo")
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(saved.Run.Envs) != 1 || *saved.Run.Envs[0].Value != secret {
		t.Fatalf("expected unnamed env to be preserved, got: %v", saved.Run.Envs)
	}
}

func TestEnvsRemove_NonExistent(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)

	foo := "foo"
	bar := "bar"
	mock := common.NewMockLoaderSaver()
	mock.LoadFn = func(path string) (fn.Function, error) {
		return fn.Function{Run: fn.RunSpec{Envs: []fn.Env{{Name: &foo, Value: &bar}}}}, nil
	}
	mock.SaveFn = func(f fn.Function) error {
		t.Error("Save must not be called when removing nonexistent env")
		return nil
	}

	cmd := setupConfigEnvCmd(mock, "remove", "--name=nonexistent")
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error when removing nonexistent env, got nil")
	}
	var envNotExist *fn.ErrEnvNotExist
	if !errors.As(err, &envNotExist) {
		t.Fatalf("expected ErrEnvNotExist, got: %v", err)
	}
}

func setupConfigLabelsCmd(mock common.FunctionLoaderSaver, args ...string) *cobra.Command {
	cmd := fnCmd.NewConfigCmd(
		mock,
		github.NewBufferWriter(),
		common.CurrentBranchStub("", nil),
		common.WorkDirStub("", nil),
		fnCmd.NewTestCIGeneratorFactory(&github.WorkflowGeneratorMock{}),
		fnCmd.NewClient,
	)
	cmd.SetArgs(append([]string{"labels"}, args...))
	return cmd
}

// TestLabelsAddNameWithoutValue ensures that providing --name without --value returns an error.
func TestLabelsAddNameWithoutValue(t *testing.T) {
	mock := common.NewMockLoaderSaver()
	mock.LoadFn = func(path string) (fn.Function, error) {
		return fn.Function{}, nil
	}
	mock.SaveFn = func(f fn.Function) error {
		t.Error("Save must not be called when validation fails")
		return nil
	}

	cmd := setupConfigLabelsCmd(mock, "add", "--name=env")
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error when --name is provided without --value, got nil")
	}
}

// TestLabelsAddValueWithoutName ensures that providing --value without --name returns an error.
func TestLabelsAddValueWithoutName(t *testing.T) {
	mock := common.NewMockLoaderSaver()
	mock.LoadFn = func(path string) (fn.Function, error) {
		return fn.Function{}, nil
	}
	mock.SaveFn = func(f fn.Function) error {
		t.Error("Save must not be called when validation fails")
		return nil
	}

	cmd := setupConfigLabelsCmd(mock, "add", "--value=prod")
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error when --value is provided without --name, got nil")
	}
}

func TestConfigLabelsAdd_Duplicate(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)

	key := "foo"
	val := "bar"
	mock := common.NewMockLoaderSaver()
	mock.LoadFn = func(path string) (fn.Function, error) {
		return fn.Function{Deploy: fn.DeploySpec{Labels: []fn.Label{{Key: &key, Value: &val}}}}, nil
	}

	cmd := setupConfigLabelsCmd(mock, "add", "--name=foo", "--value=baz")
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error when adding duplicate label key, got nil")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("expected error to contain 'already exists', got: %v", err)
	}
}

func TestConfigLabelsRemove_Success(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)

	key := "foo"
	val := "bar"
	var saved fn.Function
	mock := common.NewMockLoaderSaver()
	mock.LoadFn = func(path string) (fn.Function, error) {
		return fn.Function{Deploy: fn.DeploySpec{Labels: []fn.Label{{Key: &key, Value: &val}}}}, nil
	}
	mock.SaveFn = func(f fn.Function) error {
		saved = f
		return nil
	}

	cmd := setupConfigLabelsCmd(mock, "remove", "--name=foo")
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(saved.Deploy.Labels) != 0 {
		t.Fatalf("expected 0 labels after removal, got %d", len(saved.Deploy.Labels))
	}
}

func TestConfigLabelsRemove_NonExistent(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)

	key := "foo"
	val := "bar"
	mock := common.NewMockLoaderSaver()
	mock.LoadFn = func(path string) (fn.Function, error) {
		return fn.Function{Deploy: fn.DeploySpec{Labels: []fn.Label{{Key: &key, Value: &val}}}}, nil
	}
	mock.SaveFn = func(f fn.Function) error {
		t.Error("Save must not be called when removing nonexistent label")
		return nil
	}

	cmd := setupConfigLabelsCmd(mock, "remove", "--name=nonexistent")
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error when removing nonexistent label, got nil")
	}
	if !strings.Contains(err.Error(), "does not exist") {
		t.Fatalf("expected error to contain 'does not exist', got: %v", err)
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
