package cmd_test

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ory/viper"
	"gotest.tools/v3/assert"
	"gotest.tools/v3/assert/cmp"
	fnCmd "knative.dev/func/cmd"
	"knative.dev/func/cmd/ci"
	"knative.dev/func/cmd/common"
	fn "knative.dev/func/pkg/functions"
)

// START: Broad Unit Tests
// -----------------------
// Execution is tested starting from the entrypoint "func config ci" including
// all components working together. Infrastructure components like the
// filesystem are mocked.
func TestNewConfigCICmd_RequiresFeatureFlag(t *testing.T) {
	opts := defaultOpts()
	opts.enableFeature = false

	result := runConfigCiCmd(t, opts)

	assert.ErrorContains(t, result.executeErr, "unknown command \"ci\" for \"config\"")
}

func TestNewConfigCICmd_CISubcommandExist(t *testing.T) {
	// leave 'ci' to make this test explicitly use this subcommand
	opts := defaultOpts()
	opts.args = []string{"ci"}

	result := runConfigCiCmd(t, opts)

	assert.NilError(t, result.executeErr)
}

func TestNewConfigCICmd_WritesWorkflowFile(t *testing.T) {
	result := runConfigCiCmd(t, defaultOpts())

	assert.NilError(t, result.executeErr)
	assert.Assert(t, result.gwYamlString != "")
}

func TestNewConfigCICmd_WorkflowYAMLHasCorrectStructure(t *testing.T) {
	result := runConfigCiCmd(t, defaultOpts())

	assert.NilError(t, result.executeErr)
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
	)

	// WHEN
	result := runConfigCiCmd(t, opts)

	// THEN
	assert.NilError(t, result.executeErr)
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
	assert.Assert(t, yamlContains(result.gwYamlString, "Remote Func Deploy"))
	assert.Assert(t, yamlContains(result.gwYamlString, "func deploy --remote"))
}

func TestNewConfigCICmd_HasWorkflowDispatch(t *testing.T) {
	// GIVEN
	opts := defaultOpts()
	opts.args = append(opts.args, "--workflow-dispatch")

	// WHEN
	result := runConfigCiCmd(t, opts)

	// THEN
	assert.NilError(t, result.executeErr)
	assert.Assert(t, yamlContains(result.gwYamlString, "workflow_dispatch"))
}

func TestNewConfigCICmd_PathFlagResolution(t *testing.T) {
	var (
		// os-agnostic test paths
		cwd              = filepath.Join("current-working-directory")
		explicitFuncPath = filepath.Join("path-to-func")
	)
	testCases := []struct {
		name         string
		pathArg      string // empty means no --path flag
		getwdReturn  string
		expectedPath string
	}{
		{
			name:         "empty path uses cwd",
			pathArg:      "",
			getwdReturn:  cwd,
			expectedPath: cwd,
		},
		{
			name:         "dot path uses cwd",
			pathArg:      "--path=.",
			getwdReturn:  cwd,
			expectedPath: cwd,
		},
		{
			name:         "explicit func path used as-is",
			pathArg:      "--path=" + explicitFuncPath,
			getwdReturn:  cwd,
			expectedPath: explicitFuncPath,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// GIVEN
			opts := defaultOpts()
			opts.args = append(opts.args, tc.pathArg)
			opts.withFakeGetCwdReturn.dir = tc.getwdReturn

			// WHEN
			result := runConfigCiCmd(t, opts)

			// THEN
			assert.NilError(t, result.executeErr)
			assert.Assert(t, strings.Contains(result.actualPath, tc.expectedPath))
		})
	}
}

func TestNewConfigCICmd_PathFlagResolutionError(t *testing.T) {
	// GIVEN
	opts := defaultOpts()
	expectedErr := fmt.Errorf("failed getting current working directory")
	opts.withFakeGetCwdReturn.err = expectedErr

	// WHEN
	result := runConfigCiCmd(t, opts)

	// THEN
	assert.Error(t, result.executeErr, expectedErr.Error())
}

func TestNewConfigCICmd_BranchFlagResolution(t *testing.T) {
	testCases := []struct {
		name           string
		branchArg      string // empty means no --branch flag
		gitCliReturn   string
		expectedBranch string
	}{
		{
			name:           "empty branch uses git cli current branch",
			branchArg:      "",
			gitCliReturn:   issueBranch,
			expectedBranch: issueBranch,
		},
		{
			name:           "explicit branch flag used as-is",
			branchArg:      "--branch=" + mainBranch,
			gitCliReturn:   issueBranch,
			expectedBranch: mainBranch,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// GIVEN
			opts := defaultOpts()
			opts.args = append(opts.args, tc.branchArg)
			opts.withFakeGitCliReturn.output = tc.gitCliReturn

			// WHEN
			result := runConfigCiCmd(t, opts)

			// THEN
			assert.NilError(t, result.executeErr)
			assert.Assert(t, yamlContains(result.gwYamlString, "- "+tc.expectedBranch))
		})
	}
}

func TestNewConfigCICmd_BranchFlagResolutionError(t *testing.T) {
	// GIVEN
	opts := defaultOpts()
	expectedErr := fmt.Errorf("failed getting current branch")
	opts.withFakeGitCliReturn.err = expectedErr

	// WHEN
	result := runConfigCiCmd(t, opts)

	// THEN
	assert.Error(t, result.executeErr, expectedErr.Error())
}

