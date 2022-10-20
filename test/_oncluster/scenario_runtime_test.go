//go:build oncluster || runtime
// +build oncluster runtime

package oncluster

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	common "knative.dev/func/test/_common"
	e2e "knative.dev/func/test/_e2e"
)

var runtimeSupportMap = map[string][]string{
	"node":       {"pack", "s2i"},
	"go":         {},
	"python":     {"pack"},
	"quarkus":    {"pack", "s2i"},
	"springboot": {"pack"},
	"typescript": {"pack", "s2i"},
}

// TestRuntime will invoke a language runtime test against (by default) to all runtimes.
// The Environment Variable E2E_RUNTIMES can be used to select the languages/runtimes to be tested
func TestRuntime(t *testing.T) {

	var runtimeList = []string{}
	runtimes, present := os.LookupEnv("E2E_RUNTIMES")

	if present {
		if runtimes != "" {
			runtimeList = strings.Split(runtimes, " ")
		}
	} else {
		for k := range runtimeSupportMap {
			runtimeList = append(runtimeList, k)
		}
	}

	for _, lang := range runtimeList {
		for _, builder := range runtimeSupportMap[lang] {
			t.Run(fmt.Sprintf("%v_%v_test", lang, builder), func(t *testing.T) {
				runtimeImpl(t, lang, builder)
			})
		}
	}

}

func runtimeImpl(t *testing.T, lang string, builder string) {

	var gitProjectName = fmt.Sprintf("test-runtime-%v-%v", lang, builder)
	var gitProjectPath = filepath.Join(t.TempDir(), gitProjectName)
	var funcName = gitProjectName
	var funcPath = gitProjectPath

	gitServer := common.GitTestServerProvider{}
	gitServer.Init(t)
	remoteRepo := gitServer.CreateRepository(gitProjectName)
	defer gitServer.DeleteRepository(gitProjectName)

	knFunc := common.NewKnFuncShellCli(t)
	knFunc.Exec("create", "-l", lang, funcPath)
	defer os.RemoveAll(gitProjectPath)

	GitInitialCommitAndPush(t, gitProjectPath, remoteRepo.ExternalCloneURL)

	knFunc.Exec("deploy",
		"--registry", e2e.GetRegistry(),
		"--path", funcPath,
		"--remote",
		"--builder", builder,
		"--git-url", remoteRepo.ClusterCloneURL)

	defer knFunc.Exec("delete", "-p", funcPath)

	// -- Assertions --
	result := knFunc.Exec("invoke", "-p", funcPath)
	t.Log(result)
	AssertThatTektonPipelineRunSucceed(t, funcName)

}
