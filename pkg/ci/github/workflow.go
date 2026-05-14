package github

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// ErrWorkflowExists is returned when a GitHub workflow file already exists and --force is not specified.
var ErrWorkflowExists = errors.New("existing GitHub workflow detected, overwrite using the --force option")

const (
	defaultFuncCliVersion               = "knative-v1.22.0"
	DefaultPlatform                     = "github"
	DefaultGitHubWorkflowDir            = ".github/workflows"
	DefaultGitHubWorkflowFilename       = "func-deploy.yaml"
	DefaultBranch                       = "main"
	DefaultWorkflowName                 = "Func Deploy"
	DefaultRemoteBuildWorkflowName      = "Remote " + DefaultWorkflowName
	DefaultKubeconfigSecretName         = "KUBECONFIG"
	DefaultRegistryLoginUrlVariableName = "REGISTRY_LOGIN_URL"
	DefaultRegistryUserVariableName     = "REGISTRY_USERNAME"
	DefaultRegistryPassSecretName       = "REGISTRY_PASSWORD"
	DefaultRegistryUrlVariableName      = "REGISTRY_URL"
	DefaultRegistryLogin                = true
	DefaultWorkflowDispatch             = false
	DefaultRemoteBuild                  = false
	DefaultSelfHostedRunner             = false
	DefaultTestStep                     = true
	DefaultForce                        = false
	DefaultVerbose                      = false
)

// WorkflowConfig holds the settings for generating a GitHub Actions workflow.
type WorkflowConfig struct {
	GithubWorkflowDir,
	GithubWorkflowFilename,
	Branch,
	WorkflowName,
	KubeconfigSecret,
	RegistryLoginUrlVar,
	RegistryUserVar,
	RegistryPassSecret,
	RegistryUrlVar string
	RegistryLogin,
	SelfHostedRunner,
	RemoteBuild,
	WorkflowDispatch,
	TestStep,
	Force bool
}

func (o WorkflowConfig) fnGitHubWorkflowFilepath(fnRoot string) string {
	fnGitHubWorkflowDir := filepath.Join(fnRoot, o.GithubWorkflowDir)
	return filepath.Join(fnGitHubWorkflowDir, o.GithubWorkflowFilename)
}

func (o WorkflowConfig) outputPath() string {
	return filepath.Join(o.GithubWorkflowDir, o.GithubWorkflowFilename)
}

func defaultWorkflowConfig() WorkflowConfig {
	return WorkflowConfig{
		GithubWorkflowDir:      DefaultGitHubWorkflowDir,
		GithubWorkflowFilename: DefaultGitHubWorkflowFilename,
		Branch:                 DefaultBranch,
		WorkflowName:           DefaultWorkflowName,
		KubeconfigSecret:       DefaultKubeconfigSecretName,
		RegistryLoginUrlVar:    DefaultRegistryLoginUrlVariableName,
		RegistryUserVar:        DefaultRegistryUserVariableName,
		RegistryPassSecret:     DefaultRegistryPassSecretName,
		RegistryUrlVar:         DefaultRegistryUrlVariableName,
		RegistryLogin:          DefaultRegistryLogin,
		SelfHostedRunner:       DefaultSelfHostedRunner,
		RemoteBuild:            DefaultRemoteBuild,
		WorkflowDispatch:       DefaultWorkflowDispatch,
		TestStep:               DefaultTestStep,
		Force:                  DefaultForce,
	}
}

func setEmptyFieldsToDefaults(defaults WorkflowConfig) WorkflowConfig {
	if defaults.GithubWorkflowDir == "" {
		defaults.GithubWorkflowDir = DefaultGitHubWorkflowDir
	}
	if defaults.GithubWorkflowFilename == "" {
		defaults.GithubWorkflowFilename = DefaultGitHubWorkflowFilename
	}
	if defaults.Branch == "" {
		defaults.Branch = DefaultBranch
	}
	if defaults.WorkflowName == "" {
		defaults.WorkflowName = DefaultWorkflowName
	}
	if defaults.KubeconfigSecret == "" {
		defaults.KubeconfigSecret = DefaultKubeconfigSecretName
	}
	if defaults.RegistryLoginUrlVar == "" {
		defaults.RegistryLoginUrlVar = DefaultRegistryLoginUrlVariableName
	}
	if defaults.RegistryUserVar == "" {
		defaults.RegistryUserVar = DefaultRegistryUserVariableName
	}
	if defaults.RegistryPassSecret == "" {
		defaults.RegistryPassSecret = DefaultRegistryPassSecretName
	}
	if defaults.RegistryUrlVar == "" {
		defaults.RegistryUrlVar = DefaultRegistryUrlVariableName
	}

	return defaults
}

type workflow struct {
	Name string           `yaml:"name"`
	On   workflowTriggers `yaml:"on"`
	Jobs map[string]job   `yaml:"jobs"`
}

type workflowTriggers struct {
	Push             *pushTrigger `yaml:"push,omitempty"`
	WorkflowDispatch *struct{}    `yaml:"workflow_dispatch,omitempty"`
}

type pushTrigger struct {
	Branches []string `yaml:"branches,omitempty"`
}

type job struct {
	RunsOn string `yaml:"runs-on"`
	Steps  []step `yaml:"steps"`
}

type step struct {
	Name string            `yaml:"name,omitempty"`
	Env  map[string]string `yaml:"env,omitempty"`
	Uses string            `yaml:"uses,omitempty"`
	Run  string            `yaml:"run,omitempty"`
	With map[string]string `yaml:"with,omitempty"`
}

