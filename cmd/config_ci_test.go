package cmd_test

import (
	"bytes"
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

func TestNewConfigCICmd_WorkflowNameResolution(t *testing.T) {
	testCases := []struct {
		name                 string
		args                 []string
		expectedWorkflowName string
	}{
		{
			name:                 "default workflow name when no flags",
			args:                 nil,
			expectedWorkflowName: ci.DefaultWorkflowName,
		},
		{
			name:                 "remote build uses remote default workflow name",
			args:                 []string{"--remote"},
			expectedWorkflowName: ci.DefaultRemoteBuildWorkflowName,
		},
		{
			name:                 "custom name is preserved without remote",
			args:                 []string{"--workflow-name=" + customWorkflowName},
			expectedWorkflowName: customWorkflowName,
		},
		{
			name:                 "custom name is preserved with remote",
			args:                 []string{"--workflow-name=" + customWorkflowName, "--remote"},
			expectedWorkflowName: customWorkflowName,
		},
		{
			name:                 "custom name is preserved if its equal to default workflow name and remote is set",
			args:                 []string{"--workflow-name=" + ci.DefaultWorkflowName, "--remote"},
			expectedWorkflowName: ci.DefaultWorkflowName,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// GIVEN
			opts := defaultOpts()
			opts.args = append(opts.args, tc.args...)

			// WHEN
			result := runConfigCiCmd(t, opts)

			// THEN
			assert.NilError(t, result.executeErr)
			assert.Assert(t, yamlContains(result.gwYamlString, "name: "+tc.expectedWorkflowName))
		})
	}
}

func TestNewConfigCICmd_WorkflowNameFromEnvVarPreserveWithRemote(t *testing.T) {
	// Regression test: env var FUNC_WORKFLOW_NAME was ignored when --remote was set,
	// because only cmd.Flags().Changed() was checked (not viper.IsSet())
	opts := defaultOpts()
	opts.args = append(opts.args, "--remote")
	t.Setenv("FUNC_WORKFLOW_NAME", customWorkflowName)

	// WHEN
	result := runConfigCiCmd(t, opts)

	// THEN
	assert.NilError(t, result.executeErr)
	assert.Assert(t, yamlContains(result.gwYamlString, "name: "+customWorkflowName))
}

func TestNewConfigCICmd_WorkflowHasNoRegistryLogin(t *testing.T) {
	// GIVEN
	opts := defaultOpts()
	opts.args = append(opts.args, "--registry-login=false")

	// WHEN
	result := runConfigCiCmd(t, opts)

	// THEN
	assert.NilError(t, result.executeErr)
	assert.Assert(t, !strings.Contains(result.gwYamlString, "docker/login-action@v3"))
	assert.Assert(t, !strings.Contains(result.gwYamlString, "Login to container registry"))
	assert.Assert(t, yamlContains(result.gwYamlString, "FUNC_REGISTRY: ${{ vars.REGISTRY_URL }}"))
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
	assert.Assert(t, yamlContains(result.gwYamlString, `FUNC_REMOTE: "true"`))
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

func TestNewConfigCICmd_GithubPlatformFlagSupported(t *testing.T) {
	testCases := []struct {
		name        string
		platformArg string
	}{
		{
			name:        "empty value picks GitHub CI/CD platform as default",
			platformArg: "",
		},
		{
			name:        "GitHub value is supported",
			platformArg: "--platform=github",
		},
		{
			name:        "GitHub value is case insensitive",
			platformArg: "--platform=GitHub",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// GIVEN
			opts := defaultOpts()
			opts.args = append(opts.args, tc.platformArg)

			// WHEN
			result := runConfigCiCmd(t, opts)

			// THEN
			assert.NilError(t, result.executeErr)
		})
	}
}

