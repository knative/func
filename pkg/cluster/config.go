package cluster

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"

	"knative.dev/func/pkg/config"
)

// ClusterConfig holds all configuration for cluster create/delete operations.
type ClusterConfig struct {
	// Cluster identity
	Name   string // Cluster name (default: "func")
	Domain string // DNS domain for services (default: "localtest.me")

	// Component toggles
	Serving  bool // Install Knative Serving (default: true)
	Eventing bool // Install Knative Eventing (default: true)
	Tekton   bool // Install Tekton + PAC (default: false)
	Keda     bool // Install KEDA + HTTP add-on (default: false)

	// Operational
	Retries   int    // Max allocation attempts (default: 1)
	Namespace string // K8s namespace for RBAC bindings (default: "default")

	// ContainerEngineOverride, when non-empty, is returned verbatim by
	// ContainerEngine(). Empty means "auto-detect".
	// The CLI wires --container-engine into this field.
	ContainerEngineOverride string

	// PAC
	PacHost string // PAC controller hostname (default: "pac-ctr.localtest.me")

	// Skip flags
	SkipBinaries       bool // Skip binary downloads
	SkipRegistryConfig bool // Skip host registry configuration
	NoCleanup          bool // Don't delete cluster on failure

	// Optional tool path overrides. When non-empty, the kubectl/kind
	// accessors return the override verbatim; otherwise they resolve via
	// BinDir then PATH. The CLI samples FUNC_TEST_<TOOL> env vars into
	// these fields; the library itself never reads the environment.
	KubectlOverride string
	KindOverride    string

	// CI detection
	GitHubActions bool // Auto-detected from GITHUB_ACTIONS env
}

// BinDir returns the directory where managed tool binaries live:
// <config.Dir()>/bin. Derived on-demand so callers that mutate Name
// don't need to remember to refresh anything.
func (c ClusterConfig) BinDir() string {
	return filepath.Join(config.Dir(), "bin")
}

// Kubeconfig returns the kubeconfig path for this cluster:
// <config.Dir()>/clusters/<Name>.local/kubeconfig.yaml.
func (c ClusterConfig) Kubeconfig() string {
	return filepath.Join(config.Dir(), "clusters", c.Name+".local", "kubeconfig.yaml")
}

// ContainerEngine returns the container engine to use: the override if
// set, otherwise auto-detected. Called once per engine invocation rather
// than memoized because the auto-detect cost is trivial compared to the
// kind/kubectl work each call precedes.
//
// Auto-detection mirrors hack/common.sh:
//  1. If docker is on PATH, use it — but detect the podman-docker wrapper
//     (`/usr/bin/docker` shimmed to exec podman) by inspecting `docker
//     --version`; if it reports podman, switch to podman so downstream
//     podman-specific workarounds fire.
//  2. Otherwise, use podman if present.
//  3. Otherwise, fall back to "docker" (callers will surface a clearer
//     error when they try to exec it).
func (c ClusterConfig) ContainerEngine() string {
	if c.ContainerEngineOverride != "" {
		return c.ContainerEngineOverride
	}
	if _, err := exec.LookPath("docker"); err == nil {
		out, err := exec.Command("docker", "--version").Output()
		if err == nil && bytes.Contains(bytes.ToLower(out), []byte("podman")) {
			return "podman"
		}
		return "docker"
	}
	if _, err := exec.LookPath("podman"); err == nil {
		return "podman"
	}
	return "docker"
}

// kubectl returns the resolved path to the kubectl binary.
func (c ClusterConfig) kubectl() string {
	if c.KubectlOverride != "" {
		return c.KubectlOverride
	}
	return findTool("kubectl", c.BinDir())
}

// kind returns the resolved path to the kind binary.
func (c ClusterConfig) kind() string {
	if c.KindOverride != "" {
		return c.KindOverride
	}
	return findTool("kind", c.BinDir())
}

// findTool resolves a tool path by checking the managed BinDir first,
// then falling back to the system PATH. Overrides (e.g. from FUNC_TEST_<TOOL>)
// are handled by the ClusterConfig accessors before this is called.
func findTool(name, binDir string) string {
	if binDir != "" {
		p := filepath.Join(binDir, name)
		if info, err := os.Stat(p); err == nil && !info.IsDir() {
			return p
		}
	}
	if p, err := exec.LookPath(name); err == nil {
		return p
	}
	return name // fallback: bare name, will fail at execution time with a clear error
}

// controlPlaneContainer returns the kind control plane container name.
func (c ClusterConfig) controlPlaneContainer() string {
	return c.Name + "-control-plane"
}
