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
	ID   string            `yaml:"id,omitempty"`
	If   string            `yaml:"if,omitempty"`
	Uses string            `yaml:"uses,omitempty"`
	Run  string            `yaml:"run,omitempty"`
	With map[string]string `yaml:"with,omitempty"`
}

// TODO(twoGiants): add validation => no empty values, etc.
func NewGitHubWorkflow(conf CIConfig) *githubWorkflow {
	// TODO(twoGiants): add more runner labels => for GitHub enterprise clients
	runsOn := "ubuntu-latest"
	if conf.UseSelfHostedRunner() {
		runsOn = "self-hosted"
	}

	pushTrigger := newPushTrigger(conf.Branch(), conf.UseDebug())

	var steps []step
	checkoutCode := newStep("Checkout code").
		withUses("actions/checkout@v4")
	steps = append(steps, *checkoutCode)

	setupK8Context := newStep("Setup Kubernetes context").
		withUses("azure/k8s-set-context@v4").
		withActionConfig("method", "kubeconfig").
		withActionConfig("kubeconfig", newSecret(conf.KubeconfigSecret()))
	steps = append(steps, *setupK8Context)

	if conf.UseRegistryLogin() {
		loginToContainerRegistry := newStep("Login to container registry").
			withUses("docker/login-action@v3").
			withActionConfig("registry", newVariable(conf.RegistryLoginUrlVar())).
			withActionConfig("username", newVariable(conf.RegistryUserVar())).
			withActionConfig("password", newSecret(conf.RegistryPassSecret()))
		steps = append(steps, *loginToContainerRegistry)
	}

	if conf.UseDebug() {
		funcCliCache := newStep("Restore func cli from cache").
			withID("func-cli-cache").
			withUses("actions/cache@v4").
			withActionConfig("path", "func").
			withActionConfig("key", "func-cli-knative-v1.19.1")
		steps = append(steps, *funcCliCache)
	}

	installFuncCli := newStep("Install func cli").
		withUses("gauron99/knative-func-action@main").
		withActionConfig("version", "knative-v1.19.1").
		withActionConfig("name", "func")
	if conf.UseDebug() {
		installFuncCli.withIf("${{ steps.func-cli-cache.outputs.cache-hit != 'true' }}")
	}
	steps = append(steps, *installFuncCli)

	if conf.UseDebug() {
		addFuncToPATH := newStep("Add func to GITHUB_PATH").
			withRun(`echo "$GITHUB_WORKSPACE" >> $GITHUB_PATH`)
		steps = append(steps, *addFuncToPATH)
	}

	name := conf.WorkflowName()
	runFuncDeploy := "func deploy"
	if conf.UseRemoteBuild() {
		runFuncDeploy += " --remote"
		name = "Remote Func Deploy"
	}
	registryUrl := newVariable(conf.RegistryUrlVar())
	if conf.UseRegistryLogin() {
		registryUrl = newVariable(conf.RegistryLoginUrlVar()) + "/" + newVariable(conf.RegistryUserVar())
	}
	deployFunc := newStep("Deploy function").
		withRun(runFuncDeploy + " --registry=" + registryUrl + " -v")
	steps = append(steps, *deployFunc)

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

func newPushTrigger(branch string, debug bool) workflowTriggers {
	result := workflowTriggers{
		Push: &pushTrigger{Branches: []string{branch}},
	}

	if debug {
		result.WorkflowDispatch = &struct{}{}
	}

	return result
}

func newStep(name string) *step {
	return &step{Name: name}
}

func (s *step) withID(id string) *step {
	s.ID = id
	return s
}

func (s *step) withIf(ifCond string) *step {
	s.If = ifCond
	return s
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
