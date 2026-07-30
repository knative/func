package cluster

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/yaml"
)

const registryAddr = "registry.localtest.me"

// registryHTTPReadyURL is the hostPort path into the in-cluster registry.
// Contour + registry.localtest.me install in parallel with this goroutine, so
// readiness must not depend on Ingress being up yet — hostPort:5000 is live as
// soon as the Deployment is Available (same endpoint containerd mirrors use).
const registryHTTPReadyURL = "http://127.0.0.1:5000/v2/"

// installRegistry deploys the container registry as in-cluster Kubernetes
// resources (Deployment + ClusterIP Service + Contour Ingress), configures
// host-side trust, and applies the local-registry-hosting ConfigMap.
func installRegistry(ctx context.Context, cfg ClusterConfig, out io.Writer) error {
	start := time.Now()
	status(out, "Creating Registry")

	if err := applyObjects(ctx, out, cfg, registryDeployment(), registryService(), registryIngress()); err != nil {
		return fmt.Errorf("applying registry resources: %w", err)
	}

	if err := run(ctx, out, "",
		cfg.kubectl(), "wait",
		"--for=condition=Available", "deployment/registry",
		"-n", "default", "--timeout=5m"); err != nil {
		return fmt.Errorf("waiting for registry deployment: %w", err)
	}

	if err := waitForRegistryHTTP(ctx, out); err != nil {
		return err
	}

	if !cfg.SkipRegistryConfig {
		if err := configureHostRegistry(out); err != nil {
			return err
		}
	}

	if err := applyObjects(ctx, out, cfg, registryHostingConfigMap()); err != nil {
		return fmt.Errorf("applying registry configmap: %w", err)
	}

	success(out, "Registry", time.Since(start))
	return nil
}

// applyObjects marshals typed Kubernetes objects to multi-doc YAML and applies
// them via kubectl (same path as the string manifests elsewhere in this package).
func applyObjects(ctx context.Context, out io.Writer, cfg ClusterConfig, objs ...k8sruntime.Object) error {
	var docs []string
	for _, obj := range objs {
		b, err := yaml.Marshal(obj)
		if err != nil {
			return fmt.Errorf("marshaling %T: %w", obj, err)
		}
		docs = append(docs, string(b))
	}
	return applyManifest(ctx, out, cfg, strings.Join(docs, "---\n"))
}

func registryLabels() map[string]string {
	return map[string]string{"app": "registry"}
}

func registryDeployment() *appsv1.Deployment {
	replicas := int32(1)
	labels := registryLabels()
	return &appsv1.Deployment{
		TypeMeta: metav1.TypeMeta{APIVersion: "apps/v1", Kind: "Deployment"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "registry",
			Namespace: "default",
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{MatchLabels: labels},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: labels},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Name:  "registry",
						Image: "registry:2",
						Ports: []corev1.ContainerPort{{
							ContainerPort: 5000,
							HostPort:      5000,
						}},
						VolumeMounts: []corev1.VolumeMount{{
							Name:      "registry-data",
							MountPath: "/var/lib/registry",
						}},
					}},
					Volumes: []corev1.Volume{{
						Name: "registry-data",
						VolumeSource: corev1.VolumeSource{
							EmptyDir: &corev1.EmptyDirVolumeSource{},
						},
					}},
				},
			},
		},
	}
}

func registryService() *corev1.Service {
	return &corev1.Service{
		TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "Service"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "registry",
			Namespace: "default",
		},
		Spec: corev1.ServiceSpec{
			Selector: registryLabels(),
			Ports: []corev1.ServicePort{{
				Port:       5000,
				TargetPort: intstr.FromInt(5000),
			}},
		},
	}
}

func registryIngress() *networkingv1.Ingress {
	pathType := networkingv1.PathTypePrefix
	ingressClass := "contour-external"
	return &networkingv1.Ingress{
		TypeMeta: metav1.TypeMeta{APIVersion: "networking.k8s.io/v1", Kind: "Ingress"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "registry",
			Namespace: "default",
		},
		Spec: networkingv1.IngressSpec{
			IngressClassName: &ingressClass,
			Rules: []networkingv1.IngressRule{{
				Host: registryAddr,
				IngressRuleValue: networkingv1.IngressRuleValue{
					HTTP: &networkingv1.HTTPIngressRuleValue{
						Paths: []networkingv1.HTTPIngressPath{{
							Path:     "/",
							PathType: &pathType,
							Backend: networkingv1.IngressBackend{
								Service: &networkingv1.IngressServiceBackend{
									Name: "registry",
									Port: networkingv1.ServiceBackendPort{Number: 5000},
								},
							},
						}},
					},
				},
			}},
		},
	}
}

