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
	If   string            `yaml:"if,omitempty"`
	With map[string]string `yaml:"with,omitempty"`
}

func NewGithubWorkflow(
	name,
	kubeconfigSecretKey string,
	selfHosted bool,
) *GithubWorkflow {
	runsOn := "ubuntu-latest"
	if selfHosted {
		runsOn = "self-hosted"
	}

	return &GithubWorkflow{
		Name: name,
		On: WorkflowTriggers{
			Push: &PushTrigger{Branches: []string{"main"}},
		},
		Jobs: map[string]Job{
			"deploy": {
				RunsOn: runsOn,
				Steps: []Step{
					{
						Name: "1. Checkout code",
						Uses: "actions/checkout@v4",
					},
					{
						Name: "2. Setup Kubernetes context",
						Uses: "azure/k8s-set-context@v4",
						With: map[string]string{
							"method":     "kubeconfig",
							"kubeconfig": NewSecret(kubeconfigSecretKey),
						},
					},
					{
						Name: "3. Login to container registry",
						If:   "${{ vars.USE_REGISTRY_AUTH == 'true' }}",
						Uses: "docker/login-action@v3",
						With: map[string]string{
							"registry": "${{ secrets.REGISTRY_URL }}",
							"username": "${{ secrets.REGISTRY_USERNAME }}",
							"password": "${{ secrets.REGISTRY_PASSWORD }}",
						},
					},
					{
						Name: "4. Install func cli",
						Uses: "gauron99/knative-func-action@main",
						With: map[string]string{
							"version": "knative-v1.19.1",
							"name":    "func",
						},
					},
					{
						Name: "5. Deploy function",
						Run:  "func deploy --remote --registry=${{ secrets.REGISTRY_URL }} -v",
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

func NewSecret(key string) string {
	return fmt.Sprintf("${{ secrets.%s }}", key)
}