func newGitHubWorkflow(cfg WorkflowConfig, runtime string, messageWriter io.Writer) (*workflow, error) {
	var steps []step
	steps = createCheckoutStep(steps)
	steps = createRuntimeTestStep(cfg, runtime, messageWriter, steps)
	steps = createK8ContextStep(cfg, steps)
	steps = createRegistryLoginStep(cfg, steps)
	steps = createFuncCLIInstallStep(steps)

	steps, err := createFuncDeployStep(cfg, runtime, steps)
	if err != nil {
		return nil, err
	}

	return &workflow{
		Name: cfg.WorkflowName,
		On:   createPushTrigger(cfg),
		Jobs: map[string]job{
			"deploy": {
				RunsOn: determineRunner(cfg.SelfHostedRunner),
				Steps:  steps,
			},
		},
	}, nil
}

func createCheckoutStep(steps []step) []step {
	checkoutCode := newStep("Checkout code").
		withUses("actions/checkout@v4")

	return append(steps, *checkoutCode)
}

func createRuntimeTestStep(opts WorkflowConfig, runtime string, messageWriter io.Writer, steps []step) []step {
	if !opts.TestStep {
		return steps
	}

	testStep := newStep("Run tests")

	switch runtime {
	case "go":
		testStep.withRun("go test ./...")
	case "node", "typescript":
		testStep.withRun("npm ci && npm test")
	case "python":
		testStep.withRun("pip install . && python -m pytest")
	case "quarkus":
		testStep.withRun("./mvnw test")
	default:
		// best-effort user message; errors are non-critical
		_, _ = fmt.Fprintf(messageWriter, "WARNING: test step not supported for runtime %s\n", runtime)
		return steps
	}

	return append(steps, *testStep)
}

func createK8ContextStep(opts WorkflowConfig, steps []step) []step {
	setupK8Context := newStep("Setup Kubernetes context").
		withUses("azure/k8s-set-context@v4").
		withActionConfig("method", "kubeconfig").
		withActionConfig("kubeconfig", newSecret(opts.KubeconfigSecret))

	return append(steps, *setupK8Context)
}

func createRegistryLoginStep(opts WorkflowConfig, steps []step) []step {
	if !opts.RegistryLogin {
		return steps
	}

	loginToContainerRegistry := newStep("Login to container registry").
		withUses("docker/login-action@v3").
		withActionConfig("registry", newVariable(opts.RegistryLoginUrlVar)).
		withActionConfig("username", newVariable(opts.RegistryUserVar)).
		withActionConfig("password", newSecret(opts.RegistryPassSecret))

	return append(steps, *loginToContainerRegistry)
}

func createFuncCLIInstallStep(steps []step) []step {
	installFuncCli := newStep("Install func cli").
		withUses("functions-dev/action@main").
		withActionConfig("version", defaultFuncCliVersion).
		withActionConfig("name", "func")

	return append(steps, *installFuncCli)
}

func createFuncDeployStep(opts WorkflowConfig, runtime string, steps []step) ([]step, error) {
	deployFuncStep := newStep("Deploy function").
		withEnv("FUNC_VERBOSE", "true")

	builder, err := determineBuilder(runtime, opts.RemoteBuild)
	if err != nil {
		return nil, err
	}
	deployFuncStep.withEnv("FUNC_BUILDER", builder)

	if opts.RemoteBuild {
		deployFuncStep.withEnv("FUNC_REMOTE", "true")
	}

	registryUrl := newVariable(opts.RegistryUrlVar)
	if opts.RegistryLogin {
		registryUrl = newVariable(opts.RegistryLoginUrlVar) + "/" + newVariable(opts.RegistryUserVar)
	}
	deployFuncStep.withEnv("FUNC_REGISTRY", registryUrl).
		withRun("func deploy")

	return append(steps, *deployFuncStep), nil
}

func createPushTrigger(opts WorkflowConfig) workflowTriggers {
	result := workflowTriggers{
		Push: &pushTrigger{Branches: []string{opts.Branch}},
	}

	if opts.WorkflowDispatch {
		result.WorkflowDispatch = &struct{}{}
	}

	return result
}

func newStep(name string) *step {
	return &step{Name: name}
}

func (s *step) withUses(u string) *step {
	s.Uses = u
	return s
}

func (s *step) withRun(r string) *step {
	s.Run = r
	return s
}

func (s *step) withActionConfig(key, value string) *step {
	if s.With == nil {
		s.With = make(map[string]string)
	}

	s.With[key] = value

	return s
}

func (s *step) withEnv(key, value string) *step {
	if s.Env == nil {
		s.Env = make(map[string]string)
	}

	s.Env[key] = value

	return s
}

func (gw *workflow) Export(path string, w WorkflowWriter, force bool, m io.Writer) error {
	if !force && w.Exist(path) {
		return ErrWorkflowExists
	}

	if w.Exist(path) {
		// best-effort user message; errors are non-critical
		_, _ = fmt.Fprintf(m, "WARNING: --force flag is set, overwriting existing GitHub Workflow file\n")
	}

	raw, err := gw.toYaml()
	if err != nil {
		return err
	}

	return w.Write(path, raw)
}

func (gw *workflow) toYaml() ([]byte, error) {
	var buf bytes.Buffer
	encoder := yaml.NewEncoder(&buf)
	encoder.SetIndent(2)

	if err := encoder.Encode(gw); err != nil {
		return nil, err
	}
	encoder.Close()

	return buf.Bytes(), nil
}
