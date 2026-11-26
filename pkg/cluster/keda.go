package cluster

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/yaml"
)

const (
	kedaHTTPAddonInterceptor = "keda-add-ons-http-interceptor"
	kedaHTTPAddonScaler      = "keda-add-ons-http-scaler"
)

var kedaHTTPAddonReleaseBaseURL = "https://github.com/kedacore/http-add-on/releases/download"

// installKeda installs KEDA core components.
func installKeda(ctx context.Context, cfg ClusterConfig, out io.Writer) error {
	start := time.Now()
	status(out, "Installing Keda")
	fmt.Fprintf(out, "Version: %s\n", kedaVersion)

	kedaVersionNum := strings.TrimPrefix(kedaVersion, "v")

	kedaURL := fmt.Sprintf("https://github.com/kedacore/keda/releases/download/%s/keda-%s.yaml", kedaVersion, kedaVersionNum)
	if err := applyURLServerSide(ctx, out, cfg, kedaURL); err != nil {
		return fmt.Errorf("applying keda: %w", err)
	}

	kedaCoreURL := fmt.Sprintf("https://github.com/kedacore/keda/releases/download/%s/keda-%s-core.yaml", kedaVersion, kedaVersionNum)
	if err := applyURLServerSide(ctx, out, cfg, kedaCoreURL); err != nil {
		return fmt.Errorf("applying keda core: %w", err)
	}

	fmt.Fprintln(out, "Waiting for Keda to become ready")
	if err := run(ctx, out, "",
		cfg.kubectl(), "wait", "deployment",
		"--all", "--timeout=-1s",
		"--for=condition=Available", "--namespace", "keda"); err != nil {
		return fmt.Errorf("waiting for keda: %w", err)
	}

	_ = run(ctx, out, "", cfg.kubectl(), "get", "pod", "-n", "keda")
	success(out, "Keda", time.Since(start))
	return nil
}

// installKedaHTTPAddon installs the KEDA HTTP add-on.
func installKedaHTTPAddon(ctx context.Context, cfg ClusterConfig, out io.Writer) error {
	start := time.Now()
	status(out, "Installing Keda HTTP add-on")
	fmt.Fprintf(out, "Version: %s\n", kedaHTTPAddOnVersion)

	addonVersionNum := strings.TrimPrefix(kedaHTTPAddOnVersion, "v")

	crdsURL := fmt.Sprintf("%s/%s/keda-add-ons-http-%s-crds.yaml",
		kedaHTTPAddonReleaseBaseURL, kedaHTTPAddOnVersion, addonVersionNum)
	if err := applyURLServerSide(ctx, out, cfg, crdsURL); err != nil {
		return fmt.Errorf("applying keda HTTP add-on CRDs: %w", err)
	}

	addonURL := fmt.Sprintf("%s/%s/keda-add-ons-http-%s.yaml",
		kedaHTTPAddonReleaseBaseURL, kedaHTTPAddOnVersion, addonVersionNum)
	if cfg.GitHubActions {
		fmt.Fprintln(out, "Installing Keda HTTP add-on with single-replica settings for CI.")
		addonYAML, err := httpGet(ctx, addonURL)
		if err != nil {
			return fmt.Errorf("downloading keda HTTP add-on YAML: %w", err)
		}
		modifiedYAML, err := reduceKedaHTTPAddonReplicas(addonYAML)
		if err != nil {
			return fmt.Errorf("modifying keda HTTP add-on YAML: %w", err)
		}
		if err := run(ctx, out, modifiedYAML, cfg.kubectl(), "apply", "--server-side", "-f", "-"); err != nil {
			return fmt.Errorf("applying keda HTTP add-on: %w", err)
		}
	} else {
		if err := applyURLServerSide(ctx, out, cfg, addonURL); err != nil {
			return fmt.Errorf("applying keda HTTP add-on: %w", err)
		}
	}

	fmt.Fprintln(out, "Waiting for Keda HTTP add-on to become ready")
	if err := run(ctx, out, "",
		cfg.kubectl(), "wait", "deployment",
		"--all", "--timeout=-1s",
		"--for=condition=Available", "--namespace", "keda"); err != nil {
		return fmt.Errorf("waiting for keda HTTP add-on: %w", err)
	}

	_ = run(ctx, out, "", cfg.kubectl(), "get", "pod", "-n", "keda")
	success(out, "Keda HTTP add-on", time.Since(start))
	return nil
}

// reduceKedaHTTPAddonReplicas sets the interceptor and scaler Deployments to
// 1 replica and lowers the interceptor ScaledObject's minReplicaCount to 1 so
// CI clusters start at a single replica without waiting for HPA scale-down.
// Mirrors the yq pipeline in hack/cluster.sh keda_http_addon().
func reduceKedaHTTPAddonReplicas(yamlContent string) (string, error) {
	var docs []unstructured.Unstructured
	for _, chunk := range splitYAMLDocs(yamlContent) {
		var obj unstructured.Unstructured
		if err := yaml.NewYAMLOrJSONDecoder(strings.NewReader(chunk), 4096).Decode(&obj); err != nil {
			if errors.Is(err, io.EOF) {
				continue
			}
			return "", fmt.Errorf("decoding YAML: %w", err)
		}
		if obj.Object == nil {
			continue
		}

		switch obj.GetKind() {
		case "Deployment":
			if obj.GetName() == kedaHTTPAddonInterceptor || obj.GetName() == kedaHTTPAddonScaler {
				if err := unstructured.SetNestedField(obj.Object, int64(1), "spec", "replicas"); err != nil {
					return "", fmt.Errorf("setting replicas on Deployment/%s: %w", obj.GetName(), err)
				}
			}
		case "ScaledObject":
			if obj.GetName() == kedaHTTPAddonInterceptor {
				if err := unstructured.SetNestedField(obj.Object, int64(1), "spec", "minReplicaCount"); err != nil {
					return "", fmt.Errorf("setting minReplicaCount on ScaledObject/%s: %w", obj.GetName(), err)
				}
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
