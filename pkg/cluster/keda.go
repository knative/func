package cluster

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"
)

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

	crdsURL := fmt.Sprintf("https://github.com/kedacore/http-add-on/releases/download/%s/keda-add-ons-http-%s-crds.yaml",
		kedaHTTPAddOnVersion, addonVersionNum)
	if err := applyURLServerSide(ctx, out, cfg, crdsURL); err != nil {
		return fmt.Errorf("applying keda HTTP add-on CRDs: %w", err)
	}

	addonURL := fmt.Sprintf("https://github.com/kedacore/http-add-on/releases/download/%s/keda-add-ons-http-%s.yaml",
		kedaHTTPAddOnVersion, addonVersionNum)
	if err := applyURLServerSide(ctx, out, cfg, addonURL); err != nil {
		return fmt.Errorf("applying keda HTTP add-on: %w", err)
	}

	fmt.Fprintln(out, "Waiting for Keda HTTP add-on to become ready")
	if err := run(ctx, out, "",
		cfg.kubectl(), "wait", "deployment",
		"--all", "--timeout=-1s",
		"--for=condition=Available", "--namespace", "keda"); err != nil {
		return fmt.Errorf("waiting for keda HTTP add-on: %w", err)
	}

	// Reduce resource requests for CI environments
	if cfg.GitHubActions {
		status(out, "Reducing KEDA HTTP add-on resource requests for CI")

		_ = run(ctx, out, "",
			cfg.kubectl(), "scale", "deployment", "keda-add-ons-http-interceptor",
			"-n", "keda", "--replicas=1")
		_ = run(ctx, out, "",
			cfg.kubectl(), "scale", "deployment", "keda-add-ons-http-scaler",
			"-n", "keda", "--replicas=1")

		fmt.Fprintln(out, green("✅ Resource requests reduced for CI"))
	}

	_ = run(ctx, out, "", cfg.kubectl(), "get", "pod", "-n", "keda")
	success(out, "Keda HTTP add-on", time.Since(start))
	return nil
}
