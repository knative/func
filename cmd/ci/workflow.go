package ci

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

const (
	dirPerm  = 0755 // o: rwx, g|u: r-x
	filePerm = 0644 // o: rw,  g|u: r
)

// TODO(twoGiants)
//   - encapsulate => create Interface and defaultGithubWorkflow struct
//   - provide printers for configurable properties
//   - provide toYamlString for checks in tests
type GithubWorkflow struct {
	Name string           `yaml:"name"`
	On   WorkflowTriggers `yaml:"on"`
	Jobs map[string]Job   `yaml:"jobs"`
}

type WorkflowTriggers struct {
	Push             *PushTrigger `yaml:"push,omitempty"`
	WorkflowDispatch *struct{}    `yaml:"workflow_dispatch,omitempty"`
}

func newPushTrigger(branch string, debug bool) WorkflowTriggers {
	result := WorkflowTriggers{
		Push: &PushTrigger{Branches: []string{branch}},
	}

	if debug {
		result.WorkflowDispatch = &struct{}{}
	}

	return result
}

type PushTrigger struct {
	Branches []string `yaml:"branches,omitempty"`
}

type Job struct {
	RunsOn string `yaml:"runs-on"`
	Steps  []Step `yaml:"steps"`
}

type Step struct {
	Name string            `yaml:"name,omitempty"`
	ID   string            `yaml:"id,omitempty"`
	If   string            `yaml:"if,omitempty"`
	Uses string            `yaml:"uses,omitempty"`
	Run  string            `yaml:"run,omitempty"`
	With map[string]string `yaml:"with,omitempty"`
}

func newStep(name string) *Step {
	return &Step{Name: name}
}

func (s *Step) withID(id string) *Step {
	s.ID = id
	return s
}

func (s *Step) withIf(ifCond string) *Step {
	s.If = ifCond
	return s
}

func (s *Step) withUses(u string) *Step {
	s.Uses = u
	return s
}

func (s *Step) withRun(r string) *Step {
	s.Run = r
	return s
}

func (s *Step) withActionConfig(key, value string) *Step {
	if s.With == nil {
		s.With = make(map[string]string)
	}

	s.With[key] = value

	return s
}

func NewGithubWorkflow(conf CIConfig) *GithubWorkflow {
	runsOn := "ubuntu-latest"
	if conf.UseSelfHostedRunner() {
		runsOn = "self-hosted"
	}

	pushTrigger := newPushTrigger(conf.Branch(), conf.UseDebug())

	var steps []Step
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

	return &GithubWorkflow{
		Name: name,
		On:   pushTrigger,
		Jobs: map[string]Job{
			"deploy": {
				RunsOn: runsOn,
				Steps:  steps,
			},
		},
	}
}

func NewGithubWorkflowFromPath(path string) (*GithubWorkflow, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var result GithubWorkflow
	if err = yaml.Unmarshal(raw, &result); err != nil {
		return nil, err
	}

	return &result, nil
}

func (gw *GithubWorkflow) Persist(path string) error {
	raw, err := gw.toYaml()
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(path), dirPerm); err != nil {
		return err
	}

	if err := os.WriteFile(path, raw, filePerm); err != nil {
		return err
	}

	return nil
}

func (gw *GithubWorkflow) YamlString() (string, error) {
	raw, err := gw.toYaml()
	if err != nil {
		return "", err
	}

	return string(raw), nil
}

func (gw *GithubWorkflow) toYaml() ([]byte, error) {
	var buf bytes.Buffer
	encoder := yaml.NewEncoder(&buf)
	encoder.SetIndent(2)

	if err := encoder.Encode(gw); err != nil {
		return nil, err
	}
	encoder.Close()

	return buf.Bytes(), nil
}

func newSecret(key string) string {
	return fmt.Sprintf("${{ secrets.%s }}", key)
}

func newVariable(key string) string {
	return fmt.Sprintf("${{ vars.%s }}", key)
}
