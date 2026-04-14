package cluster

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const (
	// registryContainerName is the fixed name of the shared local registry
	// container. All func-managed dev clusters on the host share this
	// single registry.
	registryContainerName = "func-registry"
	// registryHostPort is the TCP port the registry is published on to the
	// host; it also appears in the host container engine's
	// insecure-registries list.
	registryHostPort = 50000
	// registryContainerPort is the port the `registry:2` image listens on
	// inside the container.
	registryContainerPort = 5000
)

// registryAddr is the host-side address used in daemon.json /
// registries.conf and in the in-cluster `local-registry-hosting` ConfigMap.
// Derived from registryHostPort so the two can't drift apart.
var registryAddr = fmt.Sprintf("localhost:%d", registryHostPort)

// installRegistry starts the shared local container registry, configures
// host-side trust for it, and applies the in-cluster ConfigMap + Service
// the kind cluster uses to reach it.
func installRegistry(ctx context.Context, cfg ClusterConfig, out io.Writer) error {
	start := time.Now()
	status(out, "Creating Registry")

	if err := ensureRegistry(ctx, cfg, out); err != nil {
		return err
	}

	if !cfg.SkipRegistryConfig {
		if err := configureHostRegistry(out); err != nil {
			return err
		}
	}

	// ConfigMap for local registry hosting
	registryConfigMap := fmt.Sprintf(`apiVersion: v1
kind: ConfigMap
metadata:
  name: local-registry-hosting
  namespace: kube-public
data:
  localRegistryHosting.v1: |
    host: "localhost:%d"
    help: "https://kind.sigs.k8s.io/docs/user/local-registry/"
`, registryHostPort)

	if err := applyManifest(ctx, out, cfg, registryConfigMap); err != nil {
		return fmt.Errorf("applying registry configmap: %w", err)
	}

	// ExternalName service for in-cluster access
	registrySvc := fmt.Sprintf(`apiVersion: v1
kind: Service
metadata:
  name: registry
  namespace: default
spec:
  type: ExternalName
  externalName: %s
`, registryContainerName)

	if err := applyManifest(ctx, out, cfg, registrySvc); err != nil {
		return fmt.Errorf("applying registry service: %w", err)
	}

	success(out, "Registry", time.Since(start))
	return nil
}

// ensureRegistry makes sure the shared func-registry container exists, is
// running, and is attached to the `kind` docker network. Idempotent; safe
// to call whether or not another func-managed cluster has already
// provisioned it.
//
// Scope is intentionally just the container lifecycle — host-side trust
// config is the orchestrator's responsibility (see installRegistry).
// TODO: should we rename the kind network to "kind-func" to avoid collision
// with a developer's own kind usage?
func ensureRegistry(ctx context.Context, cfg ClusterConfig, out io.Writer) error {
	exists, running, networked, err := registryStatus(ctx, cfg)
	if err != nil {
		return err
	}
	if !exists {
		portMap := fmt.Sprintf("127.0.0.1:%d:%d", registryHostPort, registryContainerPort)
		// --net=kind attaches at creation time, so no separate network
		// connect is needed on this path.
		return run(ctx, out, "",
			cfg.ContainerEngine(), "run",
			"-d",
			"--restart=always",
			"-p", portMap,
			"--net=kind",
			"--name", registryContainerName,
			"registry:2")
	}
	if !running {
		if err := run(ctx, out, "", cfg.ContainerEngine(), "start", registryContainerName); err != nil {
			return fmt.Errorf("starting registry: %w", err)
		}
	}
	if !networked {
		if err := run(ctx, out, "", cfg.ContainerEngine(), "network", "connect", "kind", registryContainerName); err != nil {
			return fmt.Errorf("connecting registry to kind network: %w", err)
		}
	}
	return nil
}

// registryStatus inspects the shared registry container. A non-nil err
// means the engine itself errored in a way that isn't "no such object";
// callers should surface it. A (false, false, false, nil) return means
// "container is absent" or the inspect was unparseable — either way,
// treated as fresh state.
func registryStatus(ctx context.Context, cfg ClusterConfig) (exists, running, networked bool, err error) {
	output, inspectErr := runOutput(ctx, cfg.ContainerEngine(), "container", "inspect", registryContainerName)
	if inspectErr != nil {
		// `container inspect <missing>` exits non-zero; so does any real
		// engine failure. Treat both as "not present" — a real failure
		// resurfaces on the next engine command.
		return false, false, false, nil
	}
	var results []struct {
		State struct {
			Running bool `json:"Running"`
		} `json:"State"`
		NetworkSettings struct {
			Networks map[string]json.RawMessage `json:"Networks"`
		} `json:"NetworkSettings"`
	}
	if err := json.Unmarshal([]byte(output), &results); err != nil {
		return false, false, false, fmt.Errorf("parsing inspect output: %w", err)
	}
	if len(results) == 0 {
		return false, false, false, nil
	}
	exists = true
	running = results[0].State.Running
	_, networked = results[0].NetworkSettings.Networks["kind"]
	return
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
		setupPodmanMacOSForwarding(out)
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

	setupPodmanMacOSForwarding(out)
	return nil
}

// setupPodmanMacOSForwarding sets up SSH port forwarding on macOS so the
// Podman VM can access the host's local registry. Idempotent: detects an
// existing backgrounded ssh forwarder and skips rather than spawning
// another (which would leak or fail to bind).
func setupPodmanMacOSForwarding(out io.Writer) {
	if runtime.GOOS != "darwin" {
		return
	}
	forward := fmt.Sprintf("-L %d:localhost:%d", registryHostPort, registryHostPort)
	if err := exec.Command("pgrep", "-f", forward).Run(); err == nil {
		fmt.Fprintln(out, "Podman VM port forwarding already active; skipping")
		return
	}
	fmt.Fprintln(out, "Setting up port forwarding for Podman VM to access registry...")
	port := fmt.Sprintf("%d", registryHostPort)
	cmd := exec.Command("podman", "machine", "ssh", "--",
		"-L", port+":localhost:"+port, "-N", "-f")
	cmd.Stdout = out
	cmd.Stderr = out
	if err := cmd.Run(); err != nil {
		fmt.Fprintf(out, "Warning: port forwarding setup failed: %v\n", err)
	}
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

// Teardowns
// ---------

// teardownRegistry stops and removes the shared registry container. Called
// from Delete when the last func-managed cluster is being removed.
func teardownRegistry(ctx context.Context, cfg ClusterConfig, out io.Writer) {
	if err := run(ctx, out, "", cfg.ContainerEngine(), "rm", "-f", registryContainerName); err != nil {
		fmt.Fprintf(out, "Warning: failed to remove registry container %q: %v\n", registryContainerName, err)
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