func registryHostingConfigMap() *corev1.ConfigMap {
	return &corev1.ConfigMap{
		TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "ConfigMap"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "local-registry-hosting",
			Namespace: "kube-public",
		},
		Data: map[string]string{
			"localRegistryHosting.v1": fmt.Sprintf(
				"host: %q\nhelp: \"https://kind.sigs.k8s.io/docs/user/local-registry/\"\n",
				registryAddr,
			),
		},
	}
}

// waitForRegistryHTTP polls the registry Distribution API until it answers 200
// or the context / timeout expires (matejvasek review on knative/func#3856).
func waitForRegistryHTTP(ctx context.Context, out io.Writer) error {
	status(out, "Waiting for Registry HTTP")
	client := &http.Client{Timeout: 2 * time.Second}
	deadline := time.Now().Add(2 * time.Minute)
	var last error
	for {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("waiting for registry HTTP: %w", err)
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, registryHTTPReadyURL, nil)
		if err != nil {
			return err
		}
		resp, err := client.Do(req)
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
			last = fmt.Errorf("status %s", resp.Status)
		} else {
			last = err
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("waiting for registry HTTP at %s: last error: %w", registryHTTPReadyURL, last)
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("waiting for registry HTTP: %w", ctx.Err())
		case <-time.After(2 * time.Second):
		}
	}
}

// configureHostRegistry configures the host's container engine(s) to
// trust the shared local registry. Mirror of revertHostRegistry; called
// at most once per installRegistry (the caller gates on
// SkipRegistryConfig). Equivalent to hack/registry.sh.
func configureHostRegistry(out io.Writer) error {
	status(out, "Enabling local HTTP access to container registry")

	warnNix(out)

	anyConfigured := false
	if hasCommand("docker") {
		if err := configureDockerHTTP(out); err != nil {
			warnf(out, "Failed to configure Docker: %v", err)
		} else {
			anyConfigured = true
		}
	}

	if hasCommand("podman") {
		if err := configurePodmanHTTP(out); err != nil {
			warnf(out, "Failed to configure Podman: %v", err)
		} else {
			anyConfigured = true
		}
	}

	if anyConfigured {
		fmt.Fprintln(out, yellow(fmt.Sprintf(
			"Note: %s is now a trusted insecure registry for this machine's container\n"+
				"      engine. Any process with local access can push, pull, or delete\n"+
				"      images there. Removed when the last func-managed cluster is\n"+
				"      deleted.",
			registryAddr)))
	}
	return nil
}

// configureDockerHTTP adds the registry to Docker's insecure-registries
// list, preserving any other daemon.json settings the user has configured.
func configureDockerHTTP(out io.Writer) error {
	path, useSudo := dockerConfigPath()
	config, err := readDockerDaemon(path, useSudo)
	if err != nil {
		return err
	}
	if err := addInsecureRegistry(config, registryAddr); err != nil {
		return err
	}

	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling daemon.json: %w", err)
	}
	if err := writeFileWithSudo(path, data, useSudo); err != nil {
		return fmt.Errorf("writing daemon.json: %w", err)
	}

	fmt.Fprintf(out, "OK %s\n", path)
	if runtime.GOOS == "darwin" {
		fmt.Fprintln(out, yellow("*** If Docker Desktop is running, please restart it via the menu bar icon ***"))
	} else {
		fmt.Fprintln(out, "daemon.json updated; not restarting dockerd mid-setup (would tear down the in-progress cluster)")
	}
	return nil
}

// addInsecureRegistry appends registry to config["insecure-registries"] if
// not already present, preserving any existing entries. Errors if the
// existing value isn't a JSON array, rather than silently overwriting.
func addInsecureRegistry(config map[string]any, registry string) error {
	raw, present := config["insecure-registries"]
	if !present {
		config["insecure-registries"] = []any{registry}
		return nil
	}
	existing, ok := raw.([]any)
	if !ok {
		return fmt.Errorf("unexpected type for insecure-registries: %T (refusing to overwrite)", raw)
	}
	for _, r := range existing {
		if s, ok := r.(string); ok && s == registry {
			return nil
		}
	}
	config["insecure-registries"] = append(existing, registry)
	return nil
}

