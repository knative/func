package ci_test

import (
	"strings"
	"testing"

	"gotest.tools/v3/assert"
	"knative.dev/func/cmd/ci"
	"knative.dev/func/cmd/common"
)

func TestGitHubWorkflow_Export(t *testing.T) {
	// GIVEN
	cfg, _ := ci.NewCIGitHubConfig(
		common.CurrentBranchStub("", nil),
		common.WorkDirStub("", nil),
	)
	gw := ci.NewGitHubWorkflow(cfg)
	bufferWriter := ci.NewBufferWriter()

	// WHEN
	exportErr := gw.Export("path", bufferWriter)

	// THEN
	assert.NilError(t, exportErr, "unexpected error when exporting GitHub Workflow")
	assert.Assert(t, strings.Contains(bufferWriter.Buffer.String(), gw.Name))
}
