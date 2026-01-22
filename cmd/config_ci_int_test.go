package cmd_test

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/ory/viper"
	"gotest.tools/v3/assert"
	fnCmd "knative.dev/func/cmd"
	"knative.dev/func/cmd/ci"
	"knative.dev/func/cmd/common"
	cmdTest "knative.dev/func/cmd/testing"
	fn "knative.dev/func/pkg/functions"
	fnTest "knative.dev/func/pkg/testing"
)

// START: Integration Tests
// ------------------------
// No more mocking. Using real filesystem here for LoaderSaver and WorkflowWriter.
func TestNewConfigCICmd_FailsWhenNotInitialized(t *testing.T) {
	// passing empty func &fn.Function{} means no func will be initialized
	// in temp dir
	opts := defaultIntegrationOpts(t, &fn.Function{})
	_, gitInitErr := common.NewGitCliWrapper().Init(fnTest.FromTempDirectory(t), mainBranch)
	expectedErr := fn.NewErrNotInitialized(fnTest.Cwd())

	err := runConfigCiCmdIntegration(t, opts)

	assert.NilError(t, gitInitErr)
	assert.Error(t, err, expectedErr.Error())
}

func TestNewConfigCICmd_SuccessWhenInitialized(t *testing.T) {
	opts := defaultIntegrationOpts(t, nil)

	err := runConfigCiCmdIntegration(t, opts)

	assert.NilError(t, err)
}

func TestNewConfigCICmd_FailsToLoadFuncWithWrongPath(t *testing.T) {
	opts := defaultIntegrationOpts(t, nil)
	opts.args = append(opts.args, "--path=nofunc")
	var expectedErr *os.PathError

	err := runConfigCiCmdIntegration(t, opts)

	// Use os.IsNotExist for cross-platform compatibility (Linux vs Windows error messages differ)
	assert.Assert(t, errors.As(err, &expectedErr))
	assert.Assert(t, os.IsNotExist(expectedErr))
}

func TestNewConfigCICmd_SuccessfulLoadWithCorrectPath(t *testing.T) {
	f := cmdTest.CreateFuncWithGitInTempDir(t, fnName)
	opts := defaultIntegrationOpts(t, &f)
	opts.args = append(opts.args, "--path="+f.Root)

	err := runConfigCiCmdIntegration(t, opts)

	assert.NilError(t, err)
}

func TestNewConfigCICmd_CreatesGitHubWorkflowDirectory(t *testing.T) {
	opts := defaultIntegrationOpts(t, nil)

	err := runConfigCiCmdIntegration(t, opts)

	assert.NilError(t, err)
	_, statErr := os.Stat(filepath.Join(opts.withFunc.Root, ci.DefaultGitHubWorkflowDir))
	assert.NilError(t, statErr)
}

func TestNewConfigCICmd_WritesWorkflowFileToFSWithCorrectYAMLStructure(t *testing.T) {
	opts := defaultIntegrationOpts(t, nil)

	err := runConfigCiCmdIntegration(t, opts)
	file, openErr := os.Open(filepath.Join(opts.withFunc.Root, ci.DefaultGitHubWorkflowDir, ci.DefaultGitHubWorkflowFilename))
	raw, readErr := io.ReadAll(file)

	assert.NilError(t, err)
	assert.NilError(t, openErr)
	assert.NilError(t, readErr)
	assertDefaultWorkflowWithBranch(t, string(raw), mainBranch)
	file.Close()
}

// ----------------------
// END: Integration Tests

// START: Testing Framework
// ------------------------
type optsIntegration struct {
	withFunc *fn.Function
	args     []string
}

// defaultIntegrationOpts returns test options for integration tests with sensible defaults:
//   - withFunc: provided f or newly created function with git in temp directory
//   - args:     []string{"ci"}
//
// If f is nil, creates a new function with git in a temp directory.
// If f is provided, uses that function (allows custom path testing).
func defaultIntegrationOpts(t *testing.T, f *fn.Function) optsIntegration {
	t.Helper()

	aFunc := f
	if f == nil {
		fnTmp := cmdTest.CreateFuncWithGitInTempDir(t, fnName)
		aFunc = &fnTmp
	}

	return optsIntegration{
		withFunc: aFunc,
		args:     []string{"ci"},
	}
}

func runConfigCiCmdIntegration(
	t *testing.T,
	opts optsIntegration,
) error {
	t.Helper()

	// PRE-RUN PREP
	// all options for "func config ci" command
	t.Setenv(ci.ConfigCIFeatureFlag, "true")

	args := opts.args
	if len(opts.args) == 0 {
		args = []string{"ci"}
	}

	viper.Reset()

	cmd := fnCmd.NewConfigCmd(
		common.DefaultLoaderSaver,
		ci.DefaultWorkflowWriter,
		common.DefaultCurrentBranch,
		common.DefaultWorkDir,
		fnCmd.NewClient,
	)
	cmd.SetArgs(args)

	// RUN
	return cmd.Execute()
}

// ----------------------
// END: Testing Framework
