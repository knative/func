//go:build oncluster

package oncluster

import (
	"path/filepath"
	"strings"
	"testing"

	"gotest.tools/v3/assert"
	common "knative.dev/func/test/common"
)

/*
Test scenario covered here:
 - As a Developer I want my function stored on a public GitHub repo to get deployed on my cluster

Notes:
 * The function used as input for this scenario is stored in this repository at /test/oncluster/testdata/simplefunc

 * On a CI Pull Request action the branch used on the on-cluster test is the pull request reference.
   The equivalent deploy func command would look like this:

   func deploy --remote \
     --git-url https://github.com/knative/func \
     --git-dir test/oncluster/testdata/simplefunc \
     --git-branch refs/pull/1650/head

 * When not on CI,  the branch used will be "main" and the repository is https://github.com/knative/func.git

*/

func resolveGitVars() (gitRepoUrl string, gitRef string) {
	// On a GitHub Action (Pull Request) these variables will be set
	// https://docs.github.com/en/actions/learn-github-actions/variables
	gitRepo := common.GetOsEnvOrDefault("GITHUB_REPOSITORY", "knative/func")
	gitRef = common.GetOsEnvOrDefault("GITHUB_REF", "main")
	gitRepoUrl = "https://github.com/" + gitRepo + ".git"
	return
}

// TestGitHubFunc tests the following use case:
//   - As a Developer I want my function stored on a public GitHub repo to get deployed on my cluster
func TestGitHubFunc(t *testing.T) {

	var tmpDir = t.TempDir()
	var tmpRepo = "knative-func"
	var cloneDir = filepath.Join(tmpDir, tmpRepo)

	var githubRepo, githubRef = resolveGitVars()
	var funcName = "simplefunc"
	var funcContextDir = filepath.Join("test", "oncluster", "testdata", funcName)
	var funcPath = filepath.Join(cloneDir, funcContextDir)

	// -- Clone Func from GITHUB and checkout Branch
	sh := common.NewShellCmd(t, tmpDir)
	sh.ShouldFailOnError = true
	sh.ShouldDumpOnSuccess = false
	sh.Exec("git init " + tmpRepo)

	sh.SourceDir = cloneDir
	sh.Exec("git remote add origin " + githubRepo)
	sh.Exec("git fetch --recurse-submodules=yes --depth=1 origin --update-head-ok --force " + githubRef)
	sh.Exec("git checkout FETCH_HEAD")

	// -- Deploy Func
	knFunc := common.NewKnFuncShellCli(t)
	knFunc.Exec("deploy",
		"--path", funcPath,
		"--registry", common.GetRegistry(),
		"--remote",
		"--verbose",
		"--git-url", githubRepo,
		"--git-branch", githubRef,
		"--git-dir", funcContextDir,
	)
	defer knFunc.Exec("delete", "-p", funcPath)

	// -- Assertions --
	result := knFunc.Exec("invoke", "-p", funcPath)
	assert.Assert(t, strings.Contains(result.Out, "simple func"), "Func body does not contain 'simple func'")
	AssertThatTektonPipelineRunSucceed(t, funcName)

}
