package oncluster

import (
	"testing"

	"gotest.tools/v3/assert"
)

// AssertNoError ensure err is nil otherwise fails testing
func AssertNoError(t *testing.T, err error) {
	if err != nil {
		t.Error(err.Error())
		t.FailNow()
	}
}

// AssertThatTektonPipelineRunSucceed verifies the pipeline and pipelinerun were actually created
// on the cluster and ensure all the Tasks of the pipelinerun executed successfully
// Also it logs a brief summary of execution of the pipeline for potential debug purposes
func AssertThatTektonPipelineRunSucceed(t *testing.T, functionName string) {
	assert.Assert(t, TektonPipelineExists(t, functionName), "tekton pipeline not found on cluster")
	RunSummary := TektonPipelineLastRunSummary(t, functionName)
	t.Logf("Tekton Run Summary:\n %v", RunSummary.ToString())
	assert.Assert(t, RunSummary.IsSucceed(), "expected pipeline run was not succeeded")
}

// AssertThatTektonPipelineResourcesNotExists is intended to check the pipeline and pipelinerun resources
// do not exists. This is meant to be called after a `func delete` to ensure everything is cleaned
func AssertThatTektonPipelineResourcesNotExists(t *testing.T, functionName string) {
	if !t.Failed() {
		t.Log("Checking resources got cleaned")
		assert.Assert(t, !TektonPipelineExists(t, functionName), "tekton pipeline was found but it should not exist")
		assert.Assert(t, !TektonPipelineRunExists(t, functionName), "tekton pipelinerun was found but it should not exist")
	}
}