func TestNewConfigCICmd_PlatformFlagErrors(t *testing.T) {
	testCases := []struct {
		name        string
		platformArg string
		expectedErr string
	}{
		{
			name:        "empty platform value",
			platformArg: "--platform=",
			expectedErr: fmt.Sprintf("platform must not be empty, supported: %s", ci.DefaultPlatform),
		},
		{
			name:        "unsupported platform value",
			platformArg: "--platform=unsupported",
			expectedErr: fmt.Sprintf("unsupported support is not implemented, supported: %s", ci.DefaultPlatform),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// GIVEN
			opts := defaultOpts()
			opts.args = append(opts.args, tc.platformArg)

			// WHEN
			result := runConfigCiCmd(t, opts)

			// THEN
			assert.Error(t, result.executeErr, tc.expectedErr)
		})
	}
}

func TestNewConfigCICmd_ForceFlagOverwritesExistingWorkflow(t *testing.T) {
	workflowName := "Func Deploy"
	changedWorkflowName := "Sales Service Deployment"
	sharedWriter := ci.NewBufferWriter()

	t.Run("initial workflow creation succeeds", func(t *testing.T) {
		opts := defaultOpts()
		opts.withWriter = sharedWriter

		result := runConfigCiCmd(t, opts)

		assert.NilError(t, result.executeErr)
		assert.Assert(t, yamlContains(result.gwYamlString, workflowName))
		assert.Assert(t, !strings.Contains(result.stdOut, forceWarning))
	})

	t.Run("overwrite without force flag fails", func(t *testing.T) {
		opts := defaultOpts()
		opts.withWriter = sharedWriter
		opts.args = append(opts.args, "--workflow-name="+changedWorkflowName)

		result := runConfigCiCmd(t, opts)

		assert.ErrorIs(t, result.executeErr, ci.ErrWorkflowExists)
		assert.Assert(t, yamlContains(result.gwYamlString, workflowName))
		assert.Assert(t, !strings.Contains(result.gwYamlString, changedWorkflowName))
		assert.Assert(t, !strings.Contains(result.stdOut, forceWarning))
	})

	t.Run("overwrite with force flag succeeds and a warning message is printed to stdout", func(t *testing.T) {
		opts := defaultOpts()
		opts.withWriter = sharedWriter
		opts.args = append(opts.args, "--workflow-name="+changedWorkflowName, "--force")

		result := runConfigCiCmd(t, opts)

		assert.NilError(t, result.executeErr)
		assert.Assert(t, yamlContains(result.gwYamlString, changedWorkflowName))
		assert.Assert(t, !strings.Contains(result.gwYamlString, workflowName))
		assert.Assert(t, strings.Contains(result.stdOut, forceWarning))
	})
}

