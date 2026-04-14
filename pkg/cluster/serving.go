package cluster

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/yaml"
)

// installServing installs Knative Serving CRDs and core components.
func installServing(ctx context.Context, cfg ClusterConfig, out io.Writer) error {
	start := time.Now()
	status(out, "Installing Serving")
	fmt.Fprintf(out, "Version: %s\n", servingVersion)

	// CRDs
	err := run(ctx, out, "",
		cfg.kubectl(), "apply", "--filename",
		fmt.Sprintf("https://github.com/knative/serving/releases/download/knative-%s/serving-crds.yaml", servingVersion))
	if err != nil {
		return fmt.Errorf("applying serving CRDs: %w", err)
	}

	err = run(ctx, out, "",
		cfg.kubectl(), "wait", "--for=condition=Established", "--all", "crd", "--timeout=5m")
	if err != nil {
		return fmt.Errorf("waiting for CRDs: %w", err)
	}

	// Core
	url := fmt.Sprintf("https://github.com/knative/serving/releases/download/knative-%s/serving-core.yaml", servingVersion)
	if err := applyURL(ctx, out, cfg, url); err != nil {
		return fmt.Errorf("applying serving core: %w", err)
	}

	err = run(ctx, out, "",
		cfg.kubectl(), "wait", "pod",
		"--for=condition=Ready", "-l", "!job-name", "-n", "knative-serving", "--timeout=5m")
	if err != nil {
		return fmt.Errorf("waiting for serving pods: %w", err)
	}

	_ = run(ctx, out, "", cfg.kubectl(), "get", "pod", "-A")
	success(out, "Knative Serving", time.Since(start))
	return nil
}

// configureDNS patches the Knative Serving config-domain to use the configured domain.
func configureDNS(ctx context.Context, cfg ClusterConfig, out io.Writer) error {
	start := time.Now()
	status(out, "Configuring DNS")

	var lastErr error
	for i := 0; i < 10; i++ {
		lastErr = run(ctx, out, "", cfg.kubectl(),
			"patch", "configmap/config-domain",
			"--namespace", "knative-serving",
			"--type", "merge",
			"--patch", fmt.Sprintf(`{"data":{"%s":""}}`, cfg.Domain))
		if lastErr == nil {
			success(out, "DNS", time.Since(start))
			return nil
		}
		fmt.Fprintln(out, "Retrying...")
		if err := wait(ctx, 5*time.Second); err != nil {
			return err
		}
	}
	return fmt.Errorf("unable to set Knative domain after 10 attempts: %w", lastErr)
}

