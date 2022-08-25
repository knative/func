//go:build oncluster
// +build oncluster

package oncluster

import (
	"os"
	"path/filepath"
	"testing"

	common "knative.dev/kn-plugin-func/test/_common"
	e2e "knative.dev/kn-plugin-func/test/_e2e"
)

// TestFromCliBuildLocal tests the scenario which func.yaml indicates that builds should be on cluster
// but users wants to run a local build on its machine
func TestFromCliBuildLocal(t *testing.T) {

	var funcName = "test-func-cli-local"
	var funcPath = filepath.Join(os.TempDir(), funcName)

	knFunc := common.NewKnFuncShellCli(t)
	knFunc.Exec("create", "-l", "node", funcPath)
	defer os.RemoveAll(funcPath)

	// Update func.yaml build as local + some fake url (it should not call it anyway)
	UpdateFuncYamlGit(t, funcPath, Git{URL: "http://fake-repo/repo.git"})

	knFunc.Exec("deploy", "-r", e2e.GetRegistry(), "-p", funcPath, "--build", "local")
	defer knFunc.Exec("delete", "-p", funcPath)

	// -- Assertions --
	knFunc.Exec("invoke", "-p", funcPath)
	AssertThatTektonPipelineResourcesNotExists(t, funcName)

}
