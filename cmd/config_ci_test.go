package cmd_test

import (
	"testing"

	"github.com/spf13/cobra"
	"gotest.tools/v3/assert"
	fnCmd "knative.dev/func/cmd"
	"knative.dev/func/cmd/common"
	cmdTest "knative.dev/func/cmd/testing"
	fn "knative.dev/func/pkg/functions"
	fnTest "knative.dev/func/pkg/testing"
)

func TestNewConfigCICmd_CommandExists(t *testing.T) {
	opts := ciOpts{withMockLoaderSaver: true, args: []string{"ci", "--github"}}
	cmd := configCIGithubCmd(t, opts)

	err := cmd.Execute()

	assert.NilError(t, err)
}

func TestNewConfigCICmd_FailsWhenNotInitialized(t *testing.T) {
	expectedErrMsg := fn.NewErrNotInitialized(fnTest.Cwd()).Error()
	cmd := configCIGithubCmd(t, ciOpts{})

	err := cmd.Execute()

	assert.Error(t, err, expectedErrMsg)
}

func TestNewConfigCICmd_SuccessWhenInitialized(t *testing.T) {
	cmd := configCIGithubCmd(t, ciOpts{withFuncInTempDir: true})

	err := cmd.Execute()

	assert.NilError(t, err)
}

// START: Testing Framework
// ------------------------
type ciOpts struct {
	withMockLoaderSaver bool     // default: false
	withFuncInTempDir   bool     // default: false
	args                []string // default: ci --github
}

func configCIGithubCmd(
	t *testing.T,
	opts ciOpts,
) *cobra.Command {
	t.Helper()

	if opts.withFuncInTempDir {
		_ = cmdTest.CreateFuncInTempDir(t, "github-ci-func")
	}

	var loaderSaver common.FunctionLoaderSaver = common.DefaultLoaderSaver
	if opts.withMockLoaderSaver {
		loaderSaver = newMockLoaderSaver()
	}

	args := opts.args
	if len(opts.args) == 0 {
		args = []string{"ci", "--github"}
	}

	result := fnCmd.NewConfigCmd(loaderSaver, fnCmd.NewClient)
	result.SetArgs(args)

	return result
}
