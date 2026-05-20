package cluster

import (
	"context"
	"fmt"
	"io"
	"time"
)

// installEventing installs Knative Eventing CRDs, core, in-memory channel, and broker.
func installEventing(ctx context.Context, cfg ClusterConfig, out io.Writer) error {
	start := time.Now()
	status(out, "Installing Eventing")
	fmt.Fprintf(out, "Version: %s\n", eventingVersion)

	baseURL := fmt.Sprintf("https://github.com/knative/eventing/releases/download/knative-%s", eventingVersion)

	// CRDs
	if err := run(ctx, out, "", cfg.kubectl(), "apply", "-f", baseURL+"/eventing-crds.yaml"); err != nil {
		return fmt.Errorf("applying eventing CRDs: %w", err)
	}

	if err := run(ctx, out, "", cfg.kubectl(), "wait", "--for=condition=Established", "--all", "crd", "--timeout=5m"); err != nil {
		return fmt.Errorf("waiting for eventing CRDs: %w", err)
	}

	// Core
	if err := applyURL(ctx, out, cfg, baseURL+"/eventing-core.yaml"); err != nil {
		return fmt.Errorf("applying eventing core: %w", err)
	}
	if err := run(ctx, out, "",
		cfg.kubectl(), "wait", "pod",
		"--for=condition=Ready", "-l", "!job-name",
		"-n", "knative-eventing", "--timeout=5m"); err != nil {
		return fmt.Errorf("waiting for eventing core: %w", err)
	}

	// In-memory channel
	if err := applyURL(ctx, out, cfg, baseURL+"/in-memory-channel.yaml"); err != nil {
		return fmt.Errorf("applying in-memory channel: %w", err)
	}
	if err := run(ctx, out, "",
		cfg.kubectl(), "wait", "pod",
		"--for=condition=Ready", "-l", "!job-name",
		"-n", "knative-eventing", "--timeout=5m"); err != nil {
		return fmt.Errorf("waiting for in-memory channel: %w", err)
	}

	// MT channel broker
	if err := applyURL(ctx, out, cfg, baseURL+"/mt-channel-broker.yaml"); err != nil {
		return fmt.Errorf("applying mt-channel-broker: %w", err)
	}
	if err := run(ctx, out, "",
		cfg.kubectl(), "wait", "pod",
		"--for=condition=Ready", "-l", "!job-name",
		"-n", "knative-eventing", "--timeout=5m"); err != nil {
		return fmt.Errorf("waiting for mt-channel-broker: %w", err)
	}

	// Broker ingress
	fmt.Fprintf(out, "Exposing broker at broker.%s\n", cfg.Domain)
	brokerIngress := fmt.Sprintf(`apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: broker-ingress
  namespace: knative-eventing
spec:
  ingressClassName: contour-external
  rules:
    - host: broker.%s
      http:
        paths:
          - backend:
              service:
                name: broker-ingress
                port:
                  number: 80
            pathType: Prefix
            path: /
`, cfg.Domain)

	if err := applyManifest(ctx, out, cfg, brokerIngress); err != nil {
		return fmt.Errorf("applying broker ingress: %w", err)
	}

	success(out, "Eventing", time.Since(start))
	return nil
}

// configureNamespace creates the "func" namespace with a default broker and channel config.
func configureNamespace(ctx context.Context, cfg ClusterConfig, out io.Writer) error {
	start := time.Now()
	status(out, `Configuring Namespace "func"`)

	// Create Namespace (apply for idempotency)
	namespace := `apiVersion: v1
kind: Namespace
metadata:
  name: func
`
	if err := applyManifest(ctx, out, cfg, namespace); err != nil {
		return fmt.Errorf("creating func namespace: %w", err)
	}

	// Default Broker
	broker := `apiVersion: eventing.knative.dev/v1
kind: Broker
metadata:
  name: func-broker
  namespace: func
`
	if err := applyManifest(ctx, out, cfg, broker); err != nil {
		return fmt.Errorf("applying func broker: %w", err)
	}

	// Default Channel
	channel := `apiVersion: v1
kind: ConfigMap
metadata:
  name: imc-channel
  namespace: knative-eventing
data:
  channelTemplateSpec: |
    apiVersion: messaging.knative.dev/v1
    kind: InMemoryChannel
`
	if err := applyManifest(ctx, out, cfg, channel); err != nil {
		return fmt.Errorf("applying imc-channel: %w", err)
	}

	// Connect Default Broker->Channel
	brokerDefaults := `apiVersion: v1
kind: ConfigMap
metadata:
  name: config-br-defaults
  namespace: knative-eventing
data:
  default-br-config: |
    # This is the cluster-wide default broker channel.
    clusterDefault:
      brokerClass: MTChannelBasedBroker
      apiVersion: v1
      kind: ConfigMap
      name: imc-channel
      namespace: knative-eventing
`
	if err := applyManifest(ctx, out, cfg, brokerDefaults); err != nil {
		return fmt.Errorf("applying broker defaults: %w", err)
	}

	success(out, "Namespace", time.Since(start))
	return nil
}
