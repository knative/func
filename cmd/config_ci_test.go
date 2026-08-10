package cmd_test

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ory/viper"
	"gotest.tools/v3/assert"
	fnCmd "knative.dev/func/cmd"
	"knative.dev/func/cmd/common"
	"knative.dev/func/pkg/ci/github"
	fn "knative.dev/func/pkg/functions"
)

func TestNewConfigCICmd_RequiresFeatureFlag(t *testing.T) {
	opts := defaultOpts()
	opts.enableFeature = false

	result := runConfigCiCmd(t, opts)

	assert.ErrorContains(t, result.executeErr, "unknown command \"ci\" for \"config\"")
	assert.Equal(t, result.generatorWasInvoked, false)
}

func TestNewConfigCICmd_CISubcommandExist(t *testing.T) {
	// leave 'ci' to make this test explicitly use this subcommand
	opts := defaultOpts()
	opts.args = []string{"ci"}

	result := runConfigCiCmd(t, opts)

	assert.NilError(t, result.executeErr)
	assert.Equal(t, result.generatorWasInvoked, true)
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
			expectedWorkflowName: github.DefaultWorkflowName,
		},
		{
			name:                 "remote build uses remote default workflow name",
			args:                 []string{"--remote"},
			expectedWorkflowName: github.DefaultRemoteBuildWorkflowName,
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
			args:                 []string{"--workflow-name=" + github.DefaultWorkflowName, "--remote"},
			expectedWorkflowName: github.DefaultWorkflowName,
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
			assert.Equal(t, result.workflowConfig.WorkflowName, tc.expectedWorkflowName)
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
	assert.Equal(t, result.workflowConfig.WorkflowName, customWorkflowName)
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
		expectedPath string
	}{
		{
			name:         "empty path uses cwd",
			pathArg:      "",
			expectedPath: cwd,
		},
		{
			name:         "dot path uses cwd",
			pathArg:      "--path=.",
			expectedPath: cwd,
		},
		{
			name:         "explicit func path used as-is",
			pathArg:      "--path=" + explicitFuncPath,
			expectedPath: explicitFuncPath,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// GIVEN
			opts := defaultOpts()
			opts.args = append(opts.args, tc.pathArg)
			opts.withFakeGetCwdReturn.dir = cwd

			// WHEN
			result := runConfigCiCmd(t, opts)

			// THEN
			assert.NilError(t, result.executeErr)
			assert.Assert(t, strings.Contains(result.fnRoot, tc.expectedPath))
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
			assert.Equal(t, result.workflowConfig.Branch, tc.expectedBranch)
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
			expectedErr: fmt.Sprintf("platform must not be empty, supported: %s", github.DefaultPlatform),
		},
		{
			name:        "unsupported platform value",
			platformArg: "--platform=unsupported",
			expectedErr: fmt.Sprintf("unsupported support is not implemented, supported: %s", github.DefaultPlatform),
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

// START: Testing Framework
// ------------------------
const (
	mainBranch         = "main"
	issueBranch        = "issue-778-current-branch"
	customWorkflowName = "Deploy Checkout Service"
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

// defaultOpts returns test options for unit tests with sensible defaults:
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
	executeErr          error
	generatorWasInvoked bool
	workflowConfig      github.WorkflowConfig
	fnRoot              string
}

func runConfigCiCmd(
	t *testing.T,
	opts opts,
) result {
	t.Helper()

	// PRE-RUN PREP
	// all options for "func config ci" command
	if opts.enableFeature {
		t.Setenv(fnCmd.ConfigCIFeatureFlag, "true")
	}

	loaderSaver := common.NewMockLoaderSaver()
	loaderSaver.LoadFn = func(path string) (fn.Function, error) {
		return fn.Function{Root: path, Runtime: "go"}, nil
	}

	currentBranch := common.CurrentBranchStub(
		opts.withFakeGitCliReturn.output,
		opts.withFakeGitCliReturn.err,
	)

	workingDir := common.WorkDirStub(
		opts.withFakeGetCwdReturn.dir,
		opts.withFakeGetCwdReturn.err,
	)

	ciGeneratorFake := github.WorkflowGeneratorMock{}

	viper.Reset()

	cmd := fnCmd.NewConfigCmd(
		loaderSaver,
		github.NewBufferWriter(),
		currentBranch,
		workingDir,
		fnCmd.NewTestCIGeneratorFactory(&ciGeneratorFake),
		fnCmd.NewClient,
	)
	cmd.SetArgs(opts.args)

	// RUN
	err := cmd.Execute()

	// POST-RUN GATHER
	return result{
		err,
		ciGeneratorFake.WasInvoked,
		ciGeneratorFake.Config,
		ciGeneratorFake.FnRoot,
	}
}

// ----------------------
// END: Testing Framework
