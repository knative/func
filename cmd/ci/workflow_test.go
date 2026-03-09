package ci_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/ory/viper"
	"gotest.tools/v3/assert"
	"knative.dev/func/cmd/ci"
	"knative.dev/func/cmd/common"
	fn "knative.dev/func/pkg/functions"
)

func TestGitHubWorkflow_Export(t *testing.T) {
	// GIVEN
	viper.Set("platform", "github")
	t.Cleanup(func() { viper.Reset() })
	loaderSaver := common.NewMockLoaderSaver()
	loaderSaver.LoadFn = func(path string) (fn.Function, error) {
		return fn.Function{Root: path, Runtime: "go"}, nil
	}
	bufferWriter := ci.NewBufferWriter()

	// WHEN
	cfg, configErr := ci.NewCIConfig(
		loaderSaver,
		common.CurrentBranchStub("", nil),
		common.WorkDirStub("", nil),
		false,
	)
	assert.NilError(t, configErr, "unexpected error when creating CIConfig")

	gw := ci.NewGitHubWorkflow(cfg, &bytes.Buffer{})
	exportErr := gw.Export(cfg.FnGitHubWorkflowFilepath(), bufferWriter, true, &bytes.Buffer{})

	// THEN
	assert.NilError(t, exportErr, "unexpected error when exporting GitHub Workflow")
	assert.Assert(t, strings.Contains(bufferWriter.Buffer.String(), gw.Name))
}
