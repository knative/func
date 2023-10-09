//go:build oncluster || runtime

package oncluster

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/util/rand"
	common "knative.dev/func/test/common"
)

var runtimeSupportMap = map[string][]string{
	"node":       {"pack", "s2i"},
	"go":         {"pack"},
	"rust":       {"pack"},
	"python":     {"pack", "s2i"},
	"quarkus":    {"pack", "s2i"},
	"springboot": {"pack"},
	"typescript": {"pack", "s2i"},
}

// TestRuntime will invoke a language runtime test against (by default) to all runtimes.
// The Environment Variable E2E_RUNTIMES can be used to select the languages/runtimes to be tested
func TestRuntime(t *testing.T) {

	var runtimeList = []string{}
	runtimes, present := os.LookupEnv("E2E_RUNTIMES")
	targetBuilder, _ := os.LookupEnv("FUNC_BUILDER")

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
			if targetBuilder == "" || builder == targetBuilder {
				t.Run(fmt.Sprintf("%v_%v_test", lang, builder), func(t *testing.T) {
					runtimeImpl(t, lang, builder)
				})
			}
		}
	}

}

func runtimeImpl(t *testing.T, lang string, builder string) {

	var gitProjectName = fmt.Sprintf("test-runtime-%v-%v-%v", lang, builder, rand.String(5))
	var gitProjectPath = filepath.Join(t.TempDir(), gitProjectName)
	var funcName = gitProjectName
	var funcPath = gitProjectPath

	gitServer := common.GitTestServerProvider{}
	gitServer.Init(t)
	remoteRepo := gitServer.CreateRepository(gitProjectName)
	t.Cleanup(func() {
		gitServer.DeleteRepository(gitProjectName)
	})

	knFunc := common.NewKnFuncShellCli(t)
	knFunc.Exec("create", "-l", lang, funcPath)
	t.Cleanup(func() {
		os.RemoveAll(gitProjectPath)
	})

	GitInitialCommitAndPush(t, gitProjectPath, remoteRepo.ExternalCloneURL)

	knFunc.Exec("deploy",
		"--registry", common.GetRegistry(),
		"--path", funcPath,
		"--remote",
		"--verbose",
		"--builder", builder,
		"--git-url", remoteRepo.ClusterCloneURL)

	t.Cleanup(func() {
		knFunc.Exec("delete", "-p", funcPath)
		time.Sleep(time.Second * 30)
	})

	// -- Assertions --
	result := knFunc.Exec("invoke", "-p", funcPath)
	t.Log(result)
	AssertThatTektonPipelineRunSucceed(t, funcName)

}
