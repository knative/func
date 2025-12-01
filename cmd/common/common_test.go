package common_test

import (
	"testing"

	"gotest.tools/v3/assert"
	"knative.dev/func/cmd/common"
	cmdTest "knative.dev/func/cmd/testing"
	fn "knative.dev/func/pkg/functions"
	fnTest "knative.dev/func/pkg/testing"
)

func TestDefaultLoaderSaver_SuccessfulLoad(t *testing.T) {
	existingFunc := cmdTest.CreateFuncInTempDir(t, "ls-func")

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
	existingFunc := cmdTest.CreateFuncInTempDir(t, "")
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
