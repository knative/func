package e2e

import (
	"strings"
	"testing"
)

// List runs 'func list' command and performs basic test
func List(t *testing.T, knFunc *TestShellCmdRunner, project FunctionTestProject) {
	result := knFunc.Exec("list")
	if result.Error != nil {
		t.Fail()
	}
	isProjectPresent := strings.Contains(result.Stdout, project.FunctionName)
	if project.IsDeployed && !isProjectPresent {
		t.Fatal("Deployed project expected")
	}
	if !project.IsDeployed && isProjectPresent {
		t.Fatal("Project is not expected to appear in list output")
	}
}
