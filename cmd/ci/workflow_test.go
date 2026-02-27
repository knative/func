package ci_test

import (
	"bytes"
	"strings"
	"testing"

	"gotest.tools/v3/assert"
	"knative.dev/func/cmd/ci"
	"knative.dev/func/cmd/common"
)

func TestGitHubWorkflow_Export(t *testing.T) {
	// GIVEN
	cfg, _ := ci.NewCIConfig(
		common.CurrentBranchStub("", nil),
		common.WorkDirStub("", nil),
		false,
	)
	gw := ci.NewGitHubWorkflow(cfg, "", &bytes.Buffer{})
	bufferWriter := ci.NewBufferWriter()

	// WHEN
	exportErr := gw.Export("path", bufferWriter, true, &bytes.Buffer{})

	// THEN
	assert.NilError(t, exportErr, "unexpected error when exporting GitHub Workflow")
	assert.Assert(t, strings.Contains(bufferWriter.Buffer.String(), gw.Name))
}
