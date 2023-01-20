package e2e

import "testing"

// Create runs `func create' command for a given test project with basic validation
func Create(t *testing.T, knFunc *TestShellCmdRunner, project FunctionTestProject) {
	var result TestShellCmdResult
	if project.RemoteRepository == "" {
		result = knFunc.Exec("create", project.ProjectPath, "--language", project.Runtime, "--template", project.Template)
	} else {
		result = knFunc.Exec("create", project.ProjectPath, "--language", project.Runtime, "--template", project.Template, "--repository", project.RemoteRepository)
	}
	if result.Error != nil {
		t.Fatal()
	}
}
