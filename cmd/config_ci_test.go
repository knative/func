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
	assertWorkflowFileContent(t, result.gw)
}

func TestNewConfigCICmd_WorkflowYAMLHasCustomValues(t *testing.T) {
	// GIVEN
	name := "Dev Cluster Remote Build and Deploy"
	key := "DEV_CLUSTER_KUBECONFIG"
	ciConfig := ci.NewCIConfig(
		".github/workflows",
		"remote-build-and-deploy.yaml",
		name,
		key,
		true,
	)
	options := opts{
		withFuncInTempDir: true,
		ciConfig:          &ciConfig,
	}

	// WHEN
	result := runConfigCiGithubCmd(t, options)

	// THEN
	assert.NilError(t, result.executeErr)
	assertWorkflowFileExists(t, result)
	assert.Equal(t, result.gw.Name, name)
	assert.Equal(t, result.gw.Jobs["deploy"].RunsOn, "self-hosted")
	assert.Equal(t, result.gw.Jobs["deploy"].Steps[1].With["kubeconfig"], ci.NewSecret(key))
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

// assertWorkflowFileContent asserts all the Github workflow value for correct values
// including the default values which can be changed:
//   - runs-on: ubuntu-latest
//   - kubeconfig: ${{ secrets.KUBECONFIG }}
//   - if: ${{ vars.USE_REGISTRY_AUTH == 'true' }
//   - registry: ${{ secrets.REGISTRY_URL }}")
//   - username: ${{ secrets.REGISTRY_USERNAME }}
//   - password: ${{ secrets.REGISTRY_PASSWORD }}
//   - run: func deploy --remote --registry=${{ secrets.REGISTRY_URL }} -v
func assertWorkflowFileContent(t *testing.T, actualGw *ci.GithubWorkflow) {
	t.Helper()

	assert.Equal(t, actualGw.Name, "Remote Build and Deploy")
	assert.Equal(t, actualGw.On.Push.Branches[0], "main")

	deployJob := actualGw.Jobs["deploy"]
	assert.Equal(t, deployJob.RunsOn, "ubuntu-latest")

	deployJobStep1 := deployJob.Steps[0]
	assert.Equal(t, deployJobStep1.Name, "1. Checkout code")
	assert.Equal(t, deployJobStep1.Uses, "actions/checkout@v4")

	deployJobStep2 := deployJob.Steps[1]
	assert.Equal(t, deployJobStep2.Name, "2. Setup Kubernetes context")
	assert.Equal(t, deployJobStep2.Uses, "azure/k8s-set-context@v4")
	assert.Equal(t, deployJobStep2.With["method"], "kubeconfig")
	assert.Equal(t, deployJobStep2.With["kubeconfig"], "${{ secrets.KUBECONFIG }}")

	deployJobStep3 := deployJob.Steps[2]
	assert.Equal(t, deployJobStep3.Name, "3. Login to container registry")
	assert.Equal(t, deployJobStep3.If, "${{ vars.USE_REGISTRY_AUTH == 'true' }}")
	assert.Equal(t, deployJobStep3.Uses, "docker/login-action@v3")
	assert.Equal(t, deployJobStep3.With["registry"], "${{ secrets.REGISTRY_URL }}")
	assert.Equal(t, deployJobStep3.With["username"], "${{ secrets.REGISTRY_USERNAME }}")
	assert.Equal(t, deployJobStep3.With["password"], "${{ secrets.REGISTRY_PASSWORD }}")

	deployJobStep4 := deployJob.Steps[3]
	assert.Equal(t, deployJobStep4.Name, "4. Install func cli")
	assert.Equal(t, deployJobStep4.Uses, "gauron99/knative-func-action@main")
	assert.Equal(t, deployJobStep4.With["version"], "knative-v1.19.1")
	assert.Equal(t, deployJobStep4.With["name"], "func")

	deployJobStep5 := deployJob.Steps[4]
	assert.Equal(t, deployJobStep5.Name, "5. Deploy function")
	assert.Equal(t, deployJobStep5.Run, "func deploy --remote --registry=${{ secrets.REGISTRY_URL }} -v")
}
