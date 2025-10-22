package ci

import (
	"bytes"
	"fmt"

	"gopkg.in/yaml.v3"
)

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
	Uses string            `yaml:"uses,omitempty"`
	Run  string            `yaml:"run,omitempty"`
	With map[string]string `yaml:"with,omitempty"`
}

func NewGitHubWorkflow(conf CIConfig) *githubWorkflow {
	name := createWorkflowName(conf)
	runsOn := createRunsOn(conf)
	pushTrigger := createPushTrigger(conf)

	var steps []step
	steps = createCheckoutStep(steps)
	steps = createK8ContextStep(conf, steps)
	steps = createRegistryLoginStep(conf, steps)
	steps = createFuncCLIInstallStep(steps)
	steps = createFuncDeployStep(conf, steps)

	return &githubWorkflow{
		Name: name,
		On:   pushTrigger,
		Jobs: map[string]job{
			"deploy": {
				RunsOn: runsOn,
				Steps:  steps,
			},
		},
	}
}

func createWorkflowName(conf CIConfig) string {
	result := conf.WorkflowName()
	if conf.UseRemoteBuild() {
		result = "Remote Func Deploy"
	}

	return result
}

func createRunsOn(conf CIConfig) string {
	runsOn := "ubuntu-latest"
	if conf.UseSelfHostedRunner() {
		runsOn = "self-hosted"
	}
	return runsOn
}

func createPushTrigger(conf CIConfig) workflowTriggers {
	result := workflowTriggers{
		Push: &pushTrigger{Branches: []string{conf.Branch()}},
	}

	if conf.UseWorkflowDispatch() {
		result.WorkflowDispatch = &struct{}{}
	}

	return result
}

func createCheckoutStep(steps []step) []step {
	checkoutCode := newStep("Checkout code").
		withUses("actions/checkout@v4")

	return append(steps, *checkoutCode)
}

func createK8ContextStep(conf CIConfig, steps []step) []step {
	setupK8Context := newStep("Setup Kubernetes context").
		withUses("azure/k8s-set-context@v4").
		withActionConfig("method", "kubeconfig").
		withActionConfig("kubeconfig", newSecret(conf.KubeconfigSecret()))

	return append(steps, *setupK8Context)
}

func createRegistryLoginStep(conf CIConfig, steps []step) []step {
	if !conf.UseRegistryLogin() {
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
		withActionConfig("version", "knative-v1.20.1").
		withActionConfig("name", "func")

	return append(steps, *installFuncCli)
}

func createFuncDeployStep(conf CIConfig, steps []step) []step {
	runFuncDeploy := "func deploy"
	if conf.UseRemoteBuild() {
		runFuncDeploy += " --remote"
	}

	registryUrl := newVariable(conf.RegistryUrlVar())
	if conf.UseRegistryLogin() {
		registryUrl = newVariable(conf.RegistryLoginUrlVar()) + "/" + newVariable(conf.RegistryUserVar())
	}
	deployFunc := newStep("Deploy function").
		withRun(runFuncDeploy + " --registry=" + registryUrl + " -v")

	return append(steps, *deployFunc)
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

func newSecret(key string) string {
	return fmt.Sprintf("${{ secrets.%s }}", key)
}

func newVariable(key string) string {
	return fmt.Sprintf("${{ vars.%s }}", key)
}

func (gw *githubWorkflow) Export(path string, w WorkflowWriter) error {
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
