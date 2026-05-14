package github_test

import (
	"bytes"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"gotest.tools/v3/assert"
	"gotest.tools/v3/assert/cmp"
	"knative.dev/func/pkg/ci/github"
	fn "knative.dev/func/pkg/functions"
)

// START: Broad Unit Tests
// -----------------------
func TestCIGenerator_WritesWorkflowFile(t *testing.T) {
	opts := defaultOpts()
	result := runGenerateWorkflow(t, opts)

	assert.NilError(t, result.executeErr)
	assert.Assert(t, result.gwYamlString != "")
}

func TestCIGenerator_WorkflowYAMLHasCorrectStructure(t *testing.T) {
	opts := defaultOpts()
	result := runGenerateWorkflow(t, opts)

	assert.NilError(t, result.executeErr)
	assertDefaultWorkflow(t, result.gwYamlString)
}

func TestCIGenerator_WorkflowYAMLHasCustomValues(t *testing.T) {
	// GIVEN
	opts := defaultOpts()
	opts.cfg.SelfHostedRunner = true
	opts.cfg.KubeconfigSecret = "DEV_CLUSTER_KUBECONFIG"
	opts.cfg.RegistryLoginUrlVar = "DEV_REGISTRY_LOGIN_URL"
	opts.cfg.RegistryUserVar = "DEV_REGISTRY_USER"
	opts.cfg.RegistryPassSecret = "DEV_REGISTRY_PASS"

	// WHEN
	result := runGenerateWorkflow(t, opts)

	// THEN
	assert.NilError(t, result.executeErr)
	assertCustomWorkflow(t, result.gwYamlString)
}

func TestCIGenerator_WorkflowHasNoRegistryLogin(t *testing.T) {
	// GIVEN
	opts := defaultOpts()
	opts.cfg.RegistryLogin = false

	// WHEN
	result := runGenerateWorkflow(t, opts)

	// THEN
	assert.NilError(t, result.executeErr)
	assert.Assert(t, !strings.Contains(result.gwYamlString, "docker/login-action@v3"))
	assert.Assert(t, !strings.Contains(result.gwYamlString, "Login to container registry"))
	assert.Assert(t, yamlContains(result.gwYamlString, "FUNC_REGISTRY: ${{ vars.REGISTRY_URL }}"))
}

func TestCIGenerator_RemoteBuildAndDeployWorkflow(t *testing.T) {
	// GIVEN
	opts := defaultOpts()
	opts.cfg.WorkflowName = "Remote Func Deploy"
	opts.cfg.RemoteBuild = true

	// WHEN
	result := runGenerateWorkflow(t, opts)

	// THEN
	assert.NilError(t, result.executeErr)
	assert.Assert(t, yamlContains(result.gwYamlString, "Remote Func Deploy"))
	assert.Assert(t, yamlContains(result.gwYamlString, `FUNC_REMOTE: "true"`))
}

func TestCIGenerator_HasWorkflowDispatch(t *testing.T) {
	// GIVEN
	opts := defaultOpts()
	opts.cfg.WorkflowDispatch = true

	// WHEN
	result := runGenerateWorkflow(t, opts)

	// THEN
	assert.NilError(t, result.executeErr)
	assert.Assert(t, yamlContains(result.gwYamlString, "workflow_dispatch"))
}

func TestCIGenerator_FunctionPath(t *testing.T) {
	t.Run("non-empty function path is accepted", func(t *testing.T) {
		// GIVEN
		opts := defaultOpts()
		opts.goFn.Root = filepath.Join("current-working-directory") // os-agnostic test path

		// WHEN
		result := runGenerateWorkflow(t, opts)

		// THEN
		assert.NilError(t, result.executeErr)
		assert.Assert(t, strings.Contains(result.actualPath, opts.goFn.Root))
	})

	t.Run("empty function path returns an error", func(t *testing.T) {
		// GIVEN
		expectedErr := fmt.Errorf("function root path can not be empty")
		opts := defaultOpts()
		opts.goFn.Root = "" // os-agnostic test path

		// WHEN
		result := runGenerateWorkflow(t, opts)

		// THEN
		assert.Error(t, result.executeErr, expectedErr.Error())
	})
}

