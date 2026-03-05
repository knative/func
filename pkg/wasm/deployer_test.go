package wasm

import (
	"context"
	"errors"
	"testing"

	wasmv1alpha1 "github.com/cardil/knative-serving-wasm/pkg/apis/wasm/v1alpha1"
	wasmclientset "github.com/cardil/knative-serving-wasm/pkg/client/clientset/versioned"
	fakewasm "github.com/cardil/knative-serving-wasm/pkg/client/clientset/versioned/fake"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	"knative.dev/func/pkg/deployer"
	fn "knative.dev/func/pkg/functions"
)

// fakeProvider returns a ClientsetProvider backed by the given fake clientset.
func fakeProvider(cs wasmclientset.Interface) ClientsetProvider {
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
		Runtime: RuntimeRustWasi,
		Deploy: fn.DeploySpec{
			Namespace: namespace,
			Image:     image,
		},
	}
}

// TestDeploy_Create verifies that deploying a new function creates a WasmModule.
func TestDeploy_Create(t *testing.T) {
	cs := newFakeClientset()
	d := NewDeployer(WithDeployerClientsetProvider(fakeProvider(cs)))

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
	if wm.Annotations[deployer.DeployerNameAnnotation] != DeployerName {
		t.Errorf("deployer annotation not set; got %q", wm.Annotations[deployer.DeployerNameAnnotation])
	}
}

// TestDeploy_Update verifies that deploying an existing function updates the WasmModule.
func TestDeploy_Update(t *testing.T) {
	existing := &wasmv1alpha1.WasmModule{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-func",
			Namespace: "default",
			Annotations: map[string]string{
				deployer.DeployerNameAnnotation: DeployerName,
			},
		},
		Spec: wasmv1alpha1.WasmModuleSpec{
			Image: "registry.example.com/my-func:v1",
		},
	}
	cs := newFakeClientset(existing)
	d := NewDeployer(WithDeployerClientsetProvider(fakeProvider(cs)))

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
	cs := newFakeClientset()
	d := NewDeployer(WithDeployerClientsetProvider(fakeProvider(cs)))

	f := fn.Function{Name: "my-func", Runtime: RuntimeRustWasi}
	f.Deploy.Image = "registry.example.com/my-func:latest"

	_, err := d.Deploy(context.Background(), f)
	if !errors.Is(err, fn.ErrNamespaceRequired) {
		t.Errorf("expected ErrNamespaceRequired, got: %v", err)
	}
}

// TestDeploy_ErrNoImageRef verifies that a missing image returns the sentinel error.
func TestDeploy_ErrNoImageRef(t *testing.T) {
	cs := newFakeClientset()
	d := NewDeployer(WithDeployerClientsetProvider(fakeProvider(cs)))

	f := fn.Function{Name: "my-func", Runtime: RuntimeRustWasi}
	f.Deploy.Namespace = "default"
	// No image set.

	_, err := d.Deploy(context.Background(), f)
	if !errors.Is(err, ErrNoImageRef) {
		t.Errorf("expected ErrNoImageRef, got: %v", err)
	}
}

// TestDeploy_Network verifies that deploy.network is mapped to WasmModule spec.
func TestDeploy_Network(t *testing.T) {
	cs := newFakeClientset()
	d := NewDeployer(WithDeployerClientsetProvider(fakeProvider(cs)))

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
	if wm.Spec.Network.AllowIpNameLookup == nil || !*wm.Spec.Network.AllowIpNameLookup {
		t.Error("expected AllowIpNameLookup to be true")
	}
	if wm.Spec.Network.Tcp == nil || len(wm.Spec.Network.Tcp.Connect) != 1 {
		t.Error("expected TCP connect to be set")
	}
	if wm.Spec.Network.Tcp.Connect[0] != "*:443" {
		t.Errorf("unexpected TCP connect: %v", wm.Spec.Network.Tcp.Connect)
	}
}

