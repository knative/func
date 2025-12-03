package cmd_test

import (
	"fmt"
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

func TestNewConfigCICmd_RequiresFeatureFlag(t *testing.T) {
	expectedErrMsg := fmt.Sprintf("set %s to 'true' to use this feature", ci.ConfigCIFeatureFlag)

	result := runConfigCiCmd(t, opts{enableFeature: false})

	assert.Error(t, result.executeErr, expectedErrMsg)
}

func TestNewConfigCICmd_CISubcommandExist(t *testing.T) {
	// leave 'ci' to make this test explicitly use this subcommand
	opts := opts{withFuncInTempDir: true, enableFeature: true, args: []string{"ci"}}

	result := runConfigCiCmd(t, opts)

	assert.NilError(t, result.executeErr)
}

func TestNewConfigCICmd_FailsWhenNotInitialized(t *testing.T) {
	expectedErrMsg := fn.NewErrNotInitialized(fnTest.Cwd()).Error()

	result := runConfigCiCmd(t, opts{withFuncInTempDir: false, enableFeature: true})

	assert.Error(t, result.executeErr, expectedErrMsg)
}

func TestNewConfigCICmd_SuccessWhenInitialized(t *testing.T) {
	result := runConfigCiCmd(t, defaultOpts())

	assert.NilError(t, result.executeErr)
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
	_, fnInitErr := fn.New().Init(
		fn.Function{Name: "github-ci-func", Runtime: "go", Root: tmpDir},
	)

	result := runConfigCiCmd(t, opt)

	assert.NilError(t, fnInitErr)
	assert.NilError(t, result.executeErr)
}

func TestNewConfigCICmd_CreatesGithubWorkflowDirectory(t *testing.T) {
	result := runConfigCiCmd(t, defaultOpts())

	assert.NilError(t, result.executeErr)
	_, err := os.Stat(result.ciConfig.FnGithubWorkflowDir(result.f.Root))
	assert.NilError(t, err)
}

func TestNewConfigCICmd_GeneratesWorkflowFile(t *testing.T) {
	result := runConfigCiCmd(t, defaultOpts())

	assert.NilError(t, result.executeErr)
	assertWorkflowFileExists(t, result)
}

func TestNewConfigCICmd_WorkflowYAMLHasCorrectStructure(t *testing.T) {
	result := runConfigCiCmd(t, defaultOpts())

	assert.NilError(t, result.executeErr)
	assertWorkflowFileExists(t, result)
	assert.NilError(t, result.gwLoadErr)
	assertDefaultWorkflow(t, result.gwYamlString)
}

func TestNewConfigCICmd_WorkflowYAMLHasCustomValues(t *testing.T) {
	// GIVEN
	opts := defaultOpts()
	opts.args = append(opts.args,
		"--self-hosted-runner",
		"--workflow-name=Custom Deploy",
		"--kubeconfig-secret-name=DEV_CLUSTER_KUBECONFIG",
		"--registry-login-url-variable-name=DEV_REGISTRY_LOGIN_URL",
		"--registry-user-variable-name=DEV_REGISTRY_USER",
		"--registry-pass-secret-name=DEV_REGISTRY_PASS",
		"--branch=master",
	)

	// WHEN
	result := runConfigCiCmd(t, opts)

	// THEN
	assert.NilError(t, result.executeErr)
	assertWorkflowFileExists(t, result)
	assertCustomWorkflow(t, result.gwYamlString)
}

func TestNewConfigCICmd_WorkflowHasNoRegistryLogin(t *testing.T) {
	// GIVEN
	opts := defaultOpts()
	opts.args = append(opts.args, "--use-registry-login=false")

	// WHEN
	result := runConfigCiCmd(t, opts)

	// THEN
	assert.NilError(t, result.executeErr)
	assertWorkflowFileExists(t, result)
	assert.Assert(t, !strings.Contains(result.gwYamlString, "docker/login-action@v3"))
	assert.Assert(t, !strings.Contains(result.gwYamlString, "Login to container registry"))
	assert.Assert(t, yamlContains(result.gwYamlString, "--registry=${{ vars.REGISTRY_URL }}"))
}

func TestNewConfigCICmd_RemoteBuildAndDeployWorkflow(t *testing.T) {
	// GIVEN
	opts := defaultOpts()
	opts.args = append(opts.args, "--remote")

	// WHEN
	result := runConfigCiCmd(t, opts)

	// THEN
	assert.NilError(t, result.executeErr)
	assertWorkflowFileExists(t, result)
	assert.Assert(t, yamlContains(result.gwYamlString, "Remote Func Deploy"))
	assert.Assert(t, yamlContains(result.gwYamlString, "func deploy --remote"))
}

func TestNewConfigCICmd_HasWorkflowDispatchAndCacheInDebugMode(t *testing.T) {
	// GIVEN
	opts := defaultOpts()
	opts.args = append(opts.args, "--debug")

	// WHEN
	result := runConfigCiCmd(t, opts)

	// THEN
	assert.NilError(t, result.executeErr)
	assertWorkflowFileExists(t, result)
	assertDebugWorkflow(t, result.gwYamlString)
}

// START: Testing Framework
// ------------------------
type opts struct {
	withFuncInTempDir bool
	enableFeature     bool
	args              []string
}

// defaultOpts provides the most used options for tests
//
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
	f            fn.Function
	ciConfig     ci.CIConfig
	executeErr   error
	gwLoadErr    error
	gwYamlString string
}