func TestCIGenerator_WorkflowConfig(t *testing.T) {
	t.Run("applies all defaults when no config is provided", func(t *testing.T) {
		opts := defaultOpts()
		opts.cfg = nil

		result := runGenerateWorkflow(t, opts)

		assert.NilError(t, result.executeErr)
		assertDefaultWorkflow(t, result.gwYamlString)
	})

	t.Run("fills empty strings but cannot default boolean fields to true", func(t *testing.T) {
		opts := defaultOpts()
		opts.cfg = &github.WorkflowConfig{}

		result := runGenerateWorkflow(t, opts)

		assert.NilError(t, result.executeErr)
		assertSemiDefaultWorkflow(t, result.gwYamlString)
	})
}

func TestCIGenerator_ForceFlagOverwritesExistingWorkflow(t *testing.T) {
	workflowName := "Func Deploy"
	changedWorkflowName := "Sales Service Deployment"
	sharedWriter := github.NewBufferWriter()

	t.Run("initial workflow creation succeeds", func(t *testing.T) {
		opts := defaultOpts()
		opts.withWriter = sharedWriter

		result := runGenerateWorkflow(t, opts)

		assert.NilError(t, result.executeErr)
		assert.Assert(t, yamlContains(result.gwYamlString, workflowName))
		assert.Assert(t, !strings.Contains(result.stdOut, forceWarning))
	})

	t.Run("overwrite without force flag fails", func(t *testing.T) {
		opts := defaultOpts()
		opts.withWriter = sharedWriter
		opts.cfg.WorkflowName = changedWorkflowName

		result := runGenerateWorkflow(t, opts)

		assert.ErrorIs(t, result.executeErr, github.ErrWorkflowExists)
		assert.Assert(t, yamlContains(result.gwYamlString, workflowName))
		assert.Assert(t, !strings.Contains(result.gwYamlString, changedWorkflowName))
		assert.Assert(t, !strings.Contains(result.stdOut, forceWarning))
	})

	t.Run("overwrite with force flag succeeds and a warning message is printed to stdout", func(t *testing.T) {
		opts := defaultOpts()
		opts.withWriter = sharedWriter
		opts.cfg.WorkflowName = changedWorkflowName
		opts.cfg.Force = true

		result := runGenerateWorkflow(t, opts)

		assert.NilError(t, result.executeErr)
		assert.Assert(t, yamlContains(result.gwYamlString, changedWorkflowName))
		assert.Assert(t, !strings.Contains(result.gwYamlString, workflowName))
		assert.Assert(t, strings.Contains(result.stdOut, forceWarning))
	})
}