func TestNewConfigCICmd_CICDFlagGitHubSupported(t *testing.T) {
	testCases := []struct {
		name            string
		cicdPlatformArg string
	}{
		{
			name:            "empty value picks GitHub CI/CD platform as default",
			cicdPlatformArg: "",
		},
		{
			name:            "GitHub value is supported",
			cicdPlatformArg: "--platform=github",
		},
		{
			name:            "GitHub value is case insensitive",
			cicdPlatformArg: "--platform=GitHub",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// GIVEN
			opts := defaultOpts()
			opts.args = append(opts.args, tc.cicdPlatformArg)

			// WHEN
			result := runConfigCiCmd(t, opts)

			// THEN
			assert.NilError(t, result.executeErr)
		})
	}
}

func TestNewConfigCICmd_UnsupportedCICDError(t *testing.T) {
	// GIVEN
	platform := "unsupported"
	expectedErr := fmt.Errorf("%s support is not implemented", platform)
	opts := defaultOpts()
	opts.args = append(opts.args, "--platform="+platform)

	// WHEN
	result := runConfigCiCmd(t, opts)

	// THEN
	assert.Error(t, result.executeErr, expectedErr.Error())
}

// ---------------------
// END: Broad Unit Tests

// START: Testing Framework
// ------------------------
const (
	mainBranch  = "main"
	issueBranch = "issue-778-current-branch"
	fnName      = "github-ci-func"
)

type opts struct {
	enableFeature        bool
	withFakeGitCliReturn struct {
		output string
		err    error
	}
	withFakeGetCwdReturn struct {
		dir string
		err error
	}
	args []string
}

// defaultOpts returns test options for broad unit tests with sensible defaults:
//   - enableFeature:        true
//   - withFakeGitCliReturn: {output: issueBranch, err: nil}
//   - withFakeGetCwdReturn: {dir: "", err: nil}
//   - args:                 []string{"ci"}
func defaultOpts() opts {
	return opts{
		enableFeature: true,
		withFakeGitCliReturn: struct {
			output string
			err    error
		}{
			output: issueBranch,
			err:    nil,
		},
		withFakeGetCwdReturn: struct {
			dir string
			err error
		}{
			dir: "",
			err: nil,
		},
		args: []string{"ci"},
	}
}

type result struct {
	executeErr error
	gwYamlString,
	actualPath string
}

func runConfigCiCmd(
	t *testing.T,
	opts opts,
) result {
	t.Helper()

	// PRE-RUN PREP
	// all options for "func config ci" command
	if opts.enableFeature {
		t.Setenv(ci.ConfigCIFeatureFlag, "true")
	}

	loaderSaver := common.NewMockLoaderSaver()
	loaderSaver.LoadFn = func(path string) (fn.Function, error) {
		return fn.Function{Root: path}, nil
	}
	writer := ci.NewBufferWriter()
	currentBranch := common.CurrentBranchStub(
		opts.withFakeGitCliReturn.output,
		opts.withFakeGitCliReturn.err,
	)
	workingDir := common.WorkDirStub(
		opts.withFakeGetCwdReturn.dir,
		opts.withFakeGetCwdReturn.err,
	)

	viper.Reset()

	cmd := fnCmd.NewConfigCmd(
		loaderSaver,
		writer,
		currentBranch,
		workingDir,
		fnCmd.NewClient,
	)
	cmd.SetArgs(opts.args)

	// RUN
	err := cmd.Execute()

	// POST-RUN GATHER
	return result{
		err,
		writer.Buffer.String(),
		writer.Path,
	}
}

// assertDefaultWorkflow asserts all the GitHub workflow value for correct values
// including the default values which can be changed:
//   - runs-on: ubuntu-latest
//   - kubeconfig: ${{ secrets.KUBECONFIG }}
//   - registry: ${{ vars.REGISTRY_LOGIN_URL }}")
//   - username: ${{ vars.REGISTRY_USERNAME }}
//   - password: ${{ secrets.REGISTRY_PASSWORD }}
//   - run: func deploy --registry=${{ vars.REGISTRY_LOGIN_URL }}/${{ vars.REGISTRY_USERNAME }} -v
func assertDefaultWorkflow(t *testing.T, actualGw string) {
	t.Helper()

	assertDefaultWorkflowWithBranch(t, actualGw, issueBranch)
}

func assertDefaultWorkflowWithBranch(t *testing.T, actualGw, branch string) {
	t.Helper()

	assert.Assert(t, yamlContains(actualGw, "Func Deploy"))
	assert.Assert(t, yamlContains(actualGw, "- "+branch))

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
	assert.Assert(t, yamlContains(actualGw, "functions-dev/action@main"))
	assert.Assert(t, yamlContains(actualGw, "version: knative-v1.20.1"))
	assert.Assert(t, yamlContains(actualGw, "name: func"))

	assert.Assert(t, yamlContains(actualGw, "Deploy function"))
	assert.Assert(t, yamlContains(actualGw, "func deploy --registry=${{ vars.REGISTRY_LOGIN_URL }}/${{ vars.REGISTRY_USERNAME }} -v"))
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

func assertCustomWorkflow(t *testing.T, actualGw string) {
	assert.Assert(t, yamlContains(actualGw, "Custom Deploy"))
	assert.Assert(t, yamlContains(actualGw, "self-hosted"))
	assert.Assert(t, yamlContains(actualGw, "DEV_CLUSTER_KUBECONFIG"))
	assert.Assert(t, yamlContains(actualGw, "DEV_REGISTRY_LOGIN_URL"))
	assert.Assert(t, yamlContains(actualGw, "DEV_REGISTRY_USER"))
	assert.Assert(t, yamlContains(actualGw, "DEV_REGISTRY_PASS"))
}

// ----------------------
// END: Testing Framework