func TestNewConfigCICmd_VerboseFlagPrintsWorkflowDetails(t *testing.T) {
	t.Run("verbose flag prints default Github Workflow configuration", func(t *testing.T) {
		opts := defaultOpts()
		opts.args = append(opts.args, "--verbose")
		expectedMessage := fmt.Sprintf(ci.MainLayoutPlainText,
			defaultOutputPath,
			ci.DefaultWorkflowName,
			issueBranch,
			"host",
			"disabled",
			"ubuntu-latest",
			"enabled",
			"enabled",
			"disabled",
			"disabled",
		) + fmt.Sprintf(ci.RequireManyPlainText,
			"secrets."+ci.DefaultKubeconfigSecretName,
			"secrets."+ci.DefaultRegistryPassSecretName,
			"vars."+ci.DefaultRegistryLoginUrlVariableName,
			"vars."+ci.DefaultRegistryUserVariableName,
			"vars."+ci.DefaultRegistryUrlVariableName,
		)

		result := runConfigCiCmd(t, opts)

		assertMessage(t, result, expectedMessage)
	})

	t.Run("verbose flag prints custom Github Workflow configuration", func(t *testing.T) {
		opts := defaultOpts()
		opts.args = append(opts.args,
			"--verbose",
			"--self-hosted-runner",
			"--workflow-name=Deploy Checkout Service",
			"--remote",
			"--test-step=false",
			"--workflow-dispatch",
			"--force",
			"--kubeconfig-secret-name=DEV_CLUSTER_KUBECONFIG",
			"--registry-pass-secret-name=DEV_REGISTRY_PASS",
			"--registry-login-url-variable-name=DEV_REGISTRY_LOGIN_URL",
			"--registry-user-variable-name=DEV_REGISTRY_USER",
			"--registry-url-variable-name=DEV_REGISTRY_URL",
		)
		expectedMessage := fmt.Sprintf(ci.MainLayoutPlainText,
			defaultOutputPath,
			customWorkflowName,
			issueBranch,
			"pack",
			"enabled",
			"self-hosted",
			"disabled",
			"enabled",
			"enabled",
			"enabled",
		) + fmt.Sprintf(ci.RequireManyPlainText,
			"secrets.DEV_CLUSTER_KUBECONFIG",
			"secrets.DEV_REGISTRY_PASS",
			"vars.DEV_REGISTRY_LOGIN_URL",
			"vars.DEV_REGISTRY_USER",
			"vars.DEV_REGISTRY_URL",
		)

		result := runConfigCiCmd(t, opts)

		assertMessage(t, result, expectedMessage)
	})

	t.Run("verbose flag prints custom Github Workflow configuration without registry login", func(t *testing.T) {
		opts := defaultOpts()
		opts.args = append(opts.args,
			"--verbose",
			"--registry-login=false",
		)
		expectedMessage := fmt.Sprintf(ci.MainLayoutPlainText,
			defaultOutputPath,
			ci.DefaultWorkflowName,
			issueBranch,
			"host",
			"disabled",
			"ubuntu-latest",
			"enabled",
			"disabled",
			"disabled",
			"disabled",
		) + fmt.Sprintf(ci.RequireOnePlainText,
			"secrets."+ci.DefaultKubeconfigSecretName,
		)

		result := runConfigCiCmd(t, opts)

		assertMessage(t, result, expectedMessage)
	})
}

func TestNewConfigCICmd_PostExportMessageShown(t *testing.T) {
	t.Run("a message is shown with all secrets and variables for k8 and registry which needs creation", func(t *testing.T) {
		opts := defaultOpts()
		expectedMessage := fmt.Sprintf(ci.PostExportManyPlainText,
			defaultOutputPath,
			"secrets."+ci.DefaultKubeconfigSecretName,
			"secrets."+ci.DefaultRegistryPassSecretName,
			"vars."+ci.DefaultRegistryLoginUrlVariableName,
			"vars."+ci.DefaultRegistryUserVariableName,
			"vars."+ci.DefaultRegistryUrlVariableName,
		)

		result := runConfigCiCmd(t, opts)

		assertMessage(t, result, expectedMessage)
	})

	t.Run("a message is shown with a secret for k8 which needs creation", func(t *testing.T) {
		opts := defaultOpts()
		opts.args = append(opts.args, "--registry-login=false")
		expectedMessage := fmt.Sprintf(ci.PostExportOnePlainText,
			defaultOutputPath,
			"secrets."+ci.DefaultKubeconfigSecretName,
		)

		result := runConfigCiCmd(t, opts)

		assertMessage(t, result, expectedMessage)
	})
}

func TestNewConfigCICmd_VerboseAndPostExportMessageAreMutuallyExclusive(t *testing.T) {
	t.Run("verbose flag shows configuration, not post-export message", func(t *testing.T) {
		opts := defaultOpts()
		opts.args = append(opts.args, "--verbose")

		result := runConfigCiCmd(t, opts)

		assert.NilError(t, result.executeErr)
		assert.Assert(t, strings.Contains(result.stdOut, "GitHub Workflow Configuration"))
		assert.Assert(t, !strings.Contains(result.stdOut, "GitHub Workflow created at:"))
	})

	t.Run("without verbose flag shows post-export message, not configuration", func(t *testing.T) {
		opts := defaultOpts()

		result := runConfigCiCmd(t, opts)

		assert.NilError(t, result.executeErr)
		assert.Assert(t, strings.Contains(result.stdOut, "GitHub Workflow created at:"))
		assert.Assert(t, !strings.Contains(result.stdOut, "GitHub Workflow Configuration"))
	})
}

