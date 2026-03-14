package ci

import (
	"bytes"
	"errors"
	"fmt"
	"io"

	"gopkg.in/yaml.v3"
)

const defaultFuncCliVersion = "knative-v1.21.0"

// ErrWorkflowExists is returned when a GitHub workflow file already exists and --force is not specified.
var ErrWorkflowExists = errors.New("existing GitHub workflow detected, overwrite using the --force option")

type githubWorkflow struct {
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

func NewGitHubWorkflow(conf CIConfig, messageWriter io.Writer) *githubWorkflow {
	var steps []step
	steps = createCheckoutStep(steps)
	steps = createRuntimeTestStep(conf, messageWriter, steps)
	steps = createK8ContextStep(conf, steps)
	steps = createRegistryLoginStep(conf, steps)
	steps = createFuncCLIInstallStep(steps)

	steps = createFuncDeployStep(conf, steps)

	return &githubWorkflow{
		Name: conf.WorkflowName(),
		On:   createPushTrigger(conf),
		Jobs: map[string]job{
			"deploy": {
				RunsOn: determineRunner(conf.SelfHostedRunner()),
				Steps:  steps,
			},
		},
	}
}

func createCheckoutStep(steps []step) []step {
	checkoutCode := newStep("Checkout code").
		withUses("actions/checkout@v4")

	return append(steps, *checkoutCode)
}

func createRuntimeTestStep(conf CIConfig, messageWriter io.Writer, steps []step) []step {
	if !conf.TestStep() {
		return steps
	}

	testStep := newStep("Run tests")

	switch conf.FnRuntime() {
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
		_, _ = fmt.Fprintf(messageWriter, "WARNING: test step not supported for runtime %s\n", conf.FnRuntime())
		return steps
	}

	return append(steps, *testStep)
}

func createK8ContextStep(conf CIConfig, steps []step) []step {
	setupK8Context := newStep("Setup Kubernetes context").
		withUses("azure/k8s-set-context@v4").
		withActionConfig("method", "kubeconfig").
		withActionConfig("kubeconfig", newSecret(conf.KubeconfigSecret()))

	return append(steps, *setupK8Context)
}

func createRegistryLoginStep(conf CIConfig, steps []step) []step {
	if !conf.RegistryLogin() {
		return steps
	}

	loginToContainerRegistry := newStep("Login to container registry").
		withUses("docker/login-action@v3").
		withActionConfig("registry", newVariable(conf.RegistryLoginUrlVar())).
		withActionConfig("username", newVariable(conf.RegistryUserVar())).
		withActionConfig("password", newSecret(conf.RegistryPassSecret()))

	return append(steps, *loginToContainerRegistry)
}

func createFuncCLIInstallStep(steps []step) []step {
	installFuncCli := newStep("Install func cli").
		withUses("functions-dev/action@main").
		withActionConfig("version", defaultFuncCliVersion).
		withActionConfig("name", "func")

	return append(steps, *installFuncCli)
}

func createFuncDeployStep(conf CIConfig, steps []step) []step {
	deployFuncStep := newStep("Deploy function").
		withEnv("FUNC_VERBOSE", "true").
		withEnv("FUNC_BUILDER", conf.FnBuilder())

	if conf.RemoteBuild() {
		deployFuncStep.withEnv("FUNC_REMOTE", "true")
	}

	registryUrl := newVariable(conf.RegistryUrlVar())
	if conf.RegistryLogin() {
		registryUrl = newVariable(conf.RegistryLoginUrlVar()) + "/" + newVariable(conf.RegistryUserVar())
	}
	deployFuncStep.withEnv("FUNC_REGISTRY", registryUrl).
		withRun("func deploy")

	return append(steps, *deployFuncStep)
}

func createPushTrigger(conf CIConfig) workflowTriggers {
	result := workflowTriggers{
		Push: &pushTrigger{Branches: []string{conf.Branch()}},
	}

	if conf.WorkflowDispatch() {
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

func (gw *githubWorkflow) Export(path string, w WorkflowWriter, force bool, m io.Writer) error {
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

func (gw *githubWorkflow) toYaml() ([]byte, error) {
	var buf bytes.Buffer
	encoder := yaml.NewEncoder(&buf)
	encoder.SetIndent(2)

	if err := encoder.Encode(gw); err != nil {
		return nil, err
	}
	encoder.Close()

	return buf.Bytes(), nil
}