// installNetworking installs Contour ingress controller and configures Knative
// to use it. The Contour YAML is modified in Go (replacing yq) to add IPv6
// dual-stack support args.
func installNetworking(ctx context.Context, cfg ClusterConfig, out io.Writer) error {
	start := time.Now()
	status(out, "Installing Ingress Controller (Contour)")
	fmt.Fprintf(out, "Version: %s\n", contourVersion)

	fmt.Fprintln(out, "Installing a configured Contour.")
	contourURL := fmt.Sprintf("https://github.com/knative/net-contour/releases/download/knative-%s/contour.yaml", contourVersion)
	contourYAML, err := httpGet(ctx, contourURL)
	if err != nil {
		return fmt.Errorf("downloading contour YAML: %w", err)
	}

	modifiedYAML, err := addContourIPv6Args(contourYAML)
	if err != nil {
		return fmt.Errorf("modifying contour YAML: %w", err)
	}

	if err := run(ctx, out, modifiedYAML, cfg.kubectl(), "apply", "-f", "-"); err != nil {
		return fmt.Errorf("applying contour: %w", err)
	}

	err = run(ctx, out, "",
		cfg.kubectl(), "wait", "pod",
		"--for=condition=Ready", "-l", "!job-name",
		"-n", "contour-external", "--timeout=10m")
	if err != nil {
		return fmt.Errorf("waiting for contour pods: %w", err)
	}

	fmt.Fprintln(out, "Installing the Knative Contour controller.")
	netContourURL := fmt.Sprintf("https://github.com/knative/net-contour/releases/download/knative-%s/net-contour.yaml", contourVersion)
	if err := run(ctx, out, "", cfg.kubectl(), "apply", "-f", netContourURL); err != nil {
		return fmt.Errorf("applying net-contour: %w", err)
	}

	err = run(ctx, out, "",
		cfg.kubectl(), "wait", "pod",
		"--for=condition=Ready", "-l", "!job-name",
		"-n", "knative-serving", "--timeout=10m")
	if err != nil {
		return fmt.Errorf("waiting for net-contour pods: %w", err)
	}

	fmt.Fprintln(out, "Configuring Knative Serving to use Contour.")
	err = run(ctx, out, "",
		cfg.kubectl(), "patch", "configmap/config-network",
		"--namespace", "knative-serving",
		"--type", "merge",
		"--patch", `{"data":{"ingress-class":"contour.ingress.networking.knative.dev"}}`)
	if err != nil {
		return fmt.Errorf("configuring contour ingress: %w", err)
	}

	fmt.Fprintln(out, "Patch domain-template")
	err = run(ctx, out, "",
		cfg.kubectl(), "patch", "-n", "knative-serving", "cm/config-network",
		"--patch", `{"data":{"domain-template":"{{.Name}}-{{.Namespace}}-ksvc.{{.Domain}}"}}`)
	if err != nil {
		return fmt.Errorf("patching domain-template: %w", err)
	}

	fmt.Fprintln(out, "Patching contour to prefer dual-stack")
	err = run(ctx, out, "",
		cfg.kubectl(), "patch", "-n", "contour-external", "svc/envoy",
		"--type", "merge",
		"--patch", `{"spec":{"ipFamilyPolicy":"PreferDualStack"}}`)
	if err != nil {
		return fmt.Errorf("patching contour dual-stack: %w", err)
	}

	err = run(ctx, out, "",
		cfg.kubectl(), "wait", "pod", "--for=condition=Ready", "-l", "!job-name",
		"-n", "contour-external", "--timeout=10m")
	if err != nil {
		return fmt.Errorf("waiting for contour: %w", err)
	}

	err = run(ctx, out, "",
		cfg.kubectl(), "wait", "pod", "--for=condition=Ready", "-l", "!job-name",
		"-n", "knative-serving", "--timeout=10m")
	if err != nil {
		return fmt.Errorf("waiting for serving: %w", err)
	}

	success(out, "Ingress", time.Since(start))
	return nil
}

// addContourIPv6Args modifies the Contour deployment YAML to add
// --envoy-service-http-address=:: and --envoy-service-https-address=:: args.
// This replaces the yq pipeline from the shell script. The input is split
// on the multi-document separator and each non-empty chunk is decoded
// individually, which avoids the k8s YAML decoder's quirk of returning a
// string-compared "Object 'Kind' is missing" error for empty documents.
func addContourIPv6Args(yamlContent string) (string, error) {
	var docs []unstructured.Unstructured
	for _, chunk := range splitYAMLDocs(yamlContent) {
		var obj unstructured.Unstructured
		if err := yaml.NewYAMLOrJSONDecoder(strings.NewReader(chunk), 4096).Decode(&obj); err != nil {
			return "", fmt.Errorf("decoding YAML: %w", err)
		}
		if obj.Object == nil {
			continue
		}

		if obj.GetKind() == "Deployment" && obj.GetName() == "contour" {
			containers, found, err := unstructured.NestedSlice(obj.Object, "spec", "template", "spec", "containers")
			if err == nil && found && len(containers) > 0 {
				container := containers[0].(map[string]any)
				argsRaw, _, _ := unstructured.NestedStringSlice(container, "args")
				args := append(argsRaw,
					"--envoy-service-http-address=::",
					"--envoy-service-https-address=::",
				)
				container["args"] = toAnySlice(args)
				containers[0] = container
				_ = unstructured.SetNestedSlice(obj.Object, containers, "spec", "template", "spec", "containers")
			}
		}

		docs = append(docs, obj)
	}

	var buf bytes.Buffer
	for i, doc := range docs {
		if i > 0 {
			buf.WriteString("---\n")
		}
		b, err := doc.MarshalJSON()
		if err != nil {
			return "", fmt.Errorf("marshaling document: %w", err)
		}
		buf.Write(b)
		buf.WriteByte('\n')
	}

	return buf.String(), nil
}

// splitYAMLDocs splits a multi-document YAML string on the "---" separator
// and returns non-empty documents. Whitespace-only chunks are skipped.
func splitYAMLDocs(content string) []string {
	var docs []string
	for _, chunk := range strings.Split(content, "\n---") {
		if strings.TrimSpace(chunk) == "" {
			continue
		}
		docs = append(docs, chunk)
	}
	return docs
}

func toAnySlice(ss []string) []any {
	result := make([]any, len(ss))
	for i, s := range ss {
		result[i] = s
	}
	return result
}
