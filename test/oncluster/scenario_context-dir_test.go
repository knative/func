//go:build oncluster
// +build oncluster

package oncluster

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gotest.tools/v3/assert"
	"k8s.io/apimachinery/pkg/util/rand"
	common "knative.dev/func/test/common"
	e2e "knative.dev/func/test/e2e"
)

// TestContextDirFunc tests the following use case:
//   - As a Developer I want my function located in a specific directory on my project, hosted on my
//     public git repository from the main branch, to get deployed on my cluster
func TestContextDirFunc(t *testing.T) {

	var gitProjectName = "test-project" + rand.String(5)
	var gitProjectPath = filepath.Join(t.TempDir(), gitProjectName)
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

		knFunc.Exec("deploy",
			"-p", funcPath,
			"-r", e2e.GetRegistry(),
			"--remote",
			"--git-url", remoteRepo.ClusterCloneURL,
			"--git-dir", funcContextDir,
		)
		defer knFunc.Exec("delete", "-p", funcPath)

		// -- Assertions --
		result := knFunc.Exec("invoke", "-p", funcPath)
		assert.Assert(t, strings.Contains(result.Out, "hello dir"), "Func body does not contain 'hello dir'")
		AssertThatTektonPipelineRunSucceed(t, funcName)

	}()

	AssertThatTektonPipelineResourcesNotExists(t, funcName)
}
