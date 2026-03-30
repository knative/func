//go:build e2e
// +build e2e

package e2e

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"

	wasmclientset "github.com/cardil/knative-serving-wasm/pkg/client/clientset/versioned"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"knative.dev/func/pkg/k8s"

	fn "knative.dev/func/pkg/functions"
	"knative.dev/func/pkg/wasm"
)

// wasmList returns the list of deployed WASM functions in namespace using the
// wasm lister directly (bypasses the CLI).
func wasmList(t *testing.T, namespace string) []fn.ListItem {
	t.Helper()
	client := fn.New(fn.WithListers(wasm.NewLister()))
	list, err := client.List(t.Context(), namespace)
	if err != nil {
		t.Fatalf("wasm list failed: %v", err)
	}
	return list
}

// wasmModuleUrl queries the WasmModule CR status to get the URL from
// .status.address.url. This is future-proof — works regardless of whether
// the controller uses ksvc, a Deployment, or a shared runner. Adjusts port
// for the e2e ingress (HTTPPort) since the status URL uses port 80 but the
// Kind cluster ingress listens on 8080.
func wasmModuleUrl(t *testing.T, name, namespace string) string {
	t.Helper()

	restConfig, err := k8s.GetClientConfig().ClientConfig()
	if err != nil {
		t.Fatalf("wasmModuleUrl: failed to get rest config: %v", err)
	}
	cs, err := wasmclientset.NewForConfig(restConfig)
	if err != nil {
		t.Fatalf("wasmModuleUrl: failed to create wasm clientset: %v", err)
	}

	module, err := cs.WasmV1alpha1().WasmModules(namespace).Get(t.Context(), name, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("wasmModuleUrl: failed to get WasmModule %s/%s: %v", namespace, name, err)
	}

	if module.Status.Address == nil || module.Status.Address.URL == nil {
		t.Fatalf("wasmModuleUrl: WasmModule %s/%s has no status address URL", namespace, name)
	}

	rawURL := module.Status.Address.URL.String()

	// Replace port 80 with HTTPPort when the cluster ingress uses a non-standard port.
	if HTTPPort != "" && HTTPPort != "80" {
		// The status URL is http://host or http://host:80 — normalise to http://host:HTTPPort
		rawURL = strings.TrimSuffix(rawURL, ":80")
		// Insert port before any path
		scheme := "http://"
		if strings.HasPrefix(rawURL, "https://") {
			scheme = "https://"
		}
		host := strings.TrimPrefix(strings.TrimPrefix(rawURL, "https://"), "http://")
		// host may contain a path; split on first "/"
		path := ""
		if idx := strings.Index(host, "/"); idx >= 0 {
			path = host[idx:]
			host = host[:idx]
		}
		rawURL = scheme + host + ":" + HTTPPort + path
	}

	return rawURL
}

// isLocalRegistry returns true when the registry is the default localhost
// registry, meaning WASM runner pods cannot reach it from inside the cluster.
func isLocalRegistry() bool {
	return Registry == DefaultRegistry
}

// rewriteImageRegistry replaces the localhost registry prefix in the
// function's func.yaml image fields with ClusterRegistry so that WASM runner
// pods can fetch the OCI artifact from within the cluster.
func rewriteImageRegistry(t *testing.T) {
	t.Helper()

	f, err := fn.NewFunction(".")
	if err != nil {
		t.Fatalf("rewriteImageRegistry: failed to load func.yaml: %v", err)
	}

	// Replace the localhost registry prefix with the cluster-internal prefix.
	replace := func(img string) string {
		if strings.HasPrefix(img, Registry+"/") {
			return ClusterRegistry + "/" + strings.TrimPrefix(img, Registry+"/")
		}
		// Also handle the case where Registry itself is the prefix without trailing slash.
		if strings.HasPrefix(img, Registry) {
			return ClusterRegistry + strings.TrimPrefix(img, Registry)
		}
		return img
	}

	f.Build.Image = replace(f.Build.Image)
	f.Deploy.Image = replace(f.Deploy.Image)
	f.Image = replace(f.Image)

	if err := f.Write(); err != nil {
		t.Fatalf("rewriteImageRegistry: failed to write func.yaml: %v", err)
	}
}

