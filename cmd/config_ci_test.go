package cmd_test

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"gotest.tools/v3/assert"
	"gotest.tools/v3/assert/cmp"
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

func TestNewConfigCICmd_GeneratesWorkflowFile(t *testing.T) {
	options := opts{withFuncInTempDir: true}

	result := runConfigCiGithubCmd(t, options)

	assert.NilError(t, result.executeErr)
	assertWorkflowFileExists(t, result)
}

func TestNewConfigCICmd_WorkflowYAMLHasCorrectStructure(t *testing.T) {
	// GIVEN
	options := opts{
		withFuncInTempDir: true,
	}

	// WHEN
	result := runConfigCiGithubCmd(t, options)

	// THEN
	assert.NilError(t, result.executeErr)
	assertWorkflowFileExists(t, result)
	assert.NilError(t, result.gwLoadErr)
	assertWorkflowFileContent(t, result.gwYamlString)
}

func TestNewConfigCICmd_WorkflowYAMLHasCustomValues(t *testing.T) {
	// GIVEN
	ciConfig := ci.NewCIConfigBuilder().
		WithWorkflowName("Custom Remote Build and Deploy").
		WithKubeconfigKey("DEV_CLUSTER_KUBECONFIG").
		WithRegistryUrlKey("DEV_REGISTRY_URL").
		WithRegistryUserKey("DEV_REGISTRY_USER").
		WithRegistryPassKey("DEV_REGISTRY_PASS").
		WithSelfHosted(true).
		Build()
	options := opts{
		withFuncInTempDir: true,
		ciConfig:          &ciConfig,
	}

	// WHEN
	result := runConfigCiGithubCmd(t, options)

	// THEN
	assert.NilError(t, result.executeErr)
	assertWorkflowFileExists(t, result)
	assert.Assert(t, yamlContains(result.gwYamlString, ciConfig.WorkflowName()))
	assert.Assert(t, yamlContains(result.gwYamlString, "self-hosted"))
	assert.Assert(t, yamlContains(result.gwYamlString, ciConfig.KubeconfigSecretKey()))
	assert.Assert(t, yamlContains(result.gwYamlString, ciConfig.RegistryUrlSecretKey()))
	assert.Assert(t, yamlContains(result.gwYamlString, ciConfig.RegistryUserSecretKey()))
	assert.Assert(t, yamlContains(result.gwYamlString, ciConfig.RegistryPassSecretKey()))
}

func TestNewConfigCICmd_WorkflowHasNoRegistryLogin(t *testing.T) {
	// GIVEN
	ciConfig := ci.NewCIConfigBuilder().
		WithoutRegistryLogin().
		Build()
	options := opts{
		withFuncInTempDir: true,
		ciConfig:          &ciConfig,
		args:              []string{"ci", "--github", "--use-registry-login=false"},
	}

	// WHEN
	result := runConfigCiGithubCmd(t, options)

	// THEN
	assert.NilError(t, result.executeErr)
	assertWorkflowFileExists(t, result)
	assert.Assert(t, !strings.Contains(result.gwYamlString, "docker/login-action@v3"))
	assert.Assert(t, !strings.Contains(result.gwYamlString, "Login to container registry"))
}

func TestNewConfigCICmd_RemoteBuildRemoteDeployWorkflow(t *testing.T) {
	// GIVEN
	ciConfig := ci.NewCIConfigBuilder().
		WithRemoteBuild().
		Build()
	options := opts{
		withFuncInTempDir: true,
		ciConfig:          &ciConfig,
	}

	// WHEN
	result := runConfigCiGithubCmd(t, options)

	// THEN
	assert.NilError(t, result.executeErr)
	assertWorkflowFileExists(t, result)
	assert.Assert(t, yamlContains(result.gwYamlString, "func deploy --remote"))
}