func TestCIGenerator_VerbosePrintsWorkflowDetails(t *testing.T) {
	t.Run("default configuration", func(t *testing.T) {
		opts := defaultOpts()
		opts.verbose = true

		expectedMessage := fmt.Sprintf(github.MainLayoutPlainText,
			defaultOutputPath,
			github.DefaultWorkflowName,
			github.DefaultBranch,
			"host",
			"disabled",
			"ubuntu-latest",
			"enabled",
			"enabled",
			"disabled",
			"disabled",
		) + fmt.Sprintf(github.RequireManyPlainText,
			"secrets."+github.DefaultKubeconfigSecretName,
			"secrets."+github.DefaultRegistryPassSecretName,
			"vars."+github.DefaultRegistryLoginUrlVariableName,
			"vars."+github.DefaultRegistryUserVariableName,
			"vars."+github.DefaultRegistryUrlVariableName,
		)

		result := runGenerateWorkflow(t, opts)

		assertMessage(t, result, expectedMessage)
	})

	t.Run("custom configuration", func(t *testing.T) {
		opts := defaultOpts()
		opts.verbose = true
		opts.cfg.SelfHostedRunner = true
		opts.cfg.WorkflowName = "Deploy Checkout Service"
		opts.cfg.RemoteBuild = true
		opts.cfg.TestStep = false
		opts.cfg.WorkflowDispatch = true
		opts.cfg.Force = true
		opts.cfg.KubeconfigSecret = "DEV_CLUSTER_KUBECONFIG"
		opts.cfg.RegistryPassSecret = "DEV_REGISTRY_PASS"
		opts.cfg.RegistryLoginUrlVar = "DEV_REGISTRY_LOGIN_URL"
		opts.cfg.RegistryUserVar = "DEV_REGISTRY_USER"
		opts.cfg.RegistryUrlVar = "DEV_REGISTRY_URL"

		expectedMessage := fmt.Sprintf(github.MainLayoutPlainText,
			defaultOutputPath,
			"Deploy Checkout Service",
			github.DefaultBranch,
			"pack",
			"enabled",
			"self-hosted",
			"disabled",
			"enabled",
			"enabled",
			"enabled",
		) + fmt.Sprintf(github.RequireManyPlainText,
			"secrets.DEV_CLUSTER_KUBECONFIG",
			"secrets.DEV_REGISTRY_PASS",
			"vars.DEV_REGISTRY_LOGIN_URL",
			"vars.DEV_REGISTRY_USER",
			"vars.DEV_REGISTRY_URL",
		)

		result := runGenerateWorkflow(t, opts)

		assertMessage(t, result, expectedMessage)
	})

	t.Run("without registry login", func(t *testing.T) {
		opts := defaultOpts()
		opts.verbose = true
		opts.cfg.RegistryLogin = false

		expectedMessage := fmt.Sprintf(github.MainLayoutPlainText,
			defaultOutputPath,
			github.DefaultWorkflowName,
			github.DefaultBranch,
			"host",
			"disabled",
			"ubuntu-latest",
			"enabled",
			"disabled",
			"disabled",
			"disabled",
		) + fmt.Sprintf(github.RequireOnePlainText,
			"secrets."+github.DefaultKubeconfigSecretName,
		)

		result := runGenerateWorkflow(t, opts)

		assertMessage(t, result, expectedMessage)
	})
}

func TestCIGenerator_PostExportMessageShown(t *testing.T) {
	t.Run("shows all secrets and variables for k8s and registry", func(t *testing.T) {
		opts := defaultOpts()

		expectedMessage := fmt.Sprintf(github.PostExportManyPlainText,
			defaultOutputPath,
			"secrets."+github.DefaultKubeconfigSecretName,
			"secrets."+github.DefaultRegistryPassSecretName,
			"vars."+github.DefaultRegistryLoginUrlVariableName,
			"vars."+github.DefaultRegistryUserVariableName,
			"vars."+github.DefaultRegistryUrlVariableName,
		)

		result := runGenerateWorkflow(t, opts)

		assertMessage(t, result, expectedMessage)
	})

	t.Run("shows only k8s secret when registry login is disabled", func(t *testing.T) {
		opts := defaultOpts()
		opts.cfg.RegistryLogin = false

		expectedMessage := fmt.Sprintf(github.PostExportOnePlainText,
			defaultOutputPath,
			"secrets."+github.DefaultKubeconfigSecretName,
		)

		result := runGenerateWorkflow(t, opts)

		assertMessage(t, result, expectedMessage)
	})
}

func TestCIGenerator_VerboseAndPostExportMessageAreMutuallyExclusive(t *testing.T) {
	t.Run("verbose shows configuration, not post-export message", func(t *testing.T) {
		opts := defaultOpts()
		opts.verbose = true

		result := runGenerateWorkflow(t, opts)

		assert.NilError(t, result.executeErr)
		assert.Assert(t, strings.Contains(result.stdOut, "GitHub Workflow Configuration"))
		assert.Assert(t, !strings.Contains(result.stdOut, "GitHub Workflow created at:"))
	})

	t.Run("non-verbose shows post-export message, not configuration", func(t *testing.T) {
		opts := defaultOpts()

		result := runGenerateWorkflow(t, opts)

		assert.NilError(t, result.executeErr)
		assert.Assert(t, strings.Contains(result.stdOut, "GitHub Workflow created at:"))
		assert.Assert(t, !strings.Contains(result.stdOut, "GitHub Workflow Configuration"))
	})
}

