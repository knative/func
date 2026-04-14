package cluster

import (
	"context"
	"fmt"
	"io"
	"time"
)

// installTekton installs Tekton Pipelines and configures RBAC.
func installTekton(ctx context.Context, cfg ClusterConfig, out io.Writer) error {
	start := time.Now()
	status(out, "Installing Tekton")
	fmt.Fprintf(out, "Version: %s\n", tektonVersion)

	tektonRelease := "previous/" + tektonVersion
	namespace := cfg.Namespace

	url := fmt.Sprintf("https://storage.googleapis.com/tekton-releases/pipeline/%s/release.yaml", tektonRelease)
	if err := run(ctx, out, "", cfg.kubectl(), "apply", "-f", url); err != nil {
		return fmt.Errorf("applying tekton: %w", err)
	}

	if err := run(ctx, out, "",
		cfg.kubectl(), "wait", "pod",
		"--for=condition=Ready", "--timeout=180s",
		"-n", "tekton-pipelines", "-l", "app=tekton-pipelines-controller"); err != nil {
		return fmt.Errorf("waiting for tekton controller: %w", err)
	}

	if err := run(ctx, out, "",
		cfg.kubectl(), "wait", "pod",
		"--for=condition=Ready", "--timeout=180s",
		"-n", "tekton-pipelines", "-l", "app=tekton-pipelines-webhook"); err != nil {
		return fmt.Errorf("waiting for tekton webhook: %w", err)
	}

	// RBAC bindings (apply for idempotency)
	rbacBindings := []struct{ name, role string }{
		{namespace + ":knative-serving-namespaced-admin", "knative-serving-namespaced-admin"},
		{namespace + ":admin", "admin"},
		{namespace + ":keda-add-ons-http-operator", "keda-add-ons-http-operator"},
	}
	for _, rb := range rbacBindings {
		manifest := fmt.Sprintf(`apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: %s
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: %s
subjects:
- kind: ServiceAccount
  name: default
  namespace: %s
`, rb.name, rb.role, namespace)
		if err := applyManifest(ctx, out, cfg, manifest); err != nil {
			return fmt.Errorf("applying clusterrolebinding %s: %w", rb.name, err)
		}
	}

	success(out, "Tekton", time.Since(start))
	return nil
}

// installPAC installs Pipelines-as-Code and creates its ingress.
func installPAC(ctx context.Context, cfg ClusterConfig, out io.Writer) error {
	start := time.Now()
	status(out, "Installing Pipelines-as-Code")
	fmt.Fprintf(out, "Version: %s\n", pacVersion)

	url := fmt.Sprintf("https://raw.githubusercontent.com/openshift-pipelines/pipelines-as-code/release-%s/release.k8s.yaml", pacVersion)
	if err := run(ctx, out, "", cfg.kubectl(), "apply", "-f", url); err != nil {
		return fmt.Errorf("applying PAC: %w", err)
	}

	if err := run(ctx, out, "",
		cfg.kubectl(), "wait", "pod",
		"--for=condition=Ready", "-l", "!job-name",
		"-n", "pipelines-as-code", "--timeout=5m"); err != nil {
		return fmt.Errorf("waiting for PAC: %w", err)
	}

	pacIngress := fmt.Sprintf(`apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: pipelines-as-code
  namespace: pipelines-as-code
spec:
  ingressClassName: contour-external
  rules:
  - host: %s
    http:
      paths:
      - backend:
          service:
            name: pipelines-as-code-controller
            port:
              number: 8080
        pathType: Prefix
        path: /
`, cfg.PacHost)

	if err := applyManifest(ctx, out, cfg, pacIngress); err != nil {
		return fmt.Errorf("applying PAC ingress: %w", err)
	}

	fmt.Fprintf(out, "the Pipeline as Code controller is available at: http://%s\n", cfg.PacHost)
	success(out, "PAC", time.Since(start))
	return nil
}
