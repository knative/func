package cmd_test

import (
	"os"
	"testing"

	"gopkg.in/yaml.v3"
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

	_, err := os.Stat(result.ciConfig.FnGithubWorkflowDir(result.f.Root))
	assert.NilError(t, err)
}

func TestNewConfigCICmd_GeneratesLocalWorkflowFile(t *testing.T) {
	result := runConfigCiGithubCmd(t, opts{withFuncInTempDir: true})
	assert.NilError(t, result.err)

	_ = assertWorkflowFileExists(t, result)
}

func TestNewConfigCICmd_WorkflowYAMLHasCorrectStructure(t *testing.T) {
	// GIVEN: -
	// WHEN
	result := runConfigCiGithubCmd(t, opts{withFuncInTempDir: true})
	assert.NilError(t, result.err)

	// THEN
	workflowFilepath := assertWorkflowFileExists(t, result)
	actualWorkflow, err := parseWorkflowYamlFromFS(workflowFilepath)
	assert.NilError(t, err)
	assert.Equal(t, actualWorkflow.Name, "Remote Build and Deploy")
	assert.Equal(t, actualWorkflow.On.Push.Branches[0], "main")
	assert.Equal(t, actualWorkflow.Jobs["deploy"].RunsOn, "ubuntu-latest")
	assert.Equal(t, actualWorkflow.Jobs["deploy"].Steps[0].Name, "Checkout code")
	assert.Equal(t, actualWorkflow.Jobs["deploy"].Steps[0].Uses, "actions/checkout@v4")
	assert.Equal(t, actualWorkflow.Jobs["deploy"].Steps[1].Name, "Install func cli")
	assert.Equal(t, actualWorkflow.Jobs["deploy"].Steps[1].Uses, "gauron99/knative-func-action@main")
	assert.Equal(t, actualWorkflow.Jobs["deploy"].Steps[1].With["version"], "knative-v1.19.1")
	assert.Equal(t, actualWorkflow.Jobs["deploy"].Steps[1].With["name"], "func")
	assert.Equal(t, actualWorkflow.Jobs["deploy"].Steps[2].Name, "Deploy function")
	assert.Equal(t, actualWorkflow.Jobs["deploy"].Steps[2].Run, "func deploy --remote")
}

func TestNewConfigCICmd_WorkflowYAMLHasCustomName(t *testing.T) {
	// GIVEN
	expectedName := "Custom workflow name"
	ciConfig := ci.NewDefaultCIConfigWithName(expectedName)
	opts := opts{
		withFuncInTempDir: true,
		ciConfig:          &ciConfig,
	}

	// WHEN
	result := runConfigCiGithubCmd(t, opts)
	assert.NilError(t, result.err)

	// THEN
	workflowFilepath := assertWorkflowFileExists(t, result)
	actualWorkflow, err := parseWorkflowYamlFromFS(workflowFilepath)
	assert.NilError(t, err)
	assert.Equal(t, actualWorkflow.Name, expectedName)
}

// START: Testing Framework
// ------------------------
type opts struct {
	withFuncInTempDir bool
	args              []string // default: ci --github
	ciConfig          *ci.CIConfig
}

type result struct {
	f        fn.Function
	ciConfig ci.CIConfig
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

	err := cmd.Execute()

	return result{
		f,
		*opts.ciConfig,
		err,
	}
}

func assertWorkflowFileExists(t *testing.T, result result) string {
	t.Helper()

	filepath := result.ciConfig.FnGithubWorkflowYamlPath(result.f.Root)
	exists, _ := fnTest.FileExists(t, filepath)

	assert.Assert(t, exists, filepath+" does not exist")

	return filepath
}

func parseWorkflowYamlFromFS(workflowFilepath string) (*ci.GithubWorkflow, error) {
	workflowAsBytes, err := os.ReadFile(workflowFilepath)
	if err != nil {
		return nil, err
	}

	var result ci.GithubWorkflow
	if err = yaml.Unmarshal(workflowAsBytes, &result); err != nil {
		return nil, err
	}

	return &result, nil
}