func TestCIGenerator_TestStepPerRuntime(t *testing.T) {
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
			opts.goFn.Runtime = tc.runtime

			// WHEN
			result := runGenerateWorkflow(t, opts)

			// THEN
			assert.NilError(t, result.executeErr)
			assert.Assert(t, yamlContains(result.gwYamlString, "Run tests"))
			assert.Assert(t, yamlContains(result.gwYamlString, tc.expectedRun))
		})
	}
}

func TestCIGenerator_TestStepSkipped(t *testing.T) {
	t.Run("unsupported runtime skips test step and prints warning", func(t *testing.T) {
		// GIVEN
		opts := defaultOpts()
		opts.goFn.Runtime = "rust"

		// WHEN
		result := runGenerateWorkflow(t, opts)

		// THEN
		assert.NilError(t, result.executeErr)
		assert.Assert(t, !strings.Contains(result.gwYamlString, "Run tests"))
		assert.Assert(t, strings.Contains(result.stdOut, "WARNING: test step not supported for runtime rust"))
	})

	t.Run("test step disabled via config", func(t *testing.T) {
		// GIVEN
		opts := defaultOpts()
		opts.cfg.TestStep = false

		// WHEN
		result := runGenerateWorkflow(t, opts)

		// THEN
		assert.NilError(t, result.executeErr)
		assert.Assert(t, !strings.Contains(result.gwYamlString, "Run tests"))
		assert.Assert(t, strings.Count(result.gwYamlString, "- name:") == 5)
	})
}

func TestCIGenerator_BuilderForRuntime(t *testing.T) {
	testCases := []struct {
		name    string
		runtime string
		builder string
		remote  bool
	}{
		{
			name:    "go function and local build",
			runtime: "go",
			builder: "host",
		},
		{
			name:    "go function and remote build",
			runtime: "go",
			builder: "pack",
			remote:  true,
		},
		{
			name:    "python function and local build",
			runtime: "python",
			builder: "host",
		},
		{
			name:    "python function and remote build",
			runtime: "python",
			builder: "s2i",
			remote:  true,
		},
		{
			name:    "node function and local build",
			runtime: "node",
			builder: "pack",
		},
		{
			name:    "node function and remote build",
			runtime: "node",
			builder: "pack",
			remote:  true,
		},
		{
			name:    "typescript function and local build",
			runtime: "typescript",
			builder: "pack",
		},
		{
			name:    "typescript function and remote build",
			runtime: "typescript",
			builder: "pack",
			remote:  true,
		},
		{
			name:    "rust function and local build",
			runtime: "rust",
			builder: "pack",
		},
		{
			name:    "rust function and remote build",
			runtime: "rust",
			builder: "pack",
			remote:  true,
		},
		{
			name:    "quarkus function and local build",
			runtime: "quarkus",
			builder: "pack",
		},
		{
			name:    "quarkus function and remote build",
			runtime: "quarkus",
			builder: "pack",
			remote:  true,
		},
		{
			name:    "springboot function and local build",
			runtime: "springboot",
			builder: "pack",
		},
		{
			name:    "springboot function and remote build",
			runtime: "springboot",
			builder: "pack",
			remote:  true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// GIVEN
			opts := defaultOpts()
			opts.goFn.Runtime = tc.runtime
			opts.cfg.RemoteBuild = tc.remote

			// WHEN
			result := runGenerateWorkflow(t, opts)

			// THEN
			assert.NilError(t, result.executeErr)
			assert.Assert(t, strings.Contains(result.gwYamlString, "FUNC_BUILDER: "+tc.builder))
		})
	}
}

