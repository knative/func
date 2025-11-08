package ci

import (
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

func NewGithubWorkflow(name string) *GithubWorkflow {
	return &GithubWorkflow{
		Name: name,
		On: WorkflowTriggers{
			Push: &PushTrigger{Branches: []string{"main"}},
		},
		Jobs: map[string]Job{
			"deploy": {
				RunsOn: "ubuntu-latest",
				Steps: []Step{
					{
						Name: "Checkout code",
						Uses: "actions/checkout@v4",
					},
					{
						Name: "Install func cli",
						Uses: "gauron99/knative-func-action@main",
						With: map[string]string{
							"version": "knative-v1.19.1",
							"name":    "func",
						},
					},
					{
						Name: "Deploy function",
						Run:  "func deploy --remote",
					},
				},
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
