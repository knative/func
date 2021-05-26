package e2e

import "testing"

// Create runs `func create' command for a given test project with basic validation
func Create(t *testing.T, knFunc *TestShellCmdRunner, project FunctionTestProject)  {
	result := knFunc.Exec("create", project.ProjectPath, "--runtime", project.Runtime, "--trigger", project.Trigger)
	if result.Error != nil {
		t.Fatal()
	}
}
