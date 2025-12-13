package cmd_test

import (
	"fmt"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/ory/viper"
	"gotest.tools/v3/assert"
	"gotest.tools/v3/assert/cmp"
	fnCmd "knative.dev/func/cmd"
	"knative.dev/func/cmd/ci"
	"knative.dev/func/cmd/common"
	cmdTest "knative.dev/func/cmd/testing"
	fn "knative.dev/func/pkg/functions"
	fnTest "knative.dev/func/pkg/testing"
)

// START: Broad Unit Tests
// -----------------------
// Execution is testet starting from the entrypoint "func config ci" including
// all components working together. Infrastructure components like the
// filesystem are mocked.
func TestNewConfigCICmd_RequiresFeatureFlag(t *testing.T) {
	result := runConfigCiCmd(t, opts{enableFeature: false})

	assert.ErrorContains(t, result.executeErr, "unknown command \"ci\" for \"config\"")
}

func TestNewConfigCICmd_CISubcommandExist(t *testing.T) {
	// leave 'ci' to make this test explicitly use this subcommand
	opts := opts{
		enableFeature:       true,
		withMockLoaderSaver: true,
		withBufferWriter:    true,
		args:                []string{"ci"},
	}

	result := runConfigCiCmd(t, opts)

	assert.NilError(t, result.executeErr)
}

func TestNewConfigCICmd_WritesWorkflowFile(t *testing.T) {
	result := runConfigCiCmd(t, unitTestOpts())

	assert.NilError(t, result.executeErr)
	assert.Assert(t, result.gwYamlString != "")
}

func TestNewConfigCICmd_WorkflowYAMLHasCorrectStructure(t *testing.T) {
	opts := unitTestOpts()
	result := runConfigCiCmd(t, opts)

	assert.NilError(t, result.executeErr)
	assertDefaultWorkflow(t, result.gwYamlString)
}

// ---------------------
// END: Broad Unit Tests

// START: Integration Tests
// ------------------------
// No more mocking. Using real filesystem here for LoaderSaver and WorkflowWriter.
func TestNewConfigCICmd_FailsWhenNotInitialized(t *testing.T) {
	opts := opts{enableFeature: true, withFuncInTempDir: false}
	expectedErrMsg := fn.NewErrNotInitialized(fnTest.Cwd()).Error()

	result := runConfigCiCmd(t, opts)

	assert.Error(t, result.executeErr, expectedErrMsg)
}

func TestNewConfigCICmd_SuccessWhenInitialized(t *testing.T) {
	result := runConfigCiCmd(t, integrationTestOpts())

	assert.NilError(t, result.executeErr)
}

func TestNewConfigCICmd_FailsToLoadFuncWithWrongPath(t *testing.T) {
	opts := integrationTestOpts()
	opts.args = append(opts.args, "--path=nofunc")
	expectedErrMsg := "failed to create new function"

	result := runConfigCiCmd(t, opts)

	assert.ErrorContains(t, result.executeErr, expectedErrMsg)
}

func TestNewConfigCICmd_SuccessfulLoadWithCorrectPath(t *testing.T) {
	tmpDir := t.TempDir()
	opts := integrationTestOpts()
	opts.args = append(opts.args, "--path="+tmpDir)
	_, fnInitErr := fn.New().Init(
		fn.Function{Name: "github-ci-func", Runtime: "go", Root: tmpDir},
	)

	result := runConfigCiCmd(t, opts)

	assert.NilError(t, fnInitErr)
	assert.NilError(t, result.executeErr)
}

func TestNewConfigCICmd_CreatesGitHubWorkflowDirectory(t *testing.T) {
	result := runConfigCiCmd(t, integrationTestOpts())

	assert.NilError(t, result.executeErr)
	_, err := os.Stat(result.ciConfig.FnGitHubWorkflowDir(result.f.Root))
	assert.NilError(t, err)
}

func TestNewConfigCICmd_WritesWorkflowFileToFSWithCorrectYAMLStructure(t *testing.T) {
	result := runConfigCiCmd(t, integrationTestOpts())
	file, openErr := os.Open(result.ciConfig.FnGitHubWorkflowFilepath(result.f.Root))
	raw, readErr := io.ReadAll(file)

	assert.NilError(t, result.executeErr)
	assert.NilError(t, openErr)
	assert.NilError(t, readErr)
	assertDefaultWorkflow(t, string(raw))

	file.Close()
}

// ----------------------
// END: Integration Tests

