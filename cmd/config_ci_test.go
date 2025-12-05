package cmd_test

import (
	"fmt"
	"testing"

	"gotest.tools/v3/assert"
	fnCmd "knative.dev/func/cmd"
	"knative.dev/func/cmd/ci"
	"knative.dev/func/cmd/common"
	cmdTest "knative.dev/func/cmd/testing"
	fn "knative.dev/func/pkg/functions"
	fnTest "knative.dev/func/pkg/testing"
)

func TestNewConfigCICmd_RequiresFeatureFlag(t *testing.T) {
	expectedErrMsg := fmt.Sprintf("set %s to 'true' to use this feature", ci.ConfigCIFeatureFlag)

	result := runConfigCiCmd(t, opts{enableFeature: false})

	assert.Error(t, result.executeErr, expectedErrMsg)
}

func TestNewConfigCICmd_CISubcommandExist(t *testing.T) {
	// leave 'ci' to make this test explicitly use this subcommand
	opts := opts{withFuncInTempDir: true, enableFeature: true, args: []string{"ci"}}

	result := runConfigCiCmd(t, opts)

	assert.Error(t, result.executeErr, "not implemented")
}

func TestNewConfigCICmd_FailsWhenNotInitialized(t *testing.T) {
	expectedErrMsg := fn.NewErrNotInitialized(fnTest.Cwd()).Error()

	result := runConfigCiCmd(t, opts{withFuncInTempDir: false, enableFeature: true})

	assert.Error(t, result.executeErr, expectedErrMsg)
}

func TestNewConfigCICmd_SuccessWhenInitialized(t *testing.T) {
	result := runConfigCiCmd(t, defaultOpts())

	assert.Error(t, result.executeErr, "not implemented")
}

func TestNewConfigCICmd_FailsToLoadFuncWithWrongPath(t *testing.T) {
	opts := defaultOpts()
	opts.args = append(opts.args, "--path=nofunc")
	expectedErrMsg := "failed to create new function"

	result := runConfigCiCmd(t, opts)

	assert.ErrorContains(t, result.executeErr, expectedErrMsg)
}

func TestNewConfigCICmd_SuccessfulLoadWithCorrectPath(t *testing.T) {
	tmpDir := t.TempDir()
	opt := opts{withFuncInTempDir: false, enableFeature: true, args: []string{"ci", "--path=" + tmpDir}}
	_, initErr := fn.New().Init(
		fn.Function{Name: "github-ci-func", Runtime: "go", Root: tmpDir},
	)

	result := runConfigCiCmd(t, opt)

	assert.NilError(t, initErr)
	assert.Error(t, result.executeErr, "not implemented")
}

// START: Testing Framework
// ------------------------
type opts struct {
	withFuncInTempDir bool
	enableFeature     bool
	args              []string
}

// defaultOpts provides the most used options for tests
//   - withFuncInTempDir: true,
//   - enableFeature:     true,
//   - args:              []string{"ci"},
func defaultOpts() opts {
	return opts{
		withFuncInTempDir: true,
		enableFeature:     true,
		args:              []string{"ci"},
	}
}

type result struct {
	executeErr error
}

func runConfigCiCmd(
	t *testing.T,
	opts opts,
) result {
	t.Helper()

	// PRE-RUN PREP
	if opts.withFuncInTempDir {
		_ = cmdTest.CreateFuncInTempDir(t, "github-ci-func")
	}

	if opts.enableFeature {
		t.Setenv(ci.ConfigCIFeatureFlag, "true")
	}

	args := opts.args
	if len(opts.args) == 0 {
		args = []string{"ci"}
	}

	cmd := fnCmd.NewConfigCmd(
		common.DefaultLoaderSaver,
		fnCmd.NewClient,
	)
	cmd.SetArgs(args)

	// RUN
	err := cmd.Execute()

	return result{
		err,
	}
}