func TestNewConfigCICmd_TestStepPerRuntime(t *testing.T) {
	testCases := []struct {
		name        string
		runtime     string
		expectedRun string
	}{
		{
			name:        "go runtime adds go test step",
			runtime:     "go",
			expectedRun: "go test ./...",
		},
		{
			name:        "nodejs runtime adds npm test step",
			runtime:     "node",
			expectedRun: "npm ci && npm test",
		},
		{
			name:        "typescript runtime adds npm test step",
			runtime:     "typescript",
			expectedRun: "npm ci && npm test",
		},
		{
			name:        "python runtime adds python -m pytest step",
			runtime:     "python",
			expectedRun: "pip install . && python -m pytest",
		},
		{
			name:        "quarkus runtime adds mvnw test step",
			runtime:     "quarkus",
			expectedRun: "./mvnw test",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// GIVEN
			opts := defaultOpts()
			opts.runtime = tc.runtime

			// WHEN
			result := runConfigCiCmd(t, opts)

			// THEN
			assert.NilError(t, result.executeErr)
			assert.Assert(t, yamlContains(result.gwYamlString, runTestStepName))
			assert.Assert(t, yamlContains(result.gwYamlString, tc.expectedRun))
		})
	}
}

func TestNewConfigCICmd_TestStepSkipped(t *testing.T) {
	t.Run("unsupported runtime skips test step and prints warning", func(t *testing.T) {
		// GIVEN
		opts := defaultOpts()
		opts.runtime = "rust"

		// WHEN
		result := runConfigCiCmd(t, opts)

		// THEN
		assert.NilError(t, result.executeErr)
		assert.Assert(t, !strings.Contains(result.gwYamlString, runTestStepName))
		assert.Assert(t, strings.Contains(result.stdOut, "WARNING: test step not supported for runtime rust"))
	})

	t.Run("test step disabled via flag", func(t *testing.T) {
		// GIVEN
		opts := defaultOpts()
		opts.args = append(opts.args, "--test-step=false")

		// WHEN
		result := runConfigCiCmd(t, opts)

		// THEN
		assert.NilError(t, result.executeErr)
		assert.Assert(t, !strings.Contains(result.gwYamlString, runTestStepName))
		assert.Assert(t, strings.Count(result.gwYamlString, "- name:") == 5)
	})
}

func TestNewConfigCICmd_BuilderForRuntime(t *testing.T) {
	testCases := []struct {
		name,
		runtime,
		builder,
		args string
	}{
		{
			name:    "go function and local build",
			args:    "",
			runtime: "go",
			builder: "host",
		},
		{
			name:    "go function and remote build",
			args:    "--remote",
			runtime: "go",
			builder: "pack",
		},
		{
			name:    "python function and local build",
			args:    "",
			runtime: "python",
			builder: "host",
		},
		{
			name:    "python function and remote build",
			args:    "--remote",
			runtime: "python",
			builder: "s2i",
		},
		{
			name:    "node function and local build",
			args:    "",
			runtime: "node",
			builder: "pack",
		},
		{
			name:    "node function and remote build",
			args:    "--remote",
			runtime: "node",
			builder: "pack",
		},
		{
			name:    "typescript function and local build",
			args:    "",
			runtime: "typescript",
			builder: "pack",
		},
		{
			name:    "typescript function and remote build",
			args:    "--remote",
			runtime: "typescript",
			builder: "pack",
		},
		{
			name:    "rust function and local build",
			args:    "",
			runtime: "rust",
			builder: "pack",
		},
		{
			name:    "rust function and remote build",
			args:    "--remote",
			runtime: "rust",
			builder: "pack",
		},
		{
			name:    "quarkus function and local build",
			args:    "",
			runtime: "quarkus",
			builder: "pack",
		},
		{
			name:    "quarkus function and remote build",
			args:    "--remote",
			runtime: "quarkus",
			builder: "pack",
		},
		{
			name:    "springboot function and local build",
			args:    "",
			runtime: "springboot",
			builder: "pack",
		},
		{
			name:    "springboot function and remote build",
			args:    "--remote",
			runtime: "springboot",
			builder: "pack",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// GIVEN
			opts := defaultOpts()
			opts.runtime = tc.runtime
			opts.args = append(opts.args, tc.args)

			// WHEN
			result := runConfigCiCmd(t, opts)

			// THEN
			assert.NilError(t, result.executeErr)
			assert.Assert(t, strings.Contains(result.gwYamlString, "FUNC_BUILDER: "+tc.builder))
		})
	}
}

