package wasm_test

import (
	"context"
	"errors"
	"net/url"
	"testing"
	"time"

	wasmv1alpha1 "github.com/cardil/knative-serving-wasm/pkg/apis/wasm/v1alpha1"
	wasmclientset "github.com/cardil/knative-serving-wasm/pkg/client/clientset/versioned"
	fakewasm "github.com/cardil/knative-serving-wasm/pkg/client/clientset/versioned/fake"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	knapis "knative.dev/pkg/apis"
	duckv1 "knative.dev/pkg/apis/duck/v1"

	"knative.dev/func/pkg/deployer"
	fn "knative.dev/func/pkg/functions"
	"knative.dev/func/pkg/wasm"
)

// fakeProvider returns a wasm.ClientsetProvider backed by the given fake clientset.
func fakeProvider(cs wasmclientset.Interface) wasm.ClientsetProvider {
	return func() (wasmclientset.Interface, error) {
		return cs, nil
	}
}

// newFakeClientset creates a fake clientset seeded with the given objects.
//
// NewSimpleClientset is used here because NewClientset (with field management) is only
// available when apply configurations are generated, which is not the case for this dependency.
func newFakeClientset(objs ...runtime.Object) *fakewasm.Clientset {
	return fakewasm.NewSimpleClientset(objs...) //nolint:staticcheck
}

// minimalFunction builds a minimal fn.Function suitable for deployer tests.
func minimalFunction(name, namespace, image string) fn.Function {
	return fn.Function{
		Name:    name,
		Runtime: wasm.RuntimeRustWasi,
		Deploy: fn.DeploySpec{
			Namespace: namespace,
			Image:     image,
		},
	}
}

// TestDeploy_Create verifies that deploying a new function creates a WasmModule.
func TestDeploy_Create(t *testing.T) {
	t.Parallel()
	cs := newFakeClientset()
	d := wasm.NewDeployer(
		wasm.WithDeployerClientsetProvider(fakeProvider(cs)),
		wasm.WithDeployerWaitTimeout(0), // skip polling in unit tests
	)

	f := minimalFunction("my-func", "default", "registry.example.com/my-func:latest")
	result, err := d.Deploy(context.Background(), f)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Status != fn.Deployed {
		t.Errorf("expected Deployed, got %v", result.Status)
	}
	if result.Namespace != "default" {
		t.Errorf("expected namespace 'default', got %q", result.Namespace)
	}

	// Verify WasmModule was created.
	wm, err := cs.WasmV1alpha1().WasmModules("default").Get(context.Background(), "my-func", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("WasmModule not found after deploy: %v", err)
	}
	if wm.Spec.Image != "registry.example.com/my-func:latest" {
		t.Errorf("unexpected image %q", wm.Spec.Image)
	}
	if wm.Annotations[deployer.DeployerNameAnnotation] != wasm.DeployerName {
		t.Errorf("deployer annotation not set; got %q", wm.Annotations[deployer.DeployerNameAnnotation])
	}
}

// TestDeploy_Update verifies that deploying an existing function updates the WasmModule.
func TestDeploy_Update(t *testing.T) {
	t.Parallel()
	existing := &wasmv1alpha1.WasmModule{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-func",
			Namespace: "default",
			Annotations: map[string]string{
				deployer.DeployerNameAnnotation: wasm.DeployerName,
			},
		},
		Spec: wasmv1alpha1.WasmModuleSpec{
			Image: "registry.example.com/my-func:v1",
		},
	}
	cs := newFakeClientset(existing)
	d := wasm.NewDeployer(
		wasm.WithDeployerClientsetProvider(fakeProvider(cs)),
		wasm.WithDeployerWaitTimeout(0), // skip polling in unit tests
	)

	f := minimalFunction("my-func", "default", "registry.example.com/my-func:v2")
	result, err := d.Deploy(context.Background(), f)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Status != fn.Updated {
		t.Errorf("expected Updated, got %v", result.Status)
	}

	wm, err := cs.WasmV1alpha1().WasmModules("default").Get(context.Background(), "my-func", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("WasmModule not found: %v", err)
	}
	if wm.Spec.Image != "registry.example.com/my-func:v2" {
		t.Errorf("image not updated; got %q", wm.Spec.Image)
	}
}

