package functions

import (
	"os"
	"slices"
	"strings"
	"testing"

	"k8s.io/utils/ptr"
)

// TestBuildRunnerEnv_InheritsParentEnv ensures that the parent process
// environment is inherited by the subprocess.
func TestBuildRunnerEnv_InheritsParentEnv(t *testing.T) {
	const testKey = "FUNC_TEST_INHERIT_CHECK"
	const testVal = "inherited_value"
	t.Setenv(testKey, testVal)

	job := &Job{Function: Function{}}
	env, err := buildRunnerEnv(job, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := testKey + "=" + testVal
	if !slices.Contains(env, expected) {
		t.Errorf("expected parent env var %q in result, but not found", expected)
	}
}

// TestBuildRunnerEnv_ExtrasAreIncluded ensures that extras like PORT,
// LISTEN_ADDRESS, and PWD are present in the environment.
func TestBuildRunnerEnv_ExtrasAreIncluded(t *testing.T) {
	job := &Job{Function: Function{}}
	extras := map[string]string{
		"PORT":           "8080",
		"LISTEN_ADDRESS": "127.0.0.1:8080",
		"PWD":            "/tmp/func",
	}

	env, err := buildRunnerEnv(job, extras)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for k, v := range extras {
		expected := k + "=" + v
		if !slices.Contains(env, expected) {
			t.Errorf("expected extra env var %q in result, but not found", expected)
		}
	}
}

// TestBuildRunnerEnv_FuncYamlEnvsIncluded ensures that envs from func.yaml
// (Function.Run.Envs) are interpolated and included.
func TestBuildRunnerEnv_FuncYamlEnvsIncluded(t *testing.T) {
	job := &Job{
		Function: Function{
			Run: RunSpec{
				Envs: Envs{
					{Name: ptr.To("MY_VAR"), Value: ptr.To("my_value")},
					{Name: ptr.To("ANOTHER"), Value: ptr.To("another_value")},
				},
			},
		},
	}

	env, err := buildRunnerEnv(job, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !slices.Contains(env, "MY_VAR=my_value") {
		t.Error("expected MY_VAR=my_value in result")
	}
	if !slices.Contains(env, "ANOTHER=another_value") {
		t.Error("expected ANOTHER=another_value in result")
	}
}

// TestBuildRunnerEnv_FuncYamlEnvsOverrideParent ensures that func.yaml envs
// take precedence over parent environment variables (last value wins in exec).
func TestBuildRunnerEnv_FuncYamlEnvsOverrideParent(t *testing.T) {
	const testKey = "FUNC_TEST_OVERRIDE"
	t.Setenv(testKey, "parent_value")

	job := &Job{
		Function: Function{
			Run: RunSpec{
				Envs: Envs{
					{Name: ptr.To(testKey), Value: ptr.To("func_yaml_value")},
				},
			},
		},
	}

	env, err := buildRunnerEnv(job, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// The func.yaml value should appear after the parent value.
	// In exec.Cmd, the last duplicate key wins.
	lastValue := ""
	for _, e := range env {
		if v, ok := strings.CutPrefix(e, testKey+"="); ok {
			lastValue = v
		}
	}
	if lastValue != "func_yaml_value" {
		t.Errorf("expected func.yaml value to override parent, got %q", lastValue)
	}
}

// TestBuildRunnerEnv_PathInherited is a quick check that PATH specifically
// is inherited from parent env.
func TestBuildRunnerEnv_PathInherited(t *testing.T) {
	job := &Job{Function: Function{}}
	env, err := buildRunnerEnv(job, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	pathPresent := false
	for _, e := range env {
		if strings.HasPrefix(e, "PATH=") {
			pathPresent = true
			break
		}
	}

	// PATH should always be set in a normal Unix environment
	if path := os.Getenv("PATH"); path != "" && !pathPresent {
		t.Error("expected PATH to be inherited from parent environment")
	}
}

// TestBuildRunnerEnv_EmptyRunEnvs ensures no error when there are no
// func.yaml envs configured.
func TestBuildRunnerEnv_EmptyRunEnvs(t *testing.T) {
	job := &Job{Function: Function{}}
	env, err := buildRunnerEnv(job, map[string]string{"PORT": "8080"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !slices.Contains(env, "PORT=8080") {
		t.Error("expected PORT=8080 in result")
	}
}
