package cluster

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"time"
)

// kindConfigTemplate is the kind cluster config.
//
// metalLBPoolTemplate is the MetalLB IPAddressPool + L2Advertisement.
// %s is the pre-formatted YAML list of "<addr>/<prefix>" lines.
const kindConfigTemplate = `kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
nodes:
  - role: control-plane
    image: kindest/node:%[1]s
    extraPortMappings:
    - containerPort: 80
      hostPort: 80
      listenAddress: "127.0.0.1"
    - containerPort: 443
      hostPort: 443
      listenAddress: "127.0.0.1"
    - containerPort: 30022
      hostPort: 30022
      listenAddress: "127.0.0.1"
containerdConfigPatches:
- |-
  [plugins."io.containerd.grpc.v1.cri".registry.mirrors."localhost:%[2]d"]
    endpoint = ["http://%[3]s:%[4]d"]
  [plugins."io.containerd.grpc.v1.cri".registry.mirrors."registry.default.svc.cluster.local:%[4]d"]
    endpoint = ["http://%[3]s:%[4]d"]
  [plugins."io.containerd.grpc.v1.cri".registry.mirrors."ghcr.io"]
    endpoint = ["http://%[3]s:%[4]d"]
  [plugins."io.containerd.grpc.v1.cri".registry.mirrors."quay.io"]
    endpoint = ["http://%[3]s:%[4]d"]
`

const metalLBPoolTemplate = `apiVersion: metallb.io/v1beta1
kind: IPAddressPool
metadata:
  name: example
  namespace: metallb-system
spec:
  addresses:
%s---
apiVersion: metallb.io/v1beta1
kind: L2Advertisement
metadata:
  name: empty
  namespace: metallb-system
`

// installKubernetes creates a kind cluster with the configured node image and port mappings.
func installKubernetes(ctx context.Context, cfg ClusterConfig, out io.Writer) error {
	start := time.Now()
	status(out, "Allocating")

	kindConfig := fmt.Sprintf(kindConfigTemplate,
		kindNodeVersion, registryHostPort, registryContainerName, registryContainerPort)

	err := run(ctx, out, kindConfig,
		cfg.kind(), "create", "cluster",
		"--name="+cfg.Name,
		"--kubeconfig="+cfg.Kubeconfig(),
		"--wait=60s",
		"--config=-")
	if err != nil {
		return fmt.Errorf("creating cluster: %w", err)
	}

	err = run(ctx, out, "",
		cfg.kubectl(), "wait", "pod",
		"--for=condition=Ready", "-l", "!job-name", "-n", "kube-system", "--timeout=5m")
	if err != nil {
		return fmt.Errorf("waiting for kube-system: %w", err)
	}

	success(out, "Kubernetes", time.Since(start))
	return nil
}

// installLoadBalancer installs MetalLB and configures the address pool using
// the kind node's IP addresses (parsed natively in Go, replacing jq).
func installLoadBalancer(ctx context.Context, cfg ClusterConfig, out io.Writer) error {
	start := time.Now()
	status(out, "Installing Load Balancer (MetalLB)")

	url := fmt.Sprintf("https://raw.githubusercontent.com/metallb/metallb/%s/config/manifests/metallb-native.yaml", metalLBVersion)
	err := run(ctx, out, "",
		cfg.kubectl(), "apply", "-f", url)
	if err != nil {
		return fmt.Errorf("applying metallb: %w", err)
	}

	err = run(ctx, out, "",
		cfg.kubectl(), "wait",
		"--namespace", "metallb-system",
		"--for=condition=ready", "pod",
		"--selector=app=metallb",
		"--timeout=300s")
	if err != nil {
		return fmt.Errorf("waiting for metallb: %w", err)
	}

	ipv4, ipv6, err := getKindNodeIPs(ctx, cfg)
	if err != nil {
		return fmt.Errorf("getting cluster node IPs: %w", err)
	}

	var addresses []string
	if ipv4 != "" {
		addresses = append(addresses, ipv4+"/32")
	}
	if ipv6 != "" {
		addresses = append(addresses, ipv6+"/128")
	}
	if len(addresses) == 0 {
		return fmt.Errorf("could not determine cluster node IP addresses")
	}

	var addrYAML string
	for _, addr := range addresses {
		addrYAML += fmt.Sprintf("    - %s\n", addr)
	}

	fmt.Fprintln(out, "Setting up address pool.")
	manifest := fmt.Sprintf(metalLBPoolTemplate, addrYAML)

	if err := applyManifest(ctx, out, cfg, manifest); err != nil {
		return fmt.Errorf("configuring metallb address pool: %w", err)
	}

	success(out, "Loadbalancer", time.Since(start))
	return nil
}

// getKindNodeIPs inspects the kind control-plane container to extract its
// IPv4 and IPv6 addresses on the "kind" network.
func getKindNodeIPs(ctx context.Context, cfg ClusterConfig) (ipv4, ipv6 string, err error) {
	output, err := runOutput(ctx, cfg.ContainerEngine(), "container", "inspect", cfg.controlPlaneContainer())
	if err != nil {
		return "", "", err
	}

	var results []containerInspectResult
	if err := json.Unmarshal([]byte(output), &results); err != nil {
		return "", "", fmt.Errorf("parsing container inspect output: %w", err)
	}

	if len(results) == 0 {
		return "", "", fmt.Errorf("container %s not found", cfg.controlPlaneContainer())
	}

	kindNet, ok := results[0].NetworkSettings.Networks["kind"]
	if !ok {
		return "", "", fmt.Errorf("network 'kind' not found on container %s", cfg.controlPlaneContainer())
	}

	return kindNet.IPAddress, kindNet.GlobalIPv6Address, nil
}

// containerInspectResult models the relevant fields from docker/podman inspect output.
type containerInspectResult struct {
	NetworkSettings struct {
		Networks map[string]struct {
			IPAddress         string `json:"IPAddress"`
			GlobalIPv6Address string `json:"GlobalIPv6Address"`
		} `json:"Networks"`
	} `json:"NetworkSettings"`
}
