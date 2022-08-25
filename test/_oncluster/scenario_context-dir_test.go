//go:build oncluster
// +build oncluster

package oncluster

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gotest.tools/v3/assert"
	common "knative.dev/kn-plugin-func/test/_common"
	e2e "knative.dev/kn-plugin-func/test/_e2e"
)

// TestContextDirFunc tests the following use case:
//  - As a Developer I want my function located in a specific directory on my project, hosted on my
//    public git repository from the main branch, to get deployed on my cluster
func TestContextDirFunc(t *testing.T) {

	var gitProjectName = "test-project"
	var gitProjectPath = filepath.Join(os.TempDir(), gitProjectName)
	var funcName = "test-func-context-dir"
	var funcContextDir = filepath.Join("functions", funcName)
	var funcPath = filepath.Join(gitProjectPath, funcContextDir)

	func() {

		gitServer := common.GitTestServerProvider{}
		gitServer.Init(t)
		remoteRepo := gitServer.CreateRepository(gitProjectName)
		defer gitServer.DeleteRepository(gitProjectName)

		knFunc := common.NewKnFuncShellCli(t)
		knFunc.Exec("create", "-l", "node", funcPath)

		WriteNewSimpleIndexJS(t, funcPath, "hello dir")

		defer os.RemoveAll(gitProjectPath)

		// Initial commit to repository: git init + commit + push
		GitInitialCommitAndPush(t, gitProjectPath, remoteRepo.ExternalCloneURL)

		// Update func.yaml build as git + url + context-dir
		UpdateFuncYamlGit(t, funcPath, Git{URL: remoteRepo.ClusterCloneURL, ContextDir: funcContextDir})

		knFunc.Exec("deploy", "-r", e2e.GetRegistry(), "-p", funcPath)
		defer knFunc.Exec("delete", "-p", funcPath)

		// -- Assertions --
		result := knFunc.Exec("invoke", "-p", funcPath)
		assert.Assert(t, strings.Contains(result.Stdout, "hello dir"), "Func body does not contain 'hello dir'")
		AssertThatTektonPipelineRunSucceed(t, funcName)

	}()

	AssertThatTektonPipelineResourcesNotExists(t, funcName)
}