func TestNewConfigCICmd_BuilderForRuntimeError(t *testing.T) {
	// GIVEN
	opts := defaultOpts()
	opts.runtime = "zig"
	expectedErr := fmt.Errorf("no builder support for runtime: %s", opts.runtime)

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
	mainBranch         = "main"
	issueBranch        = "issue-778-current-branch"
	fnName             = "github-ci-func"
	forceWarning       = "WARNING: --force flag is set, overwriting existing GitHub Workflow file"
	customWorkflowName = "Deploy Checkout Service"
	runTestStepName    = "Run tests"
)

var defaultOutputPath = filepath.Join(ci.DefaultGitHubWorkflowDir, ci.DefaultGitHubWorkflowFilename)

type opts struct {
	enableFeature        bool
	runtime              string
	withFakeGitCliReturn struct {
		output string
		err    error
	}
	withFakeGetCwdReturn struct {
		dir string
		err error
	}
	withWriter *ci.BufferWriter
	args       []string
}

// defaultOpts returns test options for broad unit tests with sensible defaults:
//   - enableFeature:        true
//   - withFakeGitCliReturn: {output: issueBranch, err: nil}
//   - withFakeGetCwdReturn: {dir: "", err: nil}
//   - args:                 []string{"ci"}
func defaultOpts() opts {
	return opts{
		enableFeature: true,
		runtime:       "go",
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
		withWriter: nil,
		args:       []string{"ci"},
	}
}

type result struct {
	executeErr error
	gwYamlString,
	actualPath,
	stdOut string
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
		return fn.Function{Root: path, Runtime: opts.runtime}, nil
	}

	writer := opts.withWriter
	if writer == nil {
		writer = ci.NewBufferWriter()
	}

	currentBranch := common.CurrentBranchStub(
		opts.withFakeGitCliReturn.output,
		opts.withFakeGitCliReturn.err,
	)

	workingDir := common.WorkDirStub(
		opts.withFakeGetCwdReturn.dir,
		opts.withFakeGetCwdReturn.err,
	)

	messageBufferWriter := &bytes.Buffer{}

	viper.Reset()

	cmd := fnCmd.NewConfigCmd(
		loaderSaver,
		writer,
		currentBranch,
		workingDir,
		fnCmd.NewClient,
	)
	cmd.SetArgs(opts.args)
	cmd.SetOut(messageBufferWriter)

	// RUN
	err := cmd.Execute()

	// POST-RUN GATHER
	return result{
		err,
		writer.Buffer.String(),
		writer.Path,
		messageBufferWriter.String(),
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
	assert.Assert(t, yamlContains(actualGw, "version: knative-v1.21.0"))
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

func assertCustomWorkflow(t *testing.T, actualGw string) {
	t.Helper()

	assert.Assert(t, yamlContains(actualGw, "self-hosted"))
	assert.Assert(t, yamlContains(actualGw, "DEV_CLUSTER_KUBECONFIG"))
	assert.Assert(t, yamlContains(actualGw, "DEV_REGISTRY_LOGIN_URL"))
	assert.Assert(t, yamlContains(actualGw, "DEV_REGISTRY_USER"))
	assert.Assert(t, yamlContains(actualGw, "DEV_REGISTRY_PASS"))
}

func assertMessage(t *testing.T, res result, expectedMessage string) {
	t.Helper()

	assert.NilError(t, res.executeErr)
	assert.Assert(t, strings.Contains(res.stdOut, expectedMessage),
		"\nexpected:\n%s\n\ngot:\n%s", expectedMessage, res.stdOut)
}

// ----------------------
// END: Testing Framework
