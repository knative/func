package functions

import "testing"

func TestEnvsAdd(t *testing.T) {
	var envs Envs
	envs.Add("KEY", "value")

	if len(envs) != 1 || *envs[0].Name != "KEY" || *envs[0].Value != "value" {
		t.Errorf("Add failed: got %v", envs)
	}
}
