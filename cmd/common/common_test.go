package common_test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"gotest.tools/v3/assert"
	"knative.dev/func/cmd/common"
	cmdTest "knative.dev/func/cmd/testing"
	fn "knative.dev/func/pkg/functions"
	fnTest "knative.dev/func/pkg/testing"
)

const mainBranch = "main"

func TestDefaultLoaderSaver_SuccessfulLoad(t *testing.T) {
	existingFunc := cmdTest.CreateFuncWithGitInTempDir(t, "ls-func")

	actualFunc, err := common.DefaultLoaderSaver.Load(existingFunc.Root)

	assert.NilError(t, err)
	assert.Equal(t, existingFunc.Name, actualFunc.Name)
}

func TestDefaultLoaderSaver_GenericFuncCreateError_WhenFuncPathInvalid(t *testing.T) {
	_, err := common.DefaultLoaderSaver.Load("/non-existing-path")

	assert.ErrorContains(t, err, "failed to create new function")
}

func TestDefaultLoaderSaver_IsNotInitializedError_WhenNoFuncAtPath(t *testing.T) {
	expectedErrMsg := fn.NewErrNotInitialized(fnTest.Cwd()).Error()

	_, err := common.DefaultLoaderSaver.Load(fnTest.Cwd())

	assert.Error(t, err, expectedErrMsg)
}

func TestDefaultLoaderSaver_SuccessfulSave(t *testing.T) {
	existingFunc := cmdTest.CreateFuncWithGitInTempDir(t, "")
	name := "environment"
	value := "test"
	existingFunc.Run.Envs.Add(name, value)

	saveErr := common.DefaultLoaderSaver.Save(existingFunc)
	actualFunc, loadErr := common.DefaultLoaderSaver.Load(existingFunc.Root)

	assert.NilError(t, saveErr)
	assert.NilError(t, loadErr)
	assert.Equal(t, actualFunc.Run.Envs.Slice()[0], "environment=test")
}

func TestDefaultLoaderSaver_ForwardsSaveError(t *testing.T) {
	err := common.DefaultLoaderSaver.Save(fn.Function{})

	assert.Error(t, err, "function root path is required")
}

func TestGitCliWrapper_Init_InitializesRepo(t *testing.T) {
	tempDir := fnTest.FromTempDirectory(t)

	_, err := common.NewGitCliWrapper().Init(tempDir, mainBranch)
	_, statErr := os.Stat(filepath.Join(tempDir, ".git"))

	assert.NilError(t, err)
	assert.NilError(t, statErr)
}

func TestGitCliWrapper_Init_ErrorForNonExistentPath(t *testing.T) {
	_, err := common.NewGitCliWrapper().Init("/non-existing-path", mainBranch)

	assert.Assert(t, os.IsNotExist(err))
}

func TestGitCliWrapper_Init_ErrorForEmptyBranch(t *testing.T) {
	_, err := common.NewGitCliWrapper().Init(t.TempDir(), "")

	assert.Error(t, err, "branch cannot be empty")
}

func TestGitCliWrapper_CurrentBranch_ReturnsBranchName(t *testing.T) {
	tempDir := fnTest.FromTempDirectory(t)
	_, initErr := common.NewGitCliWrapper().Init(tempDir, mainBranch)

	actualBranch, err := common.NewGitCliWrapper().CurrentBranch(tempDir)

	assert.NilError(t, initErr)
	assert.NilError(t, err)
	assert.Assert(t, actualBranch == mainBranch)
}

func TestGitCliWrapper_CurrentBranch_ErrorForNonExistentPath(t *testing.T) {
	_, err := common.NewGitCliWrapper().CurrentBranch("/non-existing-path")

	assert.Assert(t, os.IsNotExist(err))
}

func TestGitCliWrapper_CurrentBranch_ErrorForNonGitDirectory(t *testing.T) {
	tempDir := fnTest.FromTempDirectory(t)
	expectedErrMsg := fmt.Errorf("could not detect git branch for '%s'. "+
		"Has git been initialized for this Function?", tempDir)

	_, err := common.NewGitCliWrapper().CurrentBranch(tempDir)

	assert.ErrorContains(t, err, expectedErrMsg.Error())
	assert.ErrorContains(t, err, "failed")
	assert.ErrorContains(t, err, "stderr")
}

func TestGitCliWrapper_CurrentBranch_ErrorWhenCommandNotFound(t *testing.T) {
	tempDir := fnTest.FromTempDirectory(t)
	t.Setenv("FUNC_GIT", "nonexistent-git-command")

	_, err := common.NewGitCliWrapper().CurrentBranch(tempDir)

	assert.ErrorContains(t, err, "failed")
	assert.ErrorContains(t, err, "nonexistent-git-command")
}