func TestNewConfigCICmd_HasWorkflowDispatchAndCacheInDebugMode(t *testing.T) {
	// GIVEN
	ciConfig := ci.NewCIConfigBuilder().
		WithDebug().
		Build()
	options := opts{
		withFuncInTempDir: true,
		ciConfig:          &ciConfig,
	}

	// WHEN
	result := runConfigCiGithubCmd(t, options)

	// THEN
	assert.NilError(t, result.executeErr)
	assertWorkflowFileExists(t, result)
	assert.Assert(t, yamlContains(result.gwYamlString, "workflow_dispatch"))
	assert.Assert(t, yamlContains(result.gwYamlString, "Restore func cli from cache"))
	assert.Assert(t, yamlContains(result.gwYamlString, "func-cli-cache"))
	assert.Assert(t, yamlContains(result.gwYamlString, "actions/cache@v4"))
	assert.Assert(t, yamlContains(result.gwYamlString, "path: func"))
	assert.Assert(t, yamlContains(result.gwYamlString, "key: func-cli-knative-v1.19.1"))
	assert.Assert(t, yamlContains(result.gwYamlString, "if: ${{ steps.func-cli-cache.outputs.cache-hit != 'true' }}"))
}

// START: Testing Framework
// ------------------------
type opts struct {
	withFuncInTempDir bool
	args              []string // default: ci --github
	ciConfig          *ci.CIConfig
}

type result struct {
	f            fn.Function
	ciConfig     ci.CIConfig
	executeErr   error
	gwLoadErr    error
	gwYamlString string
	// TODO(twoGiants): probably not needed
	gwYamlMarshalErr error
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
		ciConf := ci.NewCIConfigBuilder().Build()
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
	gwYamlString, gwYamlMarshalErr := gw.YamlString()

	return result{
		f,
		*opts.ciConfig,
		err,
		gwLoadErr,
		gwYamlString,
		gwYamlMarshalErr,
	}
}

func assertWorkflowFileExists(t *testing.T, result result) {
	t.Helper()

	filepath := result.ciConfig.FnGithubWorkflowFilepath(result.f.Root)
	exists, _ := fnTest.FileExists(t, filepath)

	assert.Assert(t, exists, filepath+" does not exist")
}

// assertWorkflowFileContent asserts all the Github workflow value for correct values
// including the default values which can be changed:
//   - runs-on: ubuntu-latest
//   - kubeconfig: ${{ secrets.KUBECONFIG }}
//   - if: ${{ vars.USE_REGISTRY_AUTH == 'true' }
//   - registry: ${{ secrets.REGISTRY_URL }}")
//   - username: ${{ secrets.REGISTRY_USERNAME }}
//   - password: ${{ secrets.REGISTRY_PASSWORD }}
//   - run: func deploy --registry=${{ secrets.REGISTRY_URL }} -v
func assertWorkflowFileContent(t *testing.T, actualGw string) {
	t.Helper()

	assert.Assert(t, yamlContains(actualGw, "Remote Build and Deploy"))
	assert.Assert(t, yamlContains(actualGw, "main"))

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
	assert.Assert(t, yamlContains(actualGw, "registry: ${{ secrets.REGISTRY_URL }}"))
	assert.Assert(t, yamlContains(actualGw, "username: ${{ secrets.REGISTRY_USERNAME }}"))
	assert.Assert(t, yamlContains(actualGw, "password: ${{ secrets.REGISTRY_PASSWORD }}"))

	assert.Assert(t, yamlContains(actualGw, "Install func cli"))
	assert.Assert(t, yamlContains(actualGw, "gauron99/knative-func-action@main"))
	assert.Assert(t, yamlContains(actualGw, "version: knative-v1.19.1"))
	assert.Assert(t, yamlContains(actualGw, "name: func"))

	assert.Assert(t, yamlContains(actualGw, "Deploy function"))
	assert.Assert(t, yamlContains(actualGw, "func deploy --registry=${{ secrets.REGISTRY_URL }} -v"))
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
