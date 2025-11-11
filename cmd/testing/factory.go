package testing

import (
	"testing"

	"gotest.tools/v3/assert"
	fn "knative.dev/func/pkg/functions"
	fnTest "knative.dev/func/pkg/testing"
)

// CreateFuncInTempDir creates and initializes a Go function in a temporary
// directory for testing.
func CreateFuncInTempDir(t *testing.T, fnName string) fn.Function {
	t.Helper()

	var name string
	if fnName == "" {
		name = "go-func"
	}

	result, err := fn.New().Init(fn.Function{
		Name:    name,
		Runtime: "go",
		Root:    fnTest.FromTempDirectory(t),
	})
	assert.NilError(t, err)

	return result
}
