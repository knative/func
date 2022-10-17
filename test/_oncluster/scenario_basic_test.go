//go:build oncluster
// +build oncluster

package oncluster

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gotest.tools/v3/assert"
	common "knative.dev/func/test/_common"
	e2e "knative.dev/func/test/_e2e"
)

// TestBasicUpload check if direct source upload works
func TestBasicUpload(t *testing.T) {

	var funcName = "test-func-basic-upload"
	var funcPath = filepath.Join(t.TempDir(), funcName)

	func() {
		knFunc := common.NewKnFuncShellCli(t)
		knFunc.Exec("create", "-l", "node", funcPath)

		// Write an `index.js` that make node func to return 'first revision'
		WriteNewSimpleIndexJS(t, funcPath, "first revision")

		// Deploy it
		knFunc.Exec("deploy",
			"-p", funcPath,
			"-r", e2e.GetRegistry(),
			"--remote",
		)
		defer knFunc.Exec("delete", "-p", funcPath)

		// Assert "first revision" is returned
		result := knFunc.Exec("invoke", "-p", funcPath)
		assert.Assert(t, strings.Contains(result.Stdout, "first revision"), "Func body does not contain 'first revision'")

		previousServiceRevision := e2e.GetCurrentServiceRevision(t, funcName)

		// Update index.js to force node func to return 'new revision'
		WriteNewSimpleIndexJS(t, funcPath, "new revision")

		// Re-Deploy Func
		knFunc.Exec("deploy",
			"-r", e2e.GetRegistry(),
			"-p", funcPath,
			"--remote")
		e2e.NewRevisionCheck(t, previousServiceRevision, funcName) // Wait New Service Revision

		// -- Assertions --
		result = knFunc.Exec("invoke", "-p", funcPath)
		assert.Assert(t, strings.Contains(result.Stdout, "new revision"), "Func body does not contain 'new revision'")
		AssertThatTektonPipelineRunSucceed(t, funcName)
	}()

	AssertThatTektonPipelineResourcesNotExists(t, funcName)
}

// TestDefault covers basic test scenario that ensure on cluster build from a "default branch" and
// code changes (new commits) will be properly built and deployed on new revision
func TestBasicGit(t *testing.T) {

	var funcName = "test-func-basic-git"
	var funcPath = filepath.Join(t.TempDir(), funcName)

	func() {
		gitServer := common.GitTestServerProvider{}
		gitServer.Init(t)
		remoteRepo := gitServer.CreateRepository(funcName)
		defer gitServer.DeleteRepository(funcName)

		knFunc := common.NewKnFuncShellCli(t)
		knFunc.Exec("create", "-l", "node", funcPath)
		defer os.RemoveAll(funcPath)

		// Write an `index.js` that make node func to return 'first revision'
		WriteNewSimpleIndexJS(t, funcPath, "first revision")

		sh := GitInitialCommitAndPush(t, funcPath, remoteRepo.ExternalCloneURL)

		// Deploy it
		knFunc.Exec("deploy",
			"-p", funcPath,
			"-r", e2e.GetRegistry(),
			"--remote",
			"--git-url", remoteRepo.ClusterCloneURL,
		)
		defer knFunc.Exec("delete", "-p", funcPath)

		// Assert "first revision" is returned
		result := knFunc.Exec("invoke", "-p", funcPath)
		assert.Assert(t, strings.Contains(result.Stdout, "first revision"), "Func body does not contain 'first revision'")

		previousServiceRevision := e2e.GetCurrentServiceRevision(t, funcName)

		// Update index.js to force node func to return 'new revision'
		WriteNewSimpleIndexJS(t, funcPath, "new revision")
		sh.Exec(`git add index.js`)
		sh.Exec(`git commit -m "revision 2"`)
		sh.Exec(`git push`)

		// Re-Deploy Func
		knFunc.Exec("deploy",
			"-r", e2e.GetRegistry(),
			"-p", funcPath,
			"--remote")
		e2e.NewRevisionCheck(t, previousServiceRevision, funcName) // Wait New Service Revision

		// -- Assertions --
		result = knFunc.Exec("invoke", "-p", funcPath)
		assert.Assert(t, strings.Contains(result.Stdout, "new revision"), "Func body does not contain 'new revision'")
		AssertThatTektonPipelineRunSucceed(t, funcName)

	}()

	AssertThatTektonPipelineResourcesNotExists(t, funcName)

}
