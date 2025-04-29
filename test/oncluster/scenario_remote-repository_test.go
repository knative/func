//go:build oncluster

package oncluster

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gotest.tools/v3/assert"
	"knative.dev/func/test/common"
)

func setupRemoteRepository(t *testing.T) (reposutoryUrl string) {

	repositoryPath := filepath.Join(t.TempDir(), "repository")
	helloTemplatePath := filepath.Join(repositoryPath, "go", "testhello")

	createFolder := func(folderPath string) {
		e := os.MkdirAll(folderPath, 0755)
		if e != nil {
			t.Error(e.Error())
		}
	}
	createFile := func(path string, content string) {
		e := os.WriteFile(path, []byte(content), 0644)
		if e != nil {
			t.Error(e.Error())
		}
	}

	createFolder(helloTemplatePath)
	createFolder(filepath.Join(helloTemplatePath, "hello"))

	createFile(filepath.Join(helloTemplatePath, "go.mod"), `
module function
go 1.21
`)
	createFile(filepath.Join(helloTemplatePath, "handle.go"), `
package function

import (
	"fmt"
	"function/hello"
	"net/http"
)

func Handle(w http.ResponseWriter, r *http.Request) {
	w.Header().Add("Content-Type", "text/plain")
	fmt.Fprintf(w, hello.Hello("TEST")+"\n") // "HELLO TEST""
}
`)

	createFile(filepath.Join(helloTemplatePath, "hello", "hello.go"), `
package hello
func Hello(name string) string {
	return "HELLO " + name
}
`)
	gitServer := common.GetGitServer(t)
	remoteRepo := gitServer.CreateRepository("hello")
	t.Cleanup(func() {
		gitServer.DeleteRepository("hello")
	})

	GitInitialCommitAndPush(t, repositoryPath, remoteRepo.ExternalCloneURL)

	return remoteRepo.ExternalCloneURL
}

// TestRemoteRepository verifies function created using an
// external template from a git repository
func TestRemoteRepository(t *testing.T) {

	var funcName = "remote-repo-function"
	var funcPath = filepath.Join(t.TempDir(), funcName)

	gitRepoUrl := setupRemoteRepository(t)

	knFunc := common.NewKnFuncShellCli(t)
	knFunc.Exec("create",
		"--language", "go",
		"--template", "testhello",
		"--repository", gitRepoUrl+"#main", // enforce branch to be used
		funcPath)

	knFunc.SourceDir = funcPath

	knFunc.Exec("deploy", "--registry", common.GetRegistry(), "--remote")
	defer knFunc.Exec("delete")

	result := knFunc.Exec("invoke", "-p", funcPath)
	assert.Assert(t, strings.Contains(result.Out, "HELLO TEST"))

}