// TestDeploy_ErrNamespaceRequired verifies that a missing namespace returns the sentinel error.
func TestDeploy_ErrNamespaceRequired(t *testing.T) {
	t.Parallel()
	cs := newFakeClientset()
	d := wasm.NewDeployer(
		wasm.WithDeployerClientsetProvider(fakeProvider(cs)),
		wasm.WithDeployerWaitTimeout(0), // skip polling in unit tests
	)

	f := fn.Function{Name: "my-func", Runtime: wasm.RuntimeRustWasi}
	f.Deploy.Image = "registry.example.com/my-func:latest"

	_, err := d.Deploy(context.Background(), f)
	if !errors.Is(err, fn.ErrNamespaceRequired) {
		t.Errorf("expected ErrNamespaceRequired, got: %v", err)
	}
}

// TestDeploy_ErrNoImageRef verifies that a missing image returns the sentinel error.
func TestDeploy_ErrNoImageRef(t *testing.T) {
	t.Parallel()
	cs := newFakeClientset()
	d := wasm.NewDeployer(
		wasm.WithDeployerClientsetProvider(fakeProvider(cs)),
		wasm.WithDeployerWaitTimeout(0), // skip polling in unit tests
	)

	f := fn.Function{Name: "my-func", Runtime: wasm.RuntimeRustWasi}
	f.Deploy.Namespace = "default"
	// No image set.

	_, err := d.Deploy(context.Background(), f)
	if !errors.Is(err, wasm.ErrNoImageRef) {
		t.Errorf("expected ErrNoImageRef, got: %v", err)
	}
}

// TestDeploy_Network verifies that deploy.network is mapped to WasmModule spec.
func TestDeploy_Network(t *testing.T) {
	t.Parallel()
	cs := newFakeClientset()
	d := wasm.NewDeployer(
		wasm.WithDeployerClientsetProvider(fakeProvider(cs)),
		wasm.WithDeployerWaitTimeout(0), // skip polling in unit tests
	)

	allowDNS := true
	f := minimalFunction("net-func", "default", "registry.example.com/net-func:latest")
	f.Deploy.Network = &fn.NetworkSpec{
		AllowIpNameLookup: &allowDNS,
		Tcp: &fn.TcpNetworkSpec{
			Connect: []string{"*:443"},
		},
	}

	_, err := d.Deploy(context.Background(), f)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	wm, err := cs.WasmV1alpha1().WasmModules("default").Get(context.Background(), "net-func", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("WasmModule not found: %v", err)
	}
	if wm.Spec.Network == nil {
		t.Fatal("expected spec.network to be set")
	}
	if wm.Spec.Network.AllowIPNameLookup == nil || !*wm.Spec.Network.AllowIPNameLookup {
		t.Error("expected AllowIPNameLookup to be true")
	}
	if wm.Spec.Network.TCP == nil || len(wm.Spec.Network.TCP.Connect) != 1 {
		t.Error("expected TCP connect to be set")
	}
	if wm.Spec.Network.TCP.Connect[0] != "*:443" {
		t.Errorf("unexpected TCP connect: %v", wm.Spec.Network.TCP.Connect)
	}
}

// TestDeploy_WaitForReady verifies that Deploy() returns the URL when the
// WasmModule becomes Ready=True.
func TestDeploy_WaitForReady(t *testing.T) {
	t.Parallel()
	cs := newFakeClientset()
	d := wasm.NewDeployer(
		wasm.WithDeployerClientsetProvider(fakeProvider(cs)),
		wasm.WithDeployerWaitTimeout(5*time.Second),
	)

	f := minimalFunction("ready-func", "default", "registry.example.com/ready-func:latest")

	// In a goroutine, wait for the WasmModule to be created then set Ready=True
	// with a URL.
	go func() {
		wantURL := "http://ready-func.default.localtest.me"
		u, _ := url.Parse(wantURL)
		// Poll until the WasmModule exists.
		for {
			time.Sleep(10 * time.Millisecond)
			wm, err := cs.WasmV1alpha1().WasmModules("default").Get(context.Background(), "ready-func", metav1.GetOptions{})
			if err != nil {
				continue
			}
			// Set Ready=True with address.
			wm.Status.Address = &duckv1.Addressable{URL: (*knapis.URL)(u)}
			wm.Status.Conditions = duckv1.Conditions{
				{
					Type:   knapis.ConditionReady,
					Status: corev1.ConditionTrue,
				},
			}
			_, _ = cs.WasmV1alpha1().WasmModules("default").UpdateStatus(
				context.Background(), wm, metav1.UpdateOptions{})
			return
		}
	}()

	result, err := d.Deploy(context.Background(), f)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.URL == "" {
		t.Error("expected URL to be populated after ready")
	}
	if result.Status != fn.Deployed {
		t.Errorf("expected Deployed, got %v", result.Status)
	}
}

