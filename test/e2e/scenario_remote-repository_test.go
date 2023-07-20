//go:build e2e

package e2e

import (
	"path/filepath"
	"strings"
	"testing"

	"gotest.tools/v3/assert"
	"knative.dev/func/test/common"
	"knative.dev/func/test/testhttp"
)

// TestRemoteRepository verifies function created using an
// external template from a git repository
func TestRemoteRepository(t *testing.T) {

	var funcName = "remote-repo-function"
	var funcPath = filepath.Join(t.TempDir(), funcName)

	knFunc := common.NewKnFuncShellCli(t)
	knFunc.Exec("create",
		"--language", "go",
		"--template", "e2e",
		"--repository", "http://github.com/boson-project/test-templates.git", // TODO Make on config
		funcPath)

	knFunc.SourceDir = funcPath

	knFunc.Exec("deploy", "--builder", "pack", "--registry", common.GetRegistry())
	defer knFunc.Exec("delete")
	_, functionUrl := common.WaitForFunctionReady(t, funcName)

	_, funcResponse := testhttp.TestGet(t, functionUrl)
	assert.Assert(t, strings.Contains(funcResponse, "REMOTE_VALID"))

}
