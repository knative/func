package e2e

import (
	"testing"
)

// Delete runs `func delete' command for a given test project and verifies it get deleted
func Delete(t *testing.T, knFunc *TestShellCmdRunner, project *FunctionTestProject) {
	// Invoke delete command
	result := knFunc.Exec("delete", project.FunctionName)
	if result.Error != nil && project.IsDeployed {
		t.Fail()
	}
	project.IsDeployed = false

	// Invoke list to verify project was deleted
	List(t, knFunc, *project)
}