// configurePodmanHTTP adds the registry to Podman's registries.conf.
func configurePodmanHTTP(out io.Writer) error {
	configFile, useSudo, exists := podmanConfigPath()

	if !exists {
		// Neither user nor system config present — create a fresh user-level file.
		userConfigDir := filepath.Dir(configFile)
		fmt.Fprintln(out, "No existing Podman registries.conf found.")
		if err := os.MkdirAll(userConfigDir, 0o755); err != nil {
			fmt.Fprintln(out, "Could not create user config directory. Skipping Podman registry configuration.")
			return nil
		}
		fmt.Fprintf(out, "Creating new user-level Podman registry config at %s\n", configFile)
		content := fmt.Sprintf("# Podman registries configuration\n# Generated by func cluster create\n\n[[registry]]\nlocation = %q\ninsecure = true\n", registryAddr)
		if err := os.WriteFile(configFile, []byte(content), 0o644); err != nil {
			return fmt.Errorf("writing config: %w", err)
		}
		fmt.Fprintf(out, "Successfully created Podman registry configuration for %s\n", registryAddr)
		return nil
	}

	if useSudo {
		fmt.Fprintf(out, "Using existing system Podman registry config at %s\n", configFile)
	} else {
		fmt.Fprintf(out, "Using existing user Podman registry config at %s\n", configFile)
	}

	// Read existing config
	data, err := readFileWithSudo(configFile, useSudo)
	if err != nil {
		return fmt.Errorf("reading %s: %w", configFile, err)
	}
	content := string(data)

	// Check if already configured
	if strings.Contains(content, registryAddr) {
		fmt.Fprintf(out, "%s is already configured in %s\n", registryAddr, configFile)
		return nil
	}

	// Only v2 (`[[registry]]` stanzas) is handled. v1
	// (`[registries.insecure]`) is deprecated and its in-place edit
	// paths are error-prone, so we skip rather than risk clobbering.
	if !strings.Contains(content, "[[registry]]") && strings.Contains(content, "[registries.insecure]") {
		warnf(out, "%s appears to use the deprecated v1 registries.conf format.\n"+
			"         Skipping Podman config; add %q manually to continue.",
			configFile, registryAddr)
		return nil
	}

	fmt.Fprintln(out, "Adding insecure registry")
	appendContent := fmt.Sprintf("\n[[registry]]\nlocation = %q\ninsecure = true\n", registryAddr)
	if err := appendFileWithSudo(configFile, []byte(appendContent), useSudo); err != nil {
		return err
	}

	return nil
}

// warnNix detects Nix and emits configuration guidance.
func warnNix(out io.Writer) {
	if !hasCommand("nix") && !hasCommand("nixos-rebuild") {
		return
	}

	fmt.Fprintln(out, yellow("Warning: Nix detected"))

	if hasCommand("docker") {
		if runtime.GOOS == "darwin" {
			fmt.Fprintf(out, `If Docker Desktop was installed via Nix on macOS, you may need to manually configure the insecure registry.
Please confirm %q is specified as an insecure registry in the docker config file.
`, registryAddr)
		} else {
			fmt.Fprintf(out, `If Docker was configured using nix, this command will fail to find daemon.json.
Please configure the insecure registry by modifying your nix config:
  virtualisation.docker = {
    enable = true;
    daemon.settings.insecure-registries = [ %q ];
  };
`, registryAddr)
		}
	}

	if hasCommand("podman") {
		fmt.Fprintf(out, `If podman was configured via Nix, this command will likely fail.
The configuration required is adding the following to registries.conf:
  [[registry]]
  location = %q
  insecure = true
`, registryAddr)
	}
}

// revertHostRegistry removes the insecure-registries entry we added at
// create time and the matching podman stanza. Best-effort: per-engine
// failures warn but don't abort the delete.
func revertHostRegistry(out io.Writer) {
	if hasCommand("docker") {
		if err := revertDockerHTTP(out); err != nil {
			warnf(out, "failed to revert Docker insecure-registries: %v", err)
		}
	}
	if hasCommand("podman") {
		if err := revertPodmanHTTP(out); err != nil {
			warnf(out, "failed to revert Podman registries.conf: %v", err)
		}
	}
}

// revertDockerHTTP removes registryAddr from daemon.json's
// insecure-registries slice. No-op if the entry isn't there.
func revertDockerHTTP(out io.Writer) error {
	path, useSudo := dockerConfigPath()
	config, err := readDockerDaemon(path, useSudo)
	if err != nil {
		return err
	}
	changed, err := removeInsecureRegistry(config, registryAddr)
	if err != nil {
		return err
	}
	if !changed {
		return nil
	}
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling daemon.json: %w", err)
	}
	if err := writeFileWithSudo(path, data, useSudo); err != nil {
		return fmt.Errorf("writing daemon.json: %w", err)
	}
	fmt.Fprintf(out, "Removed %s from %s\n", registryAddr, path)
	if runtime.GOOS == "darwin" {
		fmt.Fprintln(out, yellow("*** If Docker Desktop is running, please restart it via the menu bar icon ***"))
	}
	return nil
}

