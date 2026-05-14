//go:build e2e

package e2e

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestConfigCI_DeployFuncViaGeneratedGitHubWorkflow(t *testing.T) {
	if !ConfigCI {
		t.Skip("Config CI tests not enabled. Enable with FUNC_E2E_CONFIG_CI=true")
	}
	for _, runtime := range []string{"go", "node", "typescript", "python", "quarkus"} {
		name := fmt.Sprintf("func-e2e-ci-config-%s", runtime)
		t.Run(name, func(t *testing.T) {
			root := fromCleanEnv(t, name)

			t.Setenv("FUNC_ENABLE_CI_CONFIG", "true")

			t.Cleanup(func() {
				cleanImages(t, name)
			})
			t.Cleanup(func() {
				clean(t, name, Namespace)
			})

			// Create function which will be deployed by the Github Workflow
			if err := newCmd(t, "init", "-l", runtime).Run(); err != nil {
				t.Fatalf("Failed to create %s function: %v", runtime, err)
			}

			gitInit(t, root)

			// Generate GitHub Workflow YAML
			if err := newCmd(t, "config", "ci", "--registry-login=false").Run(); err != nil {
				t.Fatal(err)
			}

			runGitHubWorkflow(t, root)

			if !waitFor(t, ksvcUrl(name)) {
				t.Fatal("deployed function not reachable")
			}
		})
	}
}

// gitInit initializes a git repository in dir with an initial commit.
func gitInit(t *testing.T, dir string) {
	t.Helper()

	gitArgsList := [][]string{
		{"init", "-b", "main"},
		{"config", "user.email", "test@test.com"},
		{"config", "user.name", "test"},
		{"add", "."},
		{"commit", "-m", "init"},
	}
	for _, args := range gitArgsList {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v failed: %v\n%s", args, err, output)
		}
	}
}

// runGitHubWorkflow runs the func-deploy GitHub Actions workflow locally using act.
func runGitHubWorkflow(t *testing.T, dir string) {
	t.Helper()

	// Patch the generated workflow to use the locally-built func binary
	// instead of downloading a release via functions-dev/action.
	patchWorkflowWithLocalBinary(t, filepath.Join(dir, ".github", "workflows", "func-deploy.yaml"))

	args := []string{"push",
		"-P", "ubuntu-latest=-self-hosted",
		"-W", ".github/workflows/func-deploy.yaml",
		"-s", "KUBECONFIG=" + readFile(t, Kubeconfig),
		"--var", "REGISTRY_URL=" + Registry,
	}
	if strings.Contains(Registry, "registry.localtest.me") {
		args = append(args, "--env", "FUNC_REGISTRY_INSECURE=true")
	}
	cmd := exec.Command("act", args...)
	cmd.Dir = dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}
}

// patchWorkflowWithLocalBinary replaces the "Install func cli" step in the
// generated GitHub workflow with a step that symlinks the locally-built binary.
// This ensures the CI config test exercises the current code rather than a
// released version that may lack recent fixes.
func patchWorkflowWithLocalBinary(t *testing.T, workflowPath string) {
	t.Helper()

	data, err := os.ReadFile(workflowPath)
	if err != nil {
		t.Fatalf("failed to read workflow %s: %v", workflowPath, err)
	}

	var wf map[string]interface{}
	if err := yaml.Unmarshal(data, &wf); err != nil {
		t.Fatalf("failed to parse workflow: %v", err)
	}

	jobs, _ := wf["jobs"].(map[string]interface{})
	deploy, _ := jobs["deploy"].(map[string]interface{})
	steps, _ := deploy["steps"].([]interface{})

	binDir := t.TempDir()
	for i, s := range steps {
		step, _ := s.(map[string]interface{})
		if step["name"] == "Install func cli" {
			steps[i] = map[string]interface{}{
				"name": "Install func cli",
				"run":  fmt.Sprintf("ln -sf %s %s/func && echo %s >> $GITHUB_PATH", Bin, binDir, binDir),
			}
			break
		}
	}

	out, err := yaml.Marshal(wf)
	if err != nil {
		t.Fatalf("failed to marshal workflow: %v", err)
	}

	if err := os.WriteFile(workflowPath, out, 0644); err != nil {
		t.Fatalf("failed to write workflow %s: %v", workflowPath, err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read %s: %v", path, err)
	}

	return string(data)
}
