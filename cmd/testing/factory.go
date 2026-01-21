package testing

import (
	"testing"

	"gotest.tools/v3/assert"
	"knative.dev/func/cmd/common"
	fn "knative.dev/func/pkg/functions"
	fnTest "knative.dev/func/pkg/testing"
)

// CreateFuncWithGitInTempDir creates and initializes a Go function and git in a
// temporary directory for testing.
func CreateFuncWithGitInTempDir(t *testing.T, fnName string) fn.Function {
	t.Helper()

	name := fnName
	if fnName == "" {
		name = "go-func"
	}

	result, err := fn.New().Init(
		fn.Function{Name: name, Runtime: "go", Root: fnTest.FromTempDirectory(t)},
	)
	assert.NilError(t, err)

	_, err = common.NewGitCliWrapper().Init(result.Root, "main")
	assert.NilError(t, err)

	return result
}
