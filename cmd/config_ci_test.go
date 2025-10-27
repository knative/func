package cmd_test

import (
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"
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
	result := runConfigCiGithubCmd(t, opts)

	assert.NilError(t, result.err)
}

func TestNewConfigCICmd_FailsWhenNotInitialized(t *testing.T) {
	expectedErrMsg := fn.NewErrNotInitialized(fnTest.Cwd()).Error()

	result := runConfigCiGithubCmd(t, opts{})

	assert.Error(t, result.err, expectedErrMsg)
}

func TestNewConfigCICmd_SuccessWhenInitialized(t *testing.T) {
	result := runConfigCiGithubCmd(t, opts{withFuncInTempDir: true})

	assert.NilError(t, result.err)
}

func TestNewConfigCICmd_CreatesGithubWorkflowDirectory(t *testing.T) {
	result := runConfigCiGithubCmd(t, opts{withFuncInTempDir: true})
	assert.NilError(t, result.err)

	expectedWorkflowPath := filepath.Join(result.f.Root, result.ciConfig.GithubWorkflowDir)
	_, err := os.Stat(expectedWorkflowPath)
	assert.NilError(t, err)
}

func TestNewConfigCICmd_GeneratesLocalWorkflowFile(t *testing.T) {
	result := runConfigCiGithubCmd(t, opts{withFuncInTempDir: true})
	assert.NilError(t, result.err)

	_ = assertWorkflowFileExists(t, result)
}

func TestNewConfigCICmd_WorkflowYAMLHasCorrectStructure(t *testing.T) {
	result := runConfigCiGithubCmd(t, opts{withFuncInTempDir: true})
	assert.NilError(t, result.err)

	workflowFilepath := assertWorkflowFileExists(t, result)

	var expectedWorkflow fnCmd.GithubWorkflow
	workflowAsBytes, err := os.ReadFile(workflowFilepath)
	assert.NilError(t, err)
	err = yaml.Unmarshal(workflowAsBytes, &expectedWorkflow)
	assert.NilError(t, err)
	assert.Equal(t, expectedWorkflow.Name, "Remote Build and Deploy")
	assert.Equal(t, expectedWorkflow.On.Push.Branches[0], "main")
	assert.Equal(t, expectedWorkflow.Jobs["deploy"].RunsOn, "ubuntu-latest")
	assert.Equal(t, expectedWorkflow.Jobs["deploy"].Steps[0].Name, "Checkout code")
	assert.Equal(t, expectedWorkflow.Jobs["deploy"].Steps[0].Uses, "actions/checkout@v4")
	assert.Equal(t, expectedWorkflow.Jobs["deploy"].Steps[1].Name, "Install func cli")
	assert.Equal(t, expectedWorkflow.Jobs["deploy"].Steps[1].Uses, "gauron99/knative-func-action@main")
	assert.Equal(t, expectedWorkflow.Jobs["deploy"].Steps[1].With["version"], "knative-v1.19.1")
	assert.Equal(t, expectedWorkflow.Jobs["deploy"].Steps[1].With["name"], "func")
	assert.Equal(t, expectedWorkflow.Jobs["deploy"].Steps[2].Name, "Deploy function")
	assert.Equal(t, expectedWorkflow.Jobs["deploy"].Steps[2].Run, "func deploy --remote")
}

// START: Testing Framework
// ------------------------
type opts struct {
	withFuncInTempDir bool
	args              []string // default: ci --github
}

type result struct {
	f        fn.Function
	ciConfig fnCmd.CIConfig
	err      error
}

func runConfigCiGithubCmd(
	t *testing.T,
	opts opts,
) result {
	t.Helper()

	f := fn.Function{}
	if opts.withFuncInTempDir {
		f = cmdTest.CreateFuncInTempDir(t, "github-ci-func")
	}

	args := opts.args
	if len(opts.args) == 0 {
		args = []string{"ci", "--github"}
	}

	ciConfig := fnCmd.NewDefaultCIConfig()
	cmd := fnCmd.NewConfigCmd(
		common.DefaultLoaderSaver,
		fnCmd.NewClient,
		ciConfig,
	)
	cmd.SetArgs(args)

	err := cmd.Execute()

	return result{
		f,
		ciConfig,
		err,
	}
}

func assertWorkflowFileExists(t *testing.T, result result) string {
	t.Helper()
	filepath := filepath.Join(result.f.Root, result.ciConfig.GithubWorkflowDir, result.ciConfig.GithubWorkflowFile)
	exists, _ := fnTest.FileExists(t, filepath)

	assert.Assert(t, exists, filepath+" does not exist")

	return filepath
}