// configureRunnerInsecureRegistries creates (or updates) the config-runner
// ConfigMap in the knative-wasm namespace so that the WASM runner controller
// injects INSECURE_REGISTRIES into runner pods.  This is needed when using
// the local in-cluster registry which serves plain HTTP.
//
// The ConfigMap is created idempotently — safe to call from multiple tests.
func configureRunnerInsecureRegistries(t *testing.T) {
	t.Helper()

	k8sClient, err := k8s.NewKubernetesClientset()
	if err != nil {
		t.Fatalf("configureRunnerInsecureRegistries: failed to create k8s client: %v", err)
	}

	// Extract the registry host (without the path/namespace suffix).
	// DefaultClusterRegistry = "registry.default.svc.cluster.local:5000/func"
	// We need just "registry.default.svc.cluster.local:5000".
	clusterHost := strings.SplitN(DefaultClusterRegistry, "/", 2)[0]
	localHost := strings.SplitN(DefaultRegistry, "/", 2)[0]

	data := map[string]string{
		"insecure-registries": fmt.Sprintf("- %s\n- %s\n", clusterHost, localHost),
	}

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "config-runner",
			Namespace: "knative-wasm",
		},
		Data: data,
	}

	ctx := t.Context()
	cms := k8sClient.CoreV1().ConfigMaps("knative-wasm")

	existing, err := cms.Get(ctx, "config-runner", metav1.GetOptions{})
	if errors.IsNotFound(err) {
		if _, err := cms.Create(ctx, cm, metav1.CreateOptions{}); err != nil {
			t.Fatalf("configureRunnerInsecureRegistries: failed to create ConfigMap: %v", err)
		}
		t.Log("configureRunnerInsecureRegistries: created config-runner ConfigMap")
		return
	}
	if err != nil {
		t.Fatalf("configureRunnerInsecureRegistries: failed to get ConfigMap: %v", err)
	}

	existing.Data = data
	if _, err := cms.Update(ctx, existing, metav1.UpdateOptions{}); err != nil {
		t.Fatalf("configureRunnerInsecureRegistries: failed to update ConfigMap: %v", err)
	}
	t.Log("configureRunnerInsecureRegistries: updated config-runner ConfigMap")
}

// deployWasm encapsulates the two-mode deploy logic for WASM functions.
// When the registry is a localhost registry (not reachable from inside the
// cluster), it builds and pushes first, configures the runner insecure
// registry list, rewrites the image reference in func.yaml to use the
// cluster-internal registry, then deploys without rebuilding.
// When an external registry is used, it simply runs func deploy.
// Returns an error rather than calling t.Fatal so callers can assert on
// expected failures (e.g. TestWasm_GoDeploy).
func deployWasm(t *testing.T) error {
	t.Helper()
	if isLocalRegistry() {
		if err := newCmd(t, "build", "--push").Run(); err != nil {
			return fmt.Errorf("build --push: %w", err)
		}
		configureRunnerInsecureRegistries(t)
		rewriteImageRegistry(t)
		return newCmd(t, "deploy", "--build=false", "--push=false").Run()
	}
	return newCmd(t, "deploy").Run()
}

// requireRustWasi skips the test if the rust-wasi toolchain is unavailable.
func requireRustWasi(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("cargo"); err != nil {
		t.Skip("cargo not found on PATH; skipping rust-wasi test")
	}
	if out, err := exec.Command("rustup", "target", "list", "--installed").Output(); err != nil {
		t.Logf("warn: cannot query rustup targets: %v", err)
	} else if !strings.Contains(string(out), "wasm32-wasip2") {
		t.Skip("wasm32-wasip2 rustup target not installed; run: rustup target add wasm32-wasip2")
	}
}

// requireGoWasi skips the test if the go-wasi toolchain is unavailable.
func requireGoWasi(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("tinygo"); err != nil {
		t.Skip("tinygo not found on PATH; skipping go-wasi test")
	}
	if _, err := exec.LookPath("wasm-opt"); err != nil {
		t.Skip("wasm-opt not found on PATH (install binaryen https://github.com/WebAssembly/binaryen); skipping go-wasi test")
	}
	if _, err := exec.LookPath("wasm-tools"); err != nil {
		t.Skip("wasm-tools not found on PATH (install from https://github.com/bytecodealliance/wasm-tools); skipping go-wasi test")
	}
	tinygoOut, _ := exec.Command("tinygo", "env", "TINYGOROOT").Output()
	if tinygoOut != nil {
		witDir := strings.TrimSpace(string(tinygoOut)) + "/lib/wasi-cli/wit"
		if _, err := os.Stat(witDir); err != nil {
			t.Skipf("tinygo WIT files not found at %s; install tinygo from https://tinygo.org for wasip2 support", witDir)
		}
	}
}

