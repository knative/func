package ci_test

import (
	"strings"
	"testing"

	"gotest.tools/v3/assert"
	"knative.dev/func/cmd/ci"
)

func TestGitHubWorkflow_Export(t *testing.T) {
	// GIVEN
	gw := ci.NewGitHubWorkflow(ci.NewCIGitHubConfig())
	bufferWriter := ci.NewBufferWriter()

	// WHEN
	exportErr := gw.Export("path", bufferWriter)

	// THEN
	assert.NilError(t, exportErr, "unexpected error when exporting GitHub Workflow")
	assert.Assert(t, strings.Contains(bufferWriter.Buffer.String(), gw.Name))
}