// removeInsecureRegistry strips registry from config["insecure-registries"]
// if present, and removes the key entirely when the slice becomes empty.
// Returns (changed, error); errors if the existing value isn't a JSON
// array, rather than silently overwriting.
func removeInsecureRegistry(config map[string]any, registry string) (bool, error) {
	raw, present := config["insecure-registries"]
	if !present {
		return false, nil
	}
	existing, ok := raw.([]any)
	if !ok {
		return false, fmt.Errorf("unexpected type for insecure-registries: %T (refusing to overwrite)", raw)
	}
	// In-place filter: `kept` reuses `existing`'s backing array. Safe here
	// because writes never race reads (we only write at `len(kept)`, and
	// the loop reads element `i` before we'd overwrite it). We reassign
	// `config["insecure-registries"]` to `kept` at the end, so any trailing
	// orphan elements in the original array become unreachable.
	kept := existing[:0]
	removed := false
	for _, r := range existing {
		if s, ok := r.(string); ok && s == registry {
			removed = true
			continue
		}
		kept = append(kept, r)
	}
	if !removed {
		return false, nil
	}
	if len(kept) == 0 {
		delete(config, "insecure-registries")
	} else {
		config["insecure-registries"] = kept
	}
	return true, nil
}

// revertPodmanHTTP removes the v2 `[[registry]]` stanza we injected at
// create time. The block has a fixed shape, so a literal string match is
// reliable. v1 (`[registries.insecure]`) is not reverted — the format is
// deprecated and entries are typically shared across sections.
func revertPodmanHTTP(out io.Writer) error {
	path, useSudo, exists := podmanConfigPath()
	if !exists {
		return nil
	}
	data, err := readFileWithSudo(path, useSudo)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) || !fileExists(path) {
			return nil
		}
		return fmt.Errorf("reading %s: %w", path, err)
	}
	stanza := fmt.Sprintf("\n[[registry]]\nlocation = %q\ninsecure = true\n", registryAddr)
	content := string(data)
	if !strings.Contains(content, stanza) {
		return nil
	}
	updated := strings.Replace(content, stanza, "", 1)
	if err := writeFileWithSudo(path, []byte(updated), useSudo); err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}
	fmt.Fprintf(out, "Removed %s from %s\n", registryAddr, path)
	return nil
}

// Helpers
// -------

// podmanConfigPath resolves Podman's registries.conf. The returned path
// is always populated; `exists` tells the caller whether the file is
// actually on disk (callers that want to *configure* create if absent,
// callers that want to *revert* no-op if absent). `useSudo` is only
// meaningful when exists=true, reflecting whether the file is the
// system-wide /etc path. When neither user nor system path exists, the
// user-level XDG path is returned as the default for create.
func podmanConfigPath() (path string, useSudo bool, exists bool) {
	xdgConfig := os.Getenv("XDG_CONFIG_HOME")
	if xdgConfig == "" {
		home, _ := os.UserHomeDir()
		xdgConfig = filepath.Join(home, ".config")
	}
	userPath := filepath.Join(xdgConfig, "containers", "registries.conf")
	if fileExists(userPath) {
		return userPath, false, true
	}
	systemPath := "/etc/containers/registries.conf"
	if fileExists(systemPath) {
		return systemPath, true, true
	}
	return userPath, false, false
}

// dockerConfigPath returns the daemon.json path and whether writing it
// requires sudo. Darwin (Docker Desktop) uses the per-user path; Linux
// writes to /etc/docker/daemon.json, which requires root.
func dockerConfigPath() (path string, useSudo bool) {
	if runtime.GOOS == "darwin" {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, ".docker", "daemon.json"), false
	}
	return "/etc/docker/daemon.json", true
}

// readDockerDaemon loads daemon.json. A missing file returns an empty
// config (first-time setup); read/parse failures return an error so we
// don't silently overwrite a daemon.json the user has customized.
func readDockerDaemon(path string, useSudo bool) (map[string]any, error) {
	data, err := readFileWithSudo(path, useSudo)
	if errors.Is(err, fs.ErrNotExist) || (err != nil && !fileExists(path)) {
		return map[string]any{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	if len(data) == 0 {
		return map[string]any{}, nil
	}
	var config map[string]any
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	if config == nil {
		config = map[string]any{}
	}
	return config, nil
}

func hasCommand(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func readFileWithSudo(path string, sudo bool) ([]byte, error) {
	if !sudo {
		return os.ReadFile(path)
	}
	out, err := exec.Command("sudo", "cat", path).Output()
	if err != nil {
		return nil, err
	}
	return out, nil
}

func writeFileWithSudo(path string, data []byte, sudo bool) error {
	if !sudo {
		return os.WriteFile(path, data, 0o644)
	}
	cmd := exec.Command("sudo", "tee", path)
	cmd.Stdin = strings.NewReader(string(data))
	cmd.Stdout = io.Discard
	return cmd.Run()
}

func appendFileWithSudo(path string, data []byte, sudo bool) error {
	if !sudo {
		f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
		if err != nil {
			return err
		}
		defer f.Close()
		_, err = f.Write(data)
		return err
	}
	cmd := exec.Command("sudo", "tee", "-a", path)
	cmd.Stdin = strings.NewReader(string(data))
	cmd.Stdout = io.Discard
	return cmd.Run()
}