// TestWasm_RustBuild verifies that a rust-wasi function can be built end-to-end.
// The "wasm" builder is inferred automatically from the rust-wasi runtime; no
// --builder flag is required.
//
// Prerequisites (skipped automatically when absent):
//   - cargo must be on PATH
//   - wasm32-wasip2 rustup target must be installed: rustup target add wasm32-wasip2
//   - A registry reachable at FUNC_E2E_REGISTRY (default: localhost:50000/func)
func TestWasm_RustBuild(t *testing.T) {
	requireRustWasi(t)

	name := "func-e2e-wasm-rust-build"
	_ = fromCleanEnv(t, name)

	if err := newCmd(t, "init", "-l", wasm.RuntimeRustWasi, "-t", "http").Run(); err != nil {
		t.Fatalf("func init failed: %v", err)
	}

	if err := newCmd(t, "build", "--verbose").Run(); err != nil {
		t.Fatalf("func build failed for rust-wasi: %v", err)
	}

	// Second build must also succeed — verifies that the inferred builder "wasm"
	// is NOT written to func.yaml (which would set BuilderExplicit=true on the
	// next invocation and break inference by locking in "wasm" explicitly).
	if err := newCmd(t, "build").Run(); err != nil {
		t.Fatalf("second func build failed for rust-wasi: %v", err)
	}
}

// TestWasm_GoBuild verifies that a go-wasi function can be built end-to-end.
// The "wasm" builder is inferred automatically from the go-wasi runtime; no
// --builder flag is required.
//
// Prerequisites (skipped automatically when absent):
//   - tinygo must be on PATH (install from https://tinygo.org)
//   - A registry reachable at FUNC_E2E_REGISTRY (default: localhost:50000/func)
func TestWasm_GoBuild(t *testing.T) {
	requireGoWasi(t)

	name := "func-e2e-wasm-go-build"
	_ = fromCleanEnv(t, name)

	if err := newCmd(t, "init", "-l", wasm.RuntimeGoWasi, "-t", "http").Run(); err != nil {
		t.Fatalf("func init failed: %v", err)
	}

	if err := newCmd(t, "build", "--verbose").Run(); err != nil {
		t.Fatalf("func build failed for go-wasi: %v", err)
	}

	// Second build must also succeed — verifies that the inferred builder "wasm"
	// is NOT written to func.yaml (which would set BuilderExplicit=true on the
	// next invocation and break inference by locking in "wasm" explicitly).
	if err := newCmd(t, "build").Run(); err != nil {
		t.Fatalf("second func build failed for go-wasi: %v", err)
	}
}

// TestWasm_RustDeploy verifies the full build→push→deploy cycle for a rust-wasi
// function.  It deploys to the cluster via "func deploy", confirms the WasmModule
// CR is listed, verifies HTTP liveness, re-deploys to exercise the update path,
// and cleans up.
//
// Prerequisites (skipped automatically when absent):
//   - cargo must be on PATH
//   - wasm32-wasip2 rustup target must be installed
//   - A reachable registry (FUNC_E2E_REGISTRY)
//   - A Kubernetes cluster with the WasmModule CRD installed
func TestWasm_RustDeploy(t *testing.T) {
	requireRustWasi(t)

	name := "func-e2e-wasm-rust-deploy"
	_ = fromCleanEnv(t, name)

	if err := newCmd(t, "init", "-l", wasm.RuntimeRustWasi, "-t", "http").Run(); err != nil {
		t.Fatalf("func init failed: %v", err)
	}

	if err := deployWasm(t); err != nil {
		t.Fatalf("func deploy failed for rust-wasi: %v", err)
	}
	defer func() {
		clean(t, name, Namespace)
	}()

	if !containsInstance(t, wasmList(t, Namespace), name, Namespace) {
		t.Fatal("deployed rust-wasi function not found in func list")
	}

	// Verify the function is actually serving HTTP traffic.
	url := wasmModuleUrl(t, name, Namespace)
	if !waitFor(t, url, withContentMatch("Hello from WASI!")) {
		t.Fatal("rust-wasi function did not become reachable after deploy")
	}

	// Re-deploy to exercise the update path (WasmModule already exists).
	if err := newCmd(t, "deploy", "--build=false", "--push=false").Run(); err != nil {
		t.Fatalf("second func deploy (update) failed for rust-wasi: %v", err)
	}

	// Verify liveness again after update.
	if !waitFor(t, url, withContentMatch("Hello from WASI!")) {
		t.Fatal("rust-wasi function did not become reachable after re-deploy")
	}
}