func TestCIGenerator_BuilderForRuntimeError(t *testing.T) {
	// GIVEN
	opts := defaultOpts()
	opts.goFn.Runtime = "zig"

	// WHEN
	result := runGenerateWorkflow(t, opts)

	// THEN
	assert.Error(t, result.executeErr, "no builder support for runtime: zig")
}

// ---------------------
// END: Broad Unit Tests

// START: Testing Framework
// ------------------------
const (
	forceWarning = "WARNING: --force flag is set, overwriting existing GitHub Workflow file"
)

var defaultOutputPath = filepath.Join(github.DefaultGitHubWorkflowDir, github.DefaultGitHubWorkflowFilename)

type result struct {
	actualPath   string
	executeErr   error
	gwYamlString string
	stdOut       string
}

type opts struct {
	cfg        *github.WorkflowConfig
	goFn       fn.Function
	withWriter *github.BufferWriter
	verbose    bool
}

func defaultOpts() opts {
	cfg := &github.WorkflowConfig{
		GithubWorkflowDir:      github.DefaultGitHubWorkflowDir,
		GithubWorkflowFilename: github.DefaultGitHubWorkflowFilename,
		Branch:                 github.DefaultBranch,
		WorkflowName:           github.DefaultWorkflowName,
		KubeconfigSecret:       github.DefaultKubeconfigSecretName,
		RegistryLoginUrlVar:    github.DefaultRegistryLoginUrlVariableName,
		RegistryUserVar:        github.DefaultRegistryUserVariableName,
		RegistryPassSecret:     github.DefaultRegistryPassSecretName,
		RegistryUrlVar:         github.DefaultRegistryUrlVariableName,
		RegistryLogin:          github.DefaultRegistryLogin,
		SelfHostedRunner:       github.DefaultSelfHostedRunner,
		RemoteBuild:            github.DefaultRemoteBuild,
		WorkflowDispatch:       github.DefaultWorkflowDispatch,
		TestStep:               github.DefaultTestStep,
		Force:                  github.DefaultForce,
	}
	goFn := fn.Function{Root: "path/to/func", Runtime: "go"}
	return opts{
		cfg:  cfg,
		goFn: goFn,
	}
}

func runGenerateWorkflow(t *testing.T, opts opts) result {
	t.Helper()

	writer := opts.withWriter
	if writer == nil {
		writer = github.NewBufferWriter()
	}

	messageBufferWriter := &bytes.Buffer{}

	generatorOptions := []github.Option{
		github.WithWorkflowWriter(writer),
		github.WithMessageWriter(messageBufferWriter),
		github.WithVerbose(opts.verbose),
	}

	if opts.cfg != nil {
		generatorOptions = append(generatorOptions, github.WithWorkflowConfig(*opts.cfg))
	}

	generator := github.NewWorkflowGenerator(generatorOptions...)
	err := generator.Generate(t.Context(), opts.goFn)

	return result{
		writer.Path,
		err,
		writer.Buffer.String(),
		messageBufferWriter.String(),
	}
}

// assertDefaultWorkflow verifies the generated YAML contains the full
// default workflow structure: all six steps, default branch, runner,
// secrets, variables, and deploy command.
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

// assertSemiDefaultWorkflow verifies the workflow generated from an empty
// WorkflowConfig{}. String fields are backfilled with defaults, but boolean
// fields stay at zero (false), so RegistryLogin and TestStep are off —
// resulting in four steps instead of six and no registry login action.
func assertSemiDefaultWorkflow(t *testing.T, actualGw string) {
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
	assert.Assert(t, yamlContains(actualGw, "functions-dev/action@main"))
	assert.Assert(t, yamlContains(actualGw, "version: knative-v1.22.0"))
	assert.Assert(t, yamlContains(actualGw, "name: func"))

	assert.Assert(t, yamlContains(actualGw, "Deploy function"))
	assert.Assert(t, yamlContains(actualGw, `FUNC_VERBOSE: "true"`))
	assert.Assert(t, yamlContains(actualGw, "FUNC_REGISTRY: ${{ vars.REGISTRY_URL }}"))
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
