package cmd_test

import (
	"os"
	"testing"

	"gotest.tools/v3/assert"
	fnCmd "knative.dev/func/cmd"
	"knative.dev/func/cmd/ci"
	"knative.dev/func/cmd/common"
	cmdTest "knative.dev/func/cmd/testing"
	fn "knative.dev/func/pkg/functions"
	fnTest "knative.dev/func/pkg/testing"
)

func TestNewConfigCICmd_CISubcommandAndGithubOptionExist(t *testing.T) {
	// leave 'ci --github' to make this test explicitly use this subcommand
	options := opts{withFuncInTempDir: true, args: []string{"ci", "--github"}}

	result := runConfigCiGithubCmd(t, options)

	assert.NilError(t, result.executeErr)
}

func TestNewConfigCICmd_FailsWhenNotInitialized(t *testing.T) {
	expectedErrMsg := fn.NewErrNotInitialized(fnTest.Cwd()).Error()

	result := runConfigCiGithubCmd(t, opts{})

	assert.Error(t, result.executeErr, expectedErrMsg)
}

func TestNewConfigCICmd_SuccessWhenInitialized(t *testing.T) {
	options := opts{withFuncInTempDir: true}

	result := runConfigCiGithubCmd(t, options)

	assert.NilError(t, result.executeErr)
}

func TestNewConfigCICmd_CreatesGithubWorkflowDirectory(t *testing.T) {
	options := opts{withFuncInTempDir: true}

	result := runConfigCiGithubCmd(t, options)

	assert.NilError(t, result.executeErr)
	_, err := os.Stat(result.ciConfig.FnGithubWorkflowDir(result.f.Root))
	assert.NilError(t, err)
}

func TestNewConfigCICmd_GeneratesLocalWorkflowFile(t *testing.T) {
	options := opts{withFuncInTempDir: true}

	result := runConfigCiGithubCmd(t, options)

	assert.NilError(t, result.executeErr)
	assertWorkflowFileExists(t, result)
}

func TestNewConfigCICmd_WorkflowYAMLHasCorrectStructure(t *testing.T) {
	// GIVEN
	options := opts{withFuncInTempDir: true}

	// WHEN
	result := runConfigCiGithubCmd(t, options)

	// THEN
	assert.NilError(t, result.executeErr)
	assertWorkflowFileExists(t, result)
	assert.NilError(t, result.gwLoadError)
	assertWorkflowFileContent(t, result.gw, "Remote Build and Deploy")
}

func TestNewConfigCICmd_WorkflowYAMLHasCustomName(t *testing.T) {
	// GIVEN
	ciConfig := ci.NewDefaultCIConfigWithName("Project Sunrise")
	options := opts{
		withFuncInTempDir: true,
		ciConfig:          &ciConfig,
	}

	// WHEN
	result := runConfigCiGithubCmd(t, options)

	// THEN
	assert.NilError(t, result.executeErr)
	assertWorkflowFileExists(t, result)
	assert.NilError(t, result.gwLoadError)
	assertWorkflowFileContent(t, result.gw, ciConfig.WorkflowName())
}

// START: Testing Framework
// ------------------------
type opts struct {
	withFuncInTempDir bool
	args              []string // default: ci --github
	ciConfig          *ci.CIConfig
}

type result struct {
	f           fn.Function
	ciConfig    ci.CIConfig
	executeErr  error
	gw          *ci.GithubWorkflow
	gwLoadError error
}

func runConfigCiGithubCmd(
	t *testing.T,
	opts opts,
) result {
	t.Helper()

	// PRE-RUN PREP
	// all options for "func config ci --github" command
	f := fn.Function{}
	if opts.withFuncInTempDir {
		f = cmdTest.CreateFuncInTempDir(t, "github-ci-func")
	}

	args := opts.args
	if len(opts.args) == 0 {
		args = []string{"ci", "--github"}
	}

	if opts.ciConfig == nil {
		ciConf := ci.NewDefaultCIConfig()
		opts.ciConfig = &ciConf
	}
	cmd := fnCmd.NewConfigCmd(
		common.DefaultLoaderSaver,
		fnCmd.NewClient,
		*opts.ciConfig,
	)
	cmd.SetArgs(args)

	// RUN
	err := cmd.Execute()

	// POST-RUN GATHER
	gwPath := opts.ciConfig.FnGithubWorkflowFilepath(f.Root)
	gw, gwLoadErr := ci.NewGithubWorkflowFromPath(gwPath)

	return result{
		f,
		*opts.ciConfig,
		err,
		gw,
		gwLoadErr,
	}
}

func assertWorkflowFileExists(t *testing.T, result result) {
	t.Helper()

	filepath := result.ciConfig.FnGithubWorkflowFilepath(result.f.Root)
	exists, _ := fnTest.FileExists(t, filepath)

	assert.Assert(t, exists, filepath+" does not exist")
}

func assertWorkflowFileContent(t *testing.T, actualGw *ci.GithubWorkflow, name string) {
	t.Helper()

	assert.Equal(t, actualGw.Name, name)
	assert.Equal(t, actualGw.On.Push.Branches[0], "main")
	assert.Equal(t, actualGw.Jobs["deploy"].RunsOn, "ubuntu-latest")
	assert.Equal(t, actualGw.Jobs["deploy"].Steps[0].Name, "Checkout code")
	assert.Equal(t, actualGw.Jobs["deploy"].Steps[0].Uses, "actions/checkout@v4")
	assert.Equal(t, actualGw.Jobs["deploy"].Steps[1].Name, "Install func cli")
	assert.Equal(t, actualGw.Jobs["deploy"].Steps[1].Uses, "gauron99/knative-func-action@main")
	assert.Equal(t, actualGw.Jobs["deploy"].Steps[1].With["version"], "knative-v1.19.1")
	assert.Equal(t, actualGw.Jobs["deploy"].Steps[1].With["name"], "func")
	assert.Equal(t, actualGw.Jobs["deploy"].Steps[2].Name, "Deploy function")
	assert.Equal(t, actualGw.Jobs["deploy"].Steps[2].Run, "func deploy --remote")
}
