package ci

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

const (
	dirPerm  = 0755 // o: rwx, g|u: r-x
	filePerm = 0644 // o: rw,  g|u: r
)

type GithubWorkflow struct {
	Name string           `yaml:"name"`
	On   WorkflowTriggers `yaml:"on"`
	Jobs map[string]Job   `yaml:"jobs"`
}

type WorkflowTriggers struct {
	Push *PushTrigger `yaml:"push,omitempty"`
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
	Uses string            `yaml:"uses,omitempty"`
	Run  string            `yaml:"run,omitempty"`
	With map[string]string `yaml:"with,omitempty"`
}

func newStep(name string) *Step {
	return &Step{Name: name}
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

func NewGithubWorkflow(
	name,
	kubeconfigSecretKey,
	registryUrlSecretKey,
	registryUserSecretKey,
	registryPassSecretKey string,
	useRegistryLogin,
	selfHosted bool,
) *GithubWorkflow {
	runsOn := "ubuntu-latest"
	if selfHosted {
		runsOn = "self-hosted"
	}

	var steps []Step
	checkoutCode := newStep("Checkout code").
		withUses("actions/checkout@v4")
	steps = append(steps, *checkoutCode)

	setupK8Context := newStep("Setup Kubernetes context").
		withUses("azure/k8s-set-context@v4").
		withActionConfig("method", "kubeconfig").
		withActionConfig("kubeconfig", newSecret(kubeconfigSecretKey))
	steps = append(steps, *setupK8Context)

	if useRegistryLogin {
		loginToContainerRegistry := newStep("Login to container registry").
			withUses("docker/login-action@v3").
			withActionConfig("registry", newSecret(registryUrlSecretKey)).
			withActionConfig("username", newSecret(registryUserSecretKey)).
			withActionConfig("password", newSecret(registryPassSecretKey))
		steps = append(steps, *loginToContainerRegistry)
	}

	installFuncCli := newStep("Install func cli").
		withUses("gauron99/knative-func-action@main").
		withActionConfig("version", "knative-v1.19.1").
		withActionConfig("name", "func")
	steps = append(steps, *installFuncCli)

	deployFunc := newStep("Deploy function").
		withRun("func deploy --remote --registry=" + newSecret(registryUrlSecretKey) + " -v")
	steps = append(steps, *deployFunc)

	return &GithubWorkflow{
		Name: name,
		On: WorkflowTriggers{
			Push: &PushTrigger{Branches: []string{"main"}},
		},
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
	raw, err := yaml.Marshal(gw)
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
	raw, err := yaml.Marshal(gw)
	if err != nil {
		return "", err
	}

	return string(raw), nil
}

func newSecret(key string) string {
	return fmt.Sprintf("${{ secrets.%s }}", key)
}
