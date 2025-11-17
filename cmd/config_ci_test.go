package cmd_test

import (
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
	assert.NilError(t, result.gwLoadErr)
	assertWorkflowFileContent(t, result.gwYamlString)
}

func TestNewConfigCICmd_WorkflowYAMLHasCustomValues(t *testing.T) {
	// GIVEN
	ciConfig := ci.NewCIConfigBuilder().
		WithWorkflowName("remote-build-and-deploy.yaml").
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
	assert.Assert(t, cmp.Contains(result.gwYamlString, ciConfig.WorkflowName()))
	assert.Assert(t, cmp.Contains(result.gwYamlString, "self-hosted"))
	assert.Assert(t, cmp.Contains(result.gwYamlString, ciConfig.KubeconfigSecretKey()))
	assert.Assert(t, cmp.Contains(result.gwYamlString, ciConfig.RegistryUrlSecretKey()))
	assert.Assert(t, cmp.Contains(result.gwYamlString, ciConfig.RegistryUserSecretKey()))
	assert.Assert(t, cmp.Contains(result.gwYamlString, ciConfig.RegistryPassSecretKey()))
}

func TestNewConfigCICmd_WorkflowUsesInsecureRegistry(t *testing.T) {
	// GIVEN
	ciConfig := ci.NewCIConfigBuilder().
		WithRegistryLogin(false).
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
	assert.Assert(t, !strings.Contains(result.gwYamlString, "docker/login-action@v3"))
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
//   - run: func deploy --remote --registry=${{ secrets.REGISTRY_URL }} -v
func assertWorkflowFileContent(t *testing.T, actualGw string) {
	t.Helper()

	assert.Assert(t, cmp.Contains(actualGw, "Remote Build and Deploy"))
	assert.Assert(t, cmp.Contains(actualGw, "main"))

	assert.Assert(t, cmp.Contains(actualGw, "ubuntu-latest"))

	assert.Assert(t, strings.Count(actualGw, "- name:") == 5)

	assert.Assert(t, cmp.Contains(actualGw, "Checkout code"))
	assert.Assert(t, cmp.Contains(actualGw, "actions/checkout@v4"))

	assert.Assert(t, cmp.Contains(actualGw, "Setup Kubernetes context"))
	assert.Assert(t, cmp.Contains(actualGw, "azure/k8s-set-context@v4"))
	assert.Assert(t, cmp.Contains(actualGw, "method: kubeconfig"))
	assert.Assert(t, cmp.Contains(actualGw, "kubeconfig: ${{ secrets.KUBECONFIG }}"))

	assert.Assert(t, cmp.Contains(actualGw, "Login to container registry"))
	assert.Assert(t, cmp.Contains(actualGw, "docker/login-action@v3"))
	assert.Assert(t, cmp.Contains(actualGw, "registry: ${{ secrets.REGISTRY_URL }}"))
	assert.Assert(t, cmp.Contains(actualGw, "username: ${{ secrets.REGISTRY_USERNAME }}"))
	assert.Assert(t, cmp.Contains(actualGw, "password: ${{ secrets.REGISTRY_PASSWORD }}"))

	assert.Assert(t, cmp.Contains(actualGw, "Install func cli"))
	assert.Assert(t, cmp.Contains(actualGw, "gauron99/knative-func-action@main"))
	assert.Assert(t, cmp.Contains(actualGw, "version: knative-v1.19.1"))
	assert.Assert(t, cmp.Contains(actualGw, "name: func"))

	assert.Assert(t, cmp.Contains(actualGw, "Deploy function"))
	assert.Assert(t, cmp.Contains(actualGw, "func deploy --remote --registry=${{ secrets.REGISTRY_URL }} -v"))
}
