//go:build oncluster
// +build oncluster

package oncluster

/*
Tests on this file covers "on cluster build" use cases:

A) I want my function hosted on my public git repository from a FEATURE BRANCH to get built deployed
b) I want my function hosted on my public git repository from a specific GIT TAG to get built and deployed
c) I want my function hosted on my public git repository from a specific COMMIT HASH to get built and deployed

*/

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/util/rand"
	fn "knative.dev/func"
	common "knative.dev/func/test/common"
	e2e "knative.dev/func/test/e2e"
)

func TestFromFeatureBranch(t *testing.T) {

	setupCodeFn := func(sh *common.TestExecCmd, funcProjectPath string, clusterCloneUrl string) {

		WriteNewSimpleIndexJS(t, funcProjectPath, "hello branch")
		sh.Exec("git checkout -b feature/branch")
		sh.Exec("git add index.js")
		sh.Exec(`git commit -m "feature branch change"`)
		sh.Exec("git push -u origin feature/branch")
		UpdateFuncGit(t, funcProjectPath, fn.Git{URL: clusterCloneUrl, Revision: "feature/branch"})

	}
	assertBodyFn := func(response string) bool {
		return strings.Contains(response, "hello branch")
	}
	GitRevisionCheck(t, "test-func-feature-branch"+rand.String(5), setupCodeFn, assertBodyFn)
}

func TestFromRevisionTag(t *testing.T) {

	setupCodeFn := func(sh *common.TestExecCmd, funcProjectPath string, clusterCloneUrl string) {

		WriteNewSimpleIndexJS(t, funcProjectPath, "hello v1")
		sh.Exec("git add index.js")
		sh.Exec(`git commit -m "version 1"`)
		sh.Exec("git push origin main")
		sh.Exec("git tag tag-v1")
		sh.Exec("git push origin tag-v1")
		WriteNewSimpleIndexJS(t, funcProjectPath, "hello v2")
		sh.Exec("git add index.js")
		sh.Exec(`git commit -m "version 2"`)
		sh.Exec("git push origin main")
		UpdateFuncGit(t, funcProjectPath, fn.Git{URL: clusterCloneUrl, Revision: "tag-v1"})

	}
	assertBodyFn := func(response string) bool {
		return strings.Contains(response, "hello v1")
	}
	GitRevisionCheck(t, "test-func-tag"+rand.String(5), setupCodeFn, assertBodyFn)
}

func TestFromCommitHash(t *testing.T) {

	setupCodeFn := func(sh *common.TestExecCmd, funcProjectPath string, clusterCloneUrl string) {

		WriteNewSimpleIndexJS(t, funcProjectPath, "hello v1")
		sh.Exec("git add index.js")
		sh.Exec(`git commit -m "version 1"`)
		sh.Exec("git push origin main")
		gitRevParse := sh.Exec("git rev-parse HEAD")
		WriteNewSimpleIndexJS(t, funcProjectPath, "hello v2")
		sh.Exec("git add index.js")
		sh.Exec(`git commit -m "version 2"`)
		sh.Exec("git push origin main")
		commitHash := strings.TrimSpace(gitRevParse.Out)
		UpdateFuncGit(t, funcProjectPath, fn.Git{URL: clusterCloneUrl, Revision: commitHash})

		t.Logf("Revision Check: commit hash resolved to [%v]", commitHash)
	}
	assertBodyFn := func(response string) bool {
		return strings.Contains(response, "hello v1")
	}
	GitRevisionCheck(t, "test-func-commit"+rand.String(5), setupCodeFn, assertBodyFn)
}

func GitRevisionCheck(
	t *testing.T,
	funcName string,
	setupCodeFn func(shell *common.TestExecCmd, funcProjectPath string, clusterCloneUrl string),
	assertBodyFn func(response string) bool) {

	var funcPath = filepath.Join(t.TempDir(), funcName)

	gitServer := common.GitTestServerProvider{}
	gitServer.Init(t)
	remoteRepo := gitServer.CreateRepository(funcName)
	defer gitServer.DeleteRepository(funcName)

	knFunc := common.NewKnFuncShellCli(t)
	knFunc.Exec("create", "-l", "node", funcPath)
	defer os.RemoveAll(funcPath)

	sh := GitInitialCommitAndPush(t, funcPath, remoteRepo.ExternalCloneURL)

	// Setup specific code
	setupCodeFn(sh, funcPath, remoteRepo.ClusterCloneURL)

	knFunc.Exec("deploy",
		"-r", e2e.GetRegistry(),
		"-p", funcPath,
		"--remote")
	defer knFunc.Exec("delete", "-p", funcPath)

	// -- Assertions --
	result := knFunc.Exec("invoke", "-p", funcPath)
	if !assertBodyFn(result.Out) {
		t.Error("Func Body does not contains expected expression")
	}
	AssertThatTektonPipelineRunSucceed(t, funcName)
}
