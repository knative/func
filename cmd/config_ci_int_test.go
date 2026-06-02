package cmd_test

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/ory/viper"
	"gotest.tools/v3/assert"
	"gotest.tools/v3/assert/cmp"
	fnCmd "knative.dev/func/cmd"
	"knative.dev/func/cmd/common"
	cmdTest "knative.dev/func/cmd/testing"
	"knative.dev/func/pkg/ci/github"
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
	_, statErr := os.Stat(filepath.Join(opts.withFunc.Root, github.DefaultGitHubWorkflowDir))
	assert.NilError(t, statErr)
}

func TestNewConfigCICmd_WritesWorkflowFileToFSWithCorrectYAMLStructure(t *testing.T) {
	opts := defaultIntegrationOpts(t, nil)

	err := runConfigCiCmdIntegration(t, opts)
	raw := readWorkflowFile(t, opts.withFunc.Root)

	assert.NilError(t, err)
	assertDefaultWorkflow(t, raw)
}

func TestNewConfigCICmd_ForceFlagOverwritesExistingWorkflowOnFS(t *testing.T) {
	workflowName := "Func Deploy"
	changedWorkflowName := "Sales Service Deployment"
	baseOpts := defaultIntegrationOpts(t, nil)

	t.Run("initial workflow creation succeeds", func(t *testing.T) {
		err := runConfigCiCmdIntegration(t, baseOpts)
		content := readWorkflowFile(t, baseOpts.withFunc.Root)

		assert.NilError(t, err)
		assert.Assert(t, yamlContains(content, workflowName))
	})

	t.Run("overwrite without force flag fails", func(t *testing.T) {
		opts := optsIntegration{
			withFunc: baseOpts.withFunc,
			args:     append(slices.Clone(baseOpts.args), "--workflow-name="+changedWorkflowName),
		}

		err := runConfigCiCmdIntegration(t, opts)
		content := readWorkflowFile(t, opts.withFunc.Root)

		assert.ErrorIs(t, err, github.ErrWorkflowExists)
		assert.Assert(t, yamlContains(content, workflowName))
		assert.Assert(t, !strings.Contains(content, changedWorkflowName))
	})

	t.Run("overwrite with force flag succeeds", func(t *testing.T) {
		opts := optsIntegration{
			withFunc: baseOpts.withFunc,
			args:     append(slices.Clone(baseOpts.args), "--workflow-name="+changedWorkflowName, "--force"),
		}

		err := runConfigCiCmdIntegration(t, opts)
		content := readWorkflowFile(t, opts.withFunc.Root)

		assert.NilError(t, err)
		assert.Assert(t, yamlContains(content, changedWorkflowName))
		assert.Assert(t, !strings.Contains(content, workflowName))
	})
}

// ----------------------
// END: Integration Tests

// START: Testing Framework
// ------------------------
const fnName = "github-ci-func"

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
	t.Setenv(fnCmd.ConfigCIFeatureFlag, "true")

	args := opts.args
	if len(opts.args) == 0 {
		args = []string{"ci"}
	}

	viper.Reset()

	cmd := fnCmd.NewConfigCmd(
		common.DefaultLoaderSaver,
		github.DefaultWorkflowWriter,
		common.DefaultCurrentBranch,
		common.DefaultWorkDir,
		fnCmd.NewCIGeneratorFactory(),
		fnCmd.NewClient,
	)
	cmd.SetArgs(args)

	// RUN
	return cmd.Execute()
}

func readWorkflowFile(t *testing.T, root string) string {
	t.Helper()

	path := filepath.Join(root, github.DefaultGitHubWorkflowDir, github.DefaultGitHubWorkflowFilename)
	result, err := os.ReadFile(path)
	assert.NilError(t, err)

	return string(result)
}

func assertDefaultWorkflow(t *testing.T, actualGw string) {
	t.Helper()

	assert.Assert(t, yamlContains(actualGw, "Func Deploy"))
	assert.Assert(t, yamlContains(actualGw, "- main"))

	assert.Assert(t, yamlContains(actualGw, "ubuntu-latest"))

	assert.Assert(t, strings.Count(actualGw, "- name:") == 6)

	assert.Assert(t, yamlContains(actualGw, "Checkout code"))
	assert.Assert(t, yamlContains(actualGw, "actions/checkout@v4"))

	assert.Assert(t, yamlContains(actualGw, "Run tests"))
	assert.Assert(t, yamlContains(actualGw, "go test ./..."))

	assert.Assert(t, yamlContains(actualGw, "Setup Kubernetes context"))
	assert.Assert(t, yamlContains(actualGw, "azure/k8s-set-context@v4"))
	assert.Assert(t, yamlContains(actualGw, "method: kubeconfig"))
	assert.Assert(t, yamlContains(actualGw, "kubeconfig: ${{ secrets.KUBECONFIG }}"))

	assert.Assert(t, yamlContains(actualGw, "Login to container registry"))
	assert.Assert(t, yamlContains(actualGw, "docker/login-action@v3"))
	assert.Assert(t, yamlContains(actualGw, "registry: ${{ vars.REGISTRY_LOGIN_URL }}"))
	assert.Assert(t, yamlContains(actualGw, "username: ${{ vars.REGISTRY_USERNAME }}"))
	assert.Assert(t, yamlContains(actualGw, "password: ${{ secrets.REGISTRY_PASSWORD }}"))

	assert.Assert(t, yamlContains(actualGw, "Install func cli"))
	assert.Assert(t, yamlContains(actualGw, "functions-dev/action@main"))
	assert.Assert(t, yamlContains(actualGw, "version: knative-v1.22.0"))
	assert.Assert(t, yamlContains(actualGw, "name: func"))

	assert.Assert(t, yamlContains(actualGw, "Deploy function"))
	assert.Assert(t, yamlContains(actualGw, `FUNC_VERBOSE: "true"`))
	assert.Assert(t, yamlContains(actualGw, "FUNC_REGISTRY: ${{ vars.REGISTRY_LOGIN_URL }}/${{ vars.REGISTRY_USERNAME }}"))
	assert.Assert(t, yamlContains(actualGw, "func deploy"))
}

func yamlContains(yaml, substr string) cmp.Comparison {
	return func() cmp.Result {
		if strings.Contains(yaml, substr) {
			return cmp.ResultSuccess
		}
		return cmp.ResultFailure(fmt.Sprintf(
			"missing '%s' in:\n\n%s", substr, yaml,
		))
	}
}

// ----------------------
// END: Testing Framework