// TestLister_List verifies that WasmModules managed by this deployer are listed.
func TestLister_List(t *testing.T) {
	managed := &wasmv1alpha1.WasmModule{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "wasm-func",
			Namespace: "default",
			Labels: map[string]string{
				"function.knative.dev/runtime": RuntimeRustWasi,
			},
			Annotations: map[string]string{
				deployer.DeployerNameAnnotation: DeployerName,
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
	l := NewLister(WithListerClientsetProvider(fakeProvider(cs)))

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
	if items[0].Deployer != DeployerName {
		t.Errorf("unexpected deployer: %q", items[0].Deployer)
	}
}

// TestRemover_Remove verifies that a managed WasmModule is deleted.
func TestRemover_Remove(t *testing.T) {
	existing := &wasmv1alpha1.WasmModule{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "wasm-func",
			Namespace: "default",
			Annotations: map[string]string{
				deployer.DeployerNameAnnotation: DeployerName,
			},
		},
		Spec: wasmv1alpha1.WasmModuleSpec{Image: "reg/wasm-func:latest"},
	}
	cs := newFakeClientset(existing)
	r := NewRemover(WithRemoverClientsetProvider(fakeProvider(cs)))

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
	r := NewRemover(WithRemoverClientsetProvider(fakeProvider(cs)))

	err := r.Remove(context.Background(), "other-func", "default")
	if !errors.Is(err, fn.ErrNotHandled) {
		t.Errorf("expected ErrNotHandled, got: %v", err)
	}
}

// TestDescriber_Describe verifies that describing a managed WasmModule returns an instance.
func TestDescriber_Describe(t *testing.T) {
	existing := &wasmv1alpha1.WasmModule{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "wasm-func",
			Namespace: "default",
			Labels: map[string]string{
				"function.knative.dev/runtime": RuntimeRustWasi,
			},
			Annotations: map[string]string{
				deployer.DeployerNameAnnotation: DeployerName,
			},
		},
		Spec: wasmv1alpha1.WasmModuleSpec{Image: "reg/wasm-func:latest"},
	}
	cs := newFakeClientset(existing)
	d := NewDescriber(WithDescriberClientsetProvider(fakeProvider(cs)))

	inst, err := d.Describe(context.Background(), "wasm-func", "default")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if inst.Name != "wasm-func" {
		t.Errorf("unexpected name: %q", inst.Name)
	}
	if inst.Deployer != DeployerName {
		t.Errorf("unexpected deployer: %q", inst.Deployer)
	}
}

// TestDescriber_Describe_ErrNotHandled verifies that a non-managed WasmModule returns ErrNotHandled.
func TestDescriber_Describe_ErrNotHandled(t *testing.T) {
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
	d := NewDescriber(WithDescriberClientsetProvider(fakeProvider(cs)))

	_, err := d.Describe(context.Background(), "other-func", "default")
	if !errors.Is(err, fn.ErrNotHandled) {
		t.Errorf("expected ErrNotHandled, got: %v", err)
	}
}

// TestBuildNetworkSpec verifies the helper that converts fn.NetworkSpec → wasmv1alpha1.NetworkSpec.
func TestBuildNetworkSpec(t *testing.T) {
	// nil input → nil output
	if out := buildNetworkSpec(nil); out != nil {
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
	out := buildNetworkSpec(in)
	if out == nil {
		t.Fatal("expected non-nil output")
	}
	if !out.Inherit {
		t.Error("Inherit not mapped")
	}
	if out.AllowIpNameLookup == nil || !*out.AllowIpNameLookup {
		t.Error("AllowIpNameLookup not mapped")
	}
	if out.Tcp == nil || len(out.Tcp.Bind) != 1 || out.Tcp.Bind[0] != "127.0.0.1:8080" {
		t.Error("TCP bind not mapped correctly")
	}
	if out.Tcp.Connect[0] != "*:443" {
		t.Error("TCP connect not mapped correctly")
	}
	if out.Udp == nil || len(out.Udp.Outgoing) != 1 || out.Udp.Outgoing[0] != "8.8.8.8:53" {
		t.Error("UDP outgoing not mapped correctly")
	}
}
