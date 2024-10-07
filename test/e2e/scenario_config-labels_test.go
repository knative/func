//go:build e2e && !windows

package e2e

import (
	"path/filepath"

	"gotest.tools/v3/assert"
	"knative.dev/func/test/common"

	"testing"
)

const (
	arrowDown = "\033[B"
	enter     = "\r"
)

// PrepareInteractiveCommand generates a generic func that can be used to test interactive `func config` commands with user input
func PrepareInteractiveCommand(knFunc *common.TestInteractiveCmd, args ...string) func(userInput ...string) {
	fn := knFunc.PrepareRun(args...)
	return func(userInput ...string) {
		result := fn(userInput...)
		if result.Error != nil {
			knFunc.T.Fatal(result.Error)
		}
	}
}

// ConfigLabelsAdd generate sa go function to test `func config labels add` with user input
func ConfigLabelsAdd(knFunc *common.TestInteractiveCmd, functionPath string) func(userInput ...string) {
	return PrepareInteractiveCommand(knFunc, "config", "labels", "add", "--path", functionPath)
}

// ConfigLabelsRemove generates a go function to test `func config labels remove` with user input
func ConfigLabelsRemove(knFunc *common.TestInteractiveCmd, functionPath string) func(userInput ...string) {
	return PrepareInteractiveCommand(knFunc, "config", "labels", "remove", "--path", functionPath)
}

// TestConfigLabel verifies function labels are properly set on the deployed functions.
// It uses "add" and "remove" sub commands with labels with specified value and labels value from environment variable.
// Test adds 3 labels and removes one.
func TestConfigLabel(t *testing.T) {

	// Given...
	labelKey1 := "l1"
	labelValue1 := "v1"
	labelKey2 := "l2"
	labelKey3 := "l3"
	testEnvName := "TEST_ENV"
	testEnvValue := "TEST_VALUE"

	knFunc := common.NewTestShellInteractiveCmd(t)

	// On When...
	funcName := "test-config-labels"
	funcPath := filepath.Join(t.TempDir(), funcName)

	knFunc.TestCmd.Exec("create", "--language", "go", "--template", "http", funcPath)
	knFunc.TestCmd.SourceDir = funcPath

	// Config labels add
	// Add 2 labels with specified key/value
	// Add 1 label with env
	configLabelsAdd := ConfigLabelsAdd(knFunc, funcPath)
	configLabelsAdd(enter, labelKey1, enter, labelValue1, enter)                   // Add first label with specified key/value
	configLabelsAdd(enter, enter, labelKey2, enter, "any", enter)                  // Add second label with specified key/value
	configLabelsAdd(enter, arrowDown, enter, labelKey3, enter, testEnvName, enter) // Add third label using value from local environment variable

	// Delete second label
	configLabelsRemove := ConfigLabelsRemove(knFunc, funcPath)
	configLabelsRemove(arrowDown, enter)

	// Deploy
	knFunc.TestCmd.
		WithEnv(testEnvName, testEnvValue).
		Exec("deploy", "--registry", common.GetRegistry())
	defer knFunc.TestCmd.Exec("delete")

	// Then assert that...
	// label1 exists and matches value2
	// label2 does not exists
	// label3 exists and matches value3
	resource := common.RetrieveKnativeServiceResource(t, funcName)
	metadataMap := resource.UnstructuredContent()["metadata"].(map[string]interface{})
	labelsMap := metadataMap["labels"].(map[string]interface{})

	assert.Assert(t, labelsMap[labelKey1] == labelValue1, "Expected label with name %v and value %v not found", labelKey1, labelValue1)
	assert.Assert(t, labelsMap[labelKey2] == nil, "Unexpected label with name %v", labelKey2)
	assert.Assert(t, labelsMap[labelKey3] == testEnvValue, "Expected label with name %v and value %v not found", labelKey3, testEnvValue)
}
