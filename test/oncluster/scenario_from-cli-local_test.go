//go:build oncluster

package oncluster

import (
	"os"
	"path/filepath"
	"testing"

	"k8s.io/apimachinery/pkg/util/rand"
	fn "knative.dev/func/pkg/functions"
	common "knative.dev/func/test/common"
)

// TestFromCliBuildLocal tests the scenario which func.yaml indicates that builds should be on cluster
// but users wants to run a local build on its machine
func TestFromCliBuildLocal(t *testing.T) {

	var funcName = "test-func-cli-local" + rand.String(5)
	var funcPath = filepath.Join(t.TempDir(), funcName)

	knFunc := common.NewKnFuncShellCli(t)
	knFunc.ShouldDumpOnSuccess = false
	knFunc.Exec("create", "-l", "node", funcPath)
	defer os.RemoveAll(funcPath)

	// Update func.yaml build as local + some fake url (it should not call it anyway)
	UpdateFuncGit(t, funcPath, fn.Git{URL: "http://fake-repo/repo.git"})

	knFunc.Exec("deploy",
		"-p", funcPath,
		"-r", common.GetRegistry(),
		// "--remote",  // NOTE: Intentionally omitted
	)
	defer knFunc.Exec("delete", "-p", funcPath)

	// -- Assertions --
	knFunc.Exec("invoke", "-p", funcPath)
	AssertThatTektonPipelineResourcesNotExists(t, funcName)

}
