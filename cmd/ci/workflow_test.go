package ci_test

import (
	"testing"

	"gotest.tools/v3/assert"
	"knative.dev/func/cmd/ci"
)

func TestGithubWorkflow_PersistAndLoad(t *testing.T) {
	// GIVEN
	gw := ci.NewGithubWorkflow(ci.NewCiGithubConfig())
	targetPath := t.TempDir() + "/" + gw.Name + ".yaml"

	// WHEN
	persistErr := gw.Persist(targetPath)
	actualGw, loadErr := ci.NewGithubWorkflowFromPath(targetPath)

	// THEN
	assert.NilError(t, persistErr, "unexpected error when persisting Github Workflow")
	assert.NilError(t, loadErr, "unexpected error when loading Github Workflow")
	assert.Equal(t, actualGw.Name, gw.Name, "expected names to be equal")
}
