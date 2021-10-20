//go:build e2e
// +build e2e

package e2e

import (
	"os"
	"path/filepath"
	"testing"
)

// TestRemoteRepository verifies function created using an
// external template from a git repository
func TestRemoteRepository(t *testing.T) {

	knFunc := NewKnFuncShellCli(t)

	project := FunctionTestProject{}
	project.Runtime = "go"
	project.Template = "e2e"
	project.FunctionName = "func-remote-repo"
	project.ProjectPath = filepath.Join(os.TempDir(), project.FunctionName)

	result := knFunc.Exec("create", project.ProjectPath,
		"--language", project.Runtime,
		"--template", project.Template,
		"--repository", testTemplateRepository)
	if result.Error != nil {
		t.Fatal()
	}
	defer project.RemoveProjectFolder()

	Build(t, knFunc, &project)
	Deploy(t, knFunc, &project)
	defer Delete(t, knFunc, &project)
	ReadyCheck(t, knFunc, project)

	functionRespValidator := FunctionHttpResponsivenessValidator{runtime: "go", targetUrl: "%v", expects: "REMOTE_VALID"}
	functionRespValidator.Validate(t, project)

}