// START: Testing Framework
// ------------------------
type opts struct {
	withMockLoaderSaver bool
	withFuncInTempDir   bool
	enableFeature       bool
	withBufferWriter    bool
	args                []string
}

// unitTestOpts contains test options for broad unit tests
//
//   - withMockLoaderSaver: true,
//   - withFuncInTempDir:   false,
//   - enableFeature:       true,
//   - withBufferWriter:    true,
//   - args:                []string{"ci"},
func unitTestOpts() opts {
	return opts{
		withMockLoaderSaver: true,
		withFuncInTempDir:   false,
		enableFeature:       true,
		withBufferWriter:    true,
		args:                []string{"ci"},
	}
}

// integrationTestOpts contains test options for integration tests
//
//   - withMockLoaderSaver: false,
//   - withFuncInTempDir:   true,
//   - enableFeature:       true,
//   - withBufferWriter:    false,
//   - args:                []string{"ci"},
func integrationTestOpts() opts {
	return opts{
		withMockLoaderSaver: false,
		withFuncInTempDir:   true,
		enableFeature:       true,
		withBufferWriter:    false,
		args:                []string{"ci"},
	}
}

type result struct {
	f            fn.Function
	ciConfig     ci.CIConfig
	executeErr   error
	gwYamlString string
}

func runConfigCiCmd(
	t *testing.T,
	opts opts,
) result {
	t.Helper()

	// PRE-RUN PREP
	// all options for "func config ci" command
	loaderSaver := common.DefaultLoaderSaver
	if opts.withMockLoaderSaver {
		loaderSaver = common.NewMockLoaderSaver()
	}

	f := fn.Function{}
	if opts.withFuncInTempDir {
		f = cmdTest.CreateFuncInTempDir(t, "github-ci-func")
	}

	if opts.enableFeature {
		t.Setenv(ci.ConfigCIFeatureFlag, "true")
	}

	var writer ci.WorkflowWriter = ci.DefaultWorkflowWriter
	bufferWriter := ci.NewBufferWriter()
	if opts.withBufferWriter {
		writer = bufferWriter
	}

	args := opts.args
	if len(opts.args) == 0 {
		args = []string{"ci"}
	}

	viper.Reset()

	cmd := fnCmd.NewConfigCmd(
		loaderSaver,
		writer,
		fnCmd.NewClient,
	)
	cmd.SetArgs(args)

	// RUN
	err := cmd.Execute()

	// POST-RUN GATHER
	ciConfig := ci.NewCIGitHubConfig()
	gwYamlString := bufferWriter.Buffer.String()

	return result{
		f,
		ciConfig,
		err,
		gwYamlString,
	}
}

// assertDefaultWorkflow asserts all the GitHub workflow value for correct values
// including the default values which can be changed:
//   - runs-on: ubuntu-latest
//   - kubeconfig: ${{ secrets.KUBECONFIG }}
//   - run: func deploy --registry=${{ vars.REGISTRY_URL }} -v
func assertDefaultWorkflow(t *testing.T, actualGw string) {
	t.Helper()

	assert.Assert(t, yamlContains(actualGw, "Func Deploy"))
	assert.Assert(t, yamlContains(actualGw, "- main"))

	assert.Assert(t, yamlContains(actualGw, "ubuntu-latest"))

	assert.Assert(t, strings.Count(actualGw, "- name:") == 4)

	assert.Assert(t, yamlContains(actualGw, "Checkout code"))
	assert.Assert(t, yamlContains(actualGw, "actions/checkout@v4"))

	assert.Assert(t, yamlContains(actualGw, "Setup Kubernetes context"))
	assert.Assert(t, yamlContains(actualGw, "azure/k8s-set-context@v4"))
	assert.Assert(t, yamlContains(actualGw, "method: kubeconfig"))
	assert.Assert(t, yamlContains(actualGw, "kubeconfig: ${{ secrets.KUBECONFIG }}"))

	assert.Assert(t, yamlContains(actualGw, "Install func cli"))
	assert.Assert(t, yamlContains(actualGw, "gauron99/knative-func-action@main"))
	assert.Assert(t, yamlContains(actualGw, "version: knative-v1.19.1"))
	assert.Assert(t, yamlContains(actualGw, "name: func"))

	assert.Assert(t, yamlContains(actualGw, "Deploy function"))
	assert.Assert(t, yamlContains(actualGw, "func deploy --registry=${{ vars.REGISTRY_URL }} -v"))
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