// TestWasm_GoDeploy verifies the full build→push→deploy cycle for a go-wasi
// function.  The go-wasi template currently produces a broken WASM module
// (Issue 1: no exported `wasi:http/incoming-handler@0.2.3` instance), so this
// test asserts that the deploy fails.  When Issue 1 is resolved this test
// should be updated to expect success and add an HTTP liveness check.
//
// Prerequisites (skipped automatically when absent):
//   - tinygo + wasm-opt + wasm-tools must be on PATH
//   - A reachable registry (FUNC_E2E_REGISTRY)
//   - A Kubernetes cluster with the WasmModule CRD installed
func TestWasm_GoDeploy(t *testing.T) {
	requireGoWasi(t)

	name := "func-e2e-wasm-go-deploy"
	_ = fromCleanEnv(t, name)

	if err := newCmd(t, "init", "-l", wasm.RuntimeGoWasi, "-t", "http").Run(); err != nil {
		t.Fatalf("func init failed: %v", err)
	}

	// Issue 1: go-wasi template produces a broken WASM module; deploy must fail.
	err := deployWasm(t)
	if err == nil {
		// If deploy unexpectedly succeeded, clean up before failing the test.
		defer clean(t, name, Namespace)
		t.Fatal("expected go-wasi deploy to fail (Issue 1: no exported wasi:http/incoming-handler@0.2.3 instance); " +
			"if this template has been fixed, update this test to expect success and add an HTTP liveness check")
	}
	t.Logf("go-wasi deploy failed as expected (reproducing Issue 1): %v", err)
}

// TestWasm_RustDescribe verifies that "func describe" returns meaningful output
// for a deployed rust-wasi function.
//
// Prerequisites: same as TestWasm_RustDeploy.
func TestWasm_RustDescribe(t *testing.T) {
	requireRustWasi(t)

	name := "func-e2e-wasm-rust-describe"
	_ = fromCleanEnv(t, name)

	if err := newCmd(t, "init", "-l", wasm.RuntimeRustWasi, "-t", "http").Run(); err != nil {
		t.Fatalf("func init failed: %v", err)
	}

	if err := deployWasm(t); err != nil {
		t.Fatalf("func deploy failed: %v", err)
	}
	defer func() {
		clean(t, name, Namespace)
	}()

	// Verify the function is live before describing it.
	url := wasmModuleUrl(t, name, Namespace)
	if !waitFor(t, url, withContentMatch("Hello from WASI!")) {
		t.Fatal("rust-wasi function did not become reachable before describe")
	}

	if err := newCmd(t, "describe").Run(); err != nil {
		t.Fatalf("func describe failed for rust-wasi: %v", err)
	}
}

// TestWasm_RustDelete verifies that "func delete" removes a deployed rust-wasi
// function from the cluster.
//
// Prerequisites: same as TestWasm_RustDeploy.
func TestWasm_RustDelete(t *testing.T) {
	requireRustWasi(t)

	name := "func-e2e-wasm-rust-delete"
	_ = fromCleanEnv(t, name)

	if err := newCmd(t, "init", "-l", wasm.RuntimeRustWasi, "-t", "http").Run(); err != nil {
		t.Fatalf("func init failed: %v", err)
	}

	if err := deployWasm(t); err != nil {
		t.Fatalf("func deploy failed: %v", err)
	}

	if !containsInstance(t, wasmList(t, Namespace), name, Namespace) {
		t.Fatal("function not listed after deploy")
	}

	// Verify the function is live before deleting it.
	url := wasmModuleUrl(t, name, Namespace)
	if !waitFor(t, url, withContentMatch("Hello from WASI!")) {
		t.Fatal("rust-wasi function did not become reachable before delete")
	}

	if err := newCmd(t, "delete", name, "--namespace", Namespace).Run(); err != nil {
		t.Fatalf("func delete failed for rust-wasi: %v", err)
	}

	if containsInstance(t, wasmList(t, Namespace), name, Namespace) {
		t.Fatal("function still listed after delete")
	}
}