func runConfigCiCmd(
	t *testing.T,
	opts opts,
) result {
	t.Helper()

	// PRE-RUN PREP
	// all options for "func config ci" command
	f := fn.Function{}
	if opts.withFuncInTempDir {
		f = cmdTest.CreateFuncInTempDir(t, "github-ci-func")
	}

	if opts.enableFeature {
		t.Setenv(ci.ConfigCIFeatureFlag, "true")
	}

	args := opts.args
	if len(opts.args) == 0 {
		args = []string{"ci"}
	}

	viper.Reset()

	cmd := fnCmd.NewConfigCmd(
		common.DefaultLoaderSaver,
		fnCmd.NewClient,
	)
	cmd.SetArgs(args)

	// RUN
	err := cmd.Execute()

	// POST-RUN GATHER
	ciConfig := ci.NewCiGithubConfig()
	gwPath := ciConfig.FnGithubWorkflowFilepath(f.Root)
	gw, gwLoadErr := ci.NewGithubWorkflowFromPath(gwPath)
	gwYamlString, marshalErr := gw.YamlString()
	if marshalErr != nil {
		panic(marshalErr)
	}

	return result{
		f,
		ciConfig,
		err,
		gwLoadErr,
		gwYamlString,
	}
}

func assertWorkflowFileExists(t *testing.T, result result) {
	t.Helper()

	filepath := result.ciConfig.FnGithubWorkflowFilepath(result.f.Root)
	exists, _ := fnTest.FileExists(t, filepath)

	assert.Assert(t, exists, filepath+" does not exist")
}

// assertDefaultWorkflow asserts all the Github workflow value for correct values
// including the default values which can be changed:
//   - runs-on: ubuntu-latest
//   - kubeconfig: ${{ secrets.KUBECONFIG }}
//   - registry: ${{ vars.REGISTRY_LOGIN_URL }}")
//   - username: ${{ vars.REGISTRY_USERNAME }}
//   - password: ${{ secrets.REGISTRY_PASSWORD }}
//   - run: func deploy --registry=${{ vars.REGISTRY_LOGIN_URL }}/${{ vars.REGISTRY_USERNAME }} -v
func assertDefaultWorkflow(t *testing.T, actualGw string) {
	t.Helper()

	assert.Assert(t, yamlContains(actualGw, "Func Deploy"))
	assert.Assert(t, yamlContains(actualGw, "- main"))

	assert.Assert(t, yamlContains(actualGw, "ubuntu-latest"))

	assert.Assert(t, strings.Count(actualGw, "- name:") == 5)

	assert.Assert(t, yamlContains(actualGw, "Checkout code"))
	assert.Assert(t, yamlContains(actualGw, "actions/checkout@v4"))

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
	assert.Assert(t, yamlContains(actualGw, "gauron99/knative-func-action@main"))
	assert.Assert(t, yamlContains(actualGw, "version: knative-v1.19.1"))
	assert.Assert(t, yamlContains(actualGw, "name: func"))

	assert.Assert(t, yamlContains(actualGw, "Deploy function"))
	assert.Assert(t, yamlContains(actualGw, "func deploy --registry=${{ vars.REGISTRY_LOGIN_URL }}/${{ vars.REGISTRY_USERNAME }} -v"))
}

func assertCustomWorkflow(t *testing.T, actualGw string) {
	assert.Assert(t, yamlContains(actualGw, "Custom Deploy"))
	assert.Assert(t, yamlContains(actualGw, "self-hosted"))
	assert.Assert(t, yamlContains(actualGw, "DEV_CLUSTER_KUBECONFIG"))
	assert.Assert(t, yamlContains(actualGw, "DEV_REGISTRY_LOGIN_URL"))
	assert.Assert(t, yamlContains(actualGw, "DEV_REGISTRY_USER"))
	assert.Assert(t, yamlContains(actualGw, "DEV_REGISTRY_PASS"))
	assert.Assert(t, yamlContains(actualGw, "- master"))
}

func assertDebugWorkflow(t *testing.T, actualGw string) {
	assert.Assert(t, yamlContains(actualGw, "workflow_dispatch"))
	assert.Assert(t, yamlContains(actualGw, "Restore func cli from cache"))
	assert.Assert(t, yamlContains(actualGw, "func-cli-cache"))
	assert.Assert(t, yamlContains(actualGw, "actions/cache@v4"))
	assert.Assert(t, yamlContains(actualGw, "path: func"))
	assert.Assert(t, yamlContains(actualGw, "key: func-cli-knative-v1.19.1"))
	assert.Assert(t, yamlContains(actualGw, "if: ${{ steps.func-cli-cache.outputs.cache-hit != 'true' }}"))
	assert.Assert(t, yamlContains(actualGw, "Add func to GITHUB_PATH"))
	assert.Assert(t, yamlContains(actualGw, `echo "$GITHUB_WORKSPACE" >> $GITHUB_PATH`))
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