// TestDeploy_WaitForReady_Failure verifies that Deploy() returns an error when
// the WasmModule becomes Ready=False.
func TestDeploy_WaitForReady_Failure(t *testing.T) {
	t.Parallel()
	cs := newFakeClientset()
	d := wasm.NewDeployer(
		wasm.WithDeployerClientsetProvider(fakeProvider(cs)),
		wasm.WithDeployerWaitTimeout(5*time.Second),
	)

	f := minimalFunction("fail-func", "default", "registry.example.com/fail-func:latest")

	// In a goroutine, wait for the WasmModule to be created then set Ready=False
	// with a terminal (non-transient) reason. "ServiceUnavailable" is transient
	// and causes the deployer to keep polling; use "ContainerCrashed" instead.
	go func() {
		for {
			time.Sleep(10 * time.Millisecond)
			wm, err := cs.WasmV1alpha1().WasmModules("default").Get(context.Background(), "fail-func", metav1.GetOptions{})
			if err != nil {
				continue
			}
			// Set Ready=False with a terminal reason (not ServiceUnavailable).
			wm.Status.Conditions = duckv1.Conditions{
				{
					Type:    knapis.ConditionReady,
					Status:  corev1.ConditionFalse,
					Reason:  "ContainerCrashed",
					Message: `no exported instance named "wasi:http/incoming-handler@0.2.3"`,
				},
			}
			_, _ = cs.WasmV1alpha1().WasmModules("default").UpdateStatus(
				context.Background(), wm, metav1.UpdateOptions{})
			return
		}
	}()

	_, err := d.Deploy(context.Background(), f)
	if err == nil {
		t.Fatal("expected error when WasmModule is Ready=False, got nil")
	}
	if !containsString(err.Error(), "ContainerCrashed") {
		t.Errorf("expected error to mention ContainerCrashed, got: %v", err)
	}
}

// containsString is a helper for error message assertions.
func containsString(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr ||
		len(s) > 0 && len(substr) > 0 &&
			func() bool {
				for i := 0; i <= len(s)-len(substr); i++ {
					if s[i:i+len(substr)] == substr {
						return true
					}
				}
				return false
			}())
}

// TestLister_List verifies that WasmModules managed by this deployer are listed.
func TestLister_List(t *testing.T) {
	t.Parallel()
	managed := &wasmv1alpha1.WasmModule{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "wasm-func",
			Namespace: "default",
			Labels: map[string]string{
				"function.knative.dev/runtime": wasm.RuntimeRustWasi,
			},
			Annotations: map[string]string{
				deployer.DeployerNameAnnotation: wasm.DeployerName,
			},
		},
		Spec: wasmv1alpha1.WasmModuleSpec{Image: "reg/wasm-func:latest"},
	}
	other := &wasmv1alpha1.WasmModule{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "other-func",
			Namespace: "default",
			Annotations: map[string]string{
				deployer.DeployerNameAnnotation: "other-deployer",
			},
		},
		Spec: wasmv1alpha1.WasmModuleSpec{Image: "reg/other-func:latest"},
	}
	cs := newFakeClientset(managed, other)
	l := wasm.NewLister(wasm.WithListerClientsetProvider(fakeProvider(cs)))

	items, err := l.List(context.Background(), "default")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	if items[0].Name != "wasm-func" {
		t.Errorf("unexpected item name: %q", items[0].Name)
	}
	if items[0].Deployer != wasm.DeployerName {
		t.Errorf("unexpected deployer: %q", items[0].Deployer)
	}
}

// TestRemover_Remove verifies that a managed WasmModule is deleted.
func TestRemover_Remove(t *testing.T) {
	t.Parallel()
	existing := &wasmv1alpha1.WasmModule{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "wasm-func",
			Namespace: "default",
			Annotations: map[string]string{
				deployer.DeployerNameAnnotation: wasm.DeployerName,
			},
		},
		Spec: wasmv1alpha1.WasmModuleSpec{Image: "reg/wasm-func:latest"},
	}
	cs := newFakeClientset(existing)
	r := wasm.NewRemover(wasm.WithRemoverClientsetProvider(fakeProvider(cs)))

	if err := r.Remove(context.Background(), "wasm-func", "default"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify deleted.
	_, err := cs.WasmV1alpha1().WasmModules("default").Get(context.Background(), "wasm-func", metav1.GetOptions{})
	if err == nil {
		t.Error("expected WasmModule to be deleted, but it still exists")
	}
}

