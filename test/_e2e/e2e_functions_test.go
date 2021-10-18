//go:build e2elc
// +build e2elc

package e2e

import (
	"testing"
)

// Main Test flows for functions that responds to HTTP and Events for a given runtime
// specified by env var E2E_RUNTIME (default to 'go')

// TestHttpFunction verifies aspects as create, deploy, read and update
// for a function that responds to HTTP
func TestHttpFunction(t *testing.T) {

	project := NewFunctionTestProject(GetRuntime(), "http")
	knFunc := NewKnFuncShellCli(t)

	Create(t, knFunc, project)
	defer project.RemoveProjectFolder()
	Deploy(t, knFunc, &project)
	defer Delete(t, knFunc, &project)
	ReadyCheck(t, knFunc, project)
	Info(t, knFunc, &project)
	DefaultFunctionHttpTest(t, knFunc, project)
	Update(t, knFunc, &project)
	NewRevisionFunctionHttpTest(t, knFunc, project)

}

// TestCloudEventsFunction verifies aspects as create, deploy, read and update
// for a function that responds to CloudEvents
func TestCloudEventsFunction(t *testing.T) {

	project := NewFunctionTestProject(GetRuntime(), "cloudevents")
	knFunc := NewKnFuncShellCli(t)

	Create(t, knFunc, project)
	defer project.RemoveProjectFolder()
	Deploy(t, knFunc, &project)
	defer Delete(t, knFunc, &project)
	ReadyCheck(t, knFunc, project)
	Info(t, knFunc, &project)
	DefaultFunctionEventsTest(t, knFunc, project)

}
