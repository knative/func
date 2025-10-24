package cmd_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"gotest.tools/v3/assert"
	fnCmd "knative.dev/func/cmd"
	"knative.dev/func/cmd/common"
	cmdTest "knative.dev/func/cmd/testing"
	fn "knative.dev/func/pkg/functions"
	fnTest "knative.dev/func/pkg/testing"
)

func TestNewConfigCICmd_CISubcommandAndGithubOptionExist(t *testing.T) {
	// leave 'ci --github' to make this test explicitly use this subcommand
	opts := opts{withFuncInTempDir: true, args: []string{"ci", "--github"}}
	cmd, _ := setupConfigCmd(t, opts)

	executeSuccess(t, cmd)
}

func TestNewConfigCICmd_FailsWhenNotInitialized(t *testing.T) {
	expectedErrMsg := fn.NewErrNotInitialized(fnTest.Cwd()).Error()
	cmd, _ := setupConfigCmd(t, opts{})

	err := cmd.Execute()

	assert.Error(t, err, expectedErrMsg)
}

func TestNewConfigCICmd_SuccessWhenInitialized(t *testing.T) {
	cmd, _ := setupConfigCmd(t, opts{withFuncInTempDir: true})

	executeSuccess(t, cmd)
}

func TestNewConfigCICmd_CreatesGithubWorkflowDirectory(t *testing.T) {
	cmd, ta := setupConfigCmd(t, opts{withFuncInTempDir: true})
	expectedWorkflowPath := filepath.Join(ta.f.Root, ta.ciConfig.GithubWorkflowDir)

	executeSuccess(t, cmd)

	_, err := os.Stat(expectedWorkflowPath)
	assert.NilError(t, err)
}

func TestNewConfigCICmd_GeneratesLocalWorkflowFile(t *testing.T) {
	cmd, ta := setupConfigCmd(t, opts{withFuncInTempDir: true})
	expectedWorkflowPath := filepath.Join(ta.f.Root, ta.ciConfig.GithubWorkflowDir)
	expectedWorkflowFile := filepath.Join(expectedWorkflowPath, ta.ciConfig.GithubWorkflowFile)

	executeSuccess(t, cmd)

	_, err := os.Stat(expectedWorkflowPath)
	assert.NilError(t, err)

	_, err = os.Stat(expectedWorkflowFile)
	assert.NilError(t, err)
}

// START: Testing Framework
// ------------------------
type opts struct {
	withFuncInTempDir bool
	args              []string // default: ci --github
}

type testArtifacts struct {
	f        fn.Function
	ciConfig fnCmd.CIConfig
}

func setupConfigCmd(
	t *testing.T,
	opts opts,
) (*cobra.Command, testArtifacts) {
	t.Helper()

	ta := testArtifacts{
		fn.Function{},
		fnCmd.NewDefaultCIConfig(),
	}

	if opts.withFuncInTempDir {
		ta.f = cmdTest.CreateFuncInTempDir(t, "github-ci-func")
	}

	args := opts.args
	if len(opts.args) == 0 {
		args = []string{"ci", "--github"}
	}

	result := fnCmd.NewConfigCmd(
		common.DefaultLoaderSaver,
		fnCmd.NewClient,
		ta.ciConfig,
	)
	result.SetArgs(args)

	return result, ta
}

func executeSuccess(t *testing.T, cmd *cobra.Command) {
	t.Helper()

	err := cmd.Execute()
	assert.NilError(t, err)
}