// TestRemover_Remove_ErrNotHandled verifies that a non-managed WasmModule is not removed.
func TestRemover_Remove_ErrNotHandled(t *testing.T) {
	t.Parallel()
	existing := &wasmv1alpha1.WasmModule{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "other-func",
			Namespace: "default",
			Annotations: map[string]string{
				deployer.DeployerNameAnnotation: "other-deployer",
			},
		},
		Spec: wasmv1alpha1.WasmModuleSpec{Image: "reg/other-func:latest"},
	}
	cs := newFakeClientset(existing)
	r := wasm.NewRemover(wasm.WithRemoverClientsetProvider(fakeProvider(cs)))

	err := r.Remove(context.Background(), "other-func", "default")
	if !errors.Is(err, fn.ErrNotHandled) {
		t.Errorf("expected ErrNotHandled, got: %v", err)
	}
}

// TestDescriber_Describe verifies that describing a managed WasmModule returns an instance.
func TestDescriber_Describe(t *testing.T) {
	t.Parallel()
	existing := &wasmv1alpha1.WasmModule{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "wasm-func",
			Namespace: "default",
			Labels: map[string]string{
				"function.knative.dev/runtime": wasm.RuntimeRustWasi,
			},
			Annotations: map[string]string{
				deployer.DeployerNameAnnotation: wasm.DeployerName,
			},
		},
		Spec: wasmv1alpha1.WasmModuleSpec{Image: "reg/wasm-func:latest"},
	}
	cs := newFakeClientset(existing)
	d := wasm.NewDescriber(wasm.WithDescriberClientsetProvider(fakeProvider(cs)))

	inst, err := d.Describe(context.Background(), "wasm-func", "default")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if inst.Name != "wasm-func" {
		t.Errorf("unexpected name: %q", inst.Name)
	}
	if inst.Deployer != wasm.DeployerName {
		t.Errorf("unexpected deployer: %q", inst.Deployer)
	}
}

// TestDescriber_Describe_ErrNotHandled verifies that a non-managed WasmModule returns ErrNotHandled.
func TestDescriber_Describe_ErrNotHandled(t *testing.T) {
	t.Parallel()
	existing := &wasmv1alpha1.WasmModule{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "other-func",
			Namespace: "default",
			Annotations: map[string]string{
				deployer.DeployerNameAnnotation: "other-deployer",
			},
		},
		Spec: wasmv1alpha1.WasmModuleSpec{Image: "reg/other-func:latest"},
	}
	cs := newFakeClientset(existing)
	d := wasm.NewDescriber(wasm.WithDescriberClientsetProvider(fakeProvider(cs)))

	_, err := d.Describe(context.Background(), "other-func", "default")
	if !errors.Is(err, fn.ErrNotHandled) {
		t.Errorf("expected ErrNotHandled, got: %v", err)
	}
}

// TestBuildNetworkSpec verifies the helper that converts fn.NetworkSpec → wasmv1alpha1.NetworkSpec.
func TestBuildNetworkSpec(t *testing.T) {
	t.Parallel()
	// nil input → nil output
	if out := wasm.BuildNetworkSpec(nil); out != nil {
		t.Errorf("expected nil, got %v", out)
	}

	// Full mapping.
	allowDNS := true
	in := &fn.NetworkSpec{
		Inherit:           true,
		AllowIpNameLookup: &allowDNS,
		Tcp: &fn.TcpNetworkSpec{
			Bind:    []string{"127.0.0.1:8080"},
			Connect: []string{"*:443"},
		},
		Udp: &fn.UdpNetworkSpec{
			Outgoing: []string{"8.8.8.8:53"},
		},
	}
	out := wasm.BuildNetworkSpec(in)
	if out == nil {
		t.Fatal("expected non-nil output")
	}
	if !out.Inherit {
		t.Error("Inherit not mapped")
	}
	if out.AllowIPNameLookup == nil || !*out.AllowIPNameLookup {
		t.Error("AllowIPNameLookup not mapped")
	}
	if out.TCP == nil || len(out.TCP.Bind) != 1 || out.TCP.Bind[0] != "127.0.0.1:8080" {
		t.Error("TCP bind not mapped correctly")
	}
	if out.TCP.Connect[0] != "*:443" {
		t.Error("TCP connect not mapped correctly")
	}
	if out.UDP == nil || len(out.UDP.Outgoing) != 1 || out.UDP.Outgoing[0] != "8.8.8.8:53" {
		t.Error("UDP outgoing not mapped correctly")
	}
}
