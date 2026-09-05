package k8s_test

import (
	"fmt"
	"testing"

	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"

	"knative.dev/func/pkg/k8s"
	. "knative.dev/func/pkg/testing"
)

// inMemoryLoader returns a kubeconfig loader for a single-context config
// pointing at host, without touching the filesystem.
func inMemoryLoader(host string) clientcmd.ClientConfig {
	return clientcmd.NewDefaultClientConfig(clientcmdapi.Config{
		CurrentContext: "test",
		Contexts:       map[string]*clientcmdapi.Context{"test": {Cluster: "test", AuthInfo: "test"}},
		Clusters:       map[string]*clientcmdapi.Cluster{"test": {Server: host}},
		AuthInfos:      map[string]*clientcmdapi.AuthInfo{"test": {Token: "t"}},
	}, nil)
}

// TestClient_RestConfig verifies the rest config is resolved from the loader
// and that each call yields its own copy, safe to mutate.
func TestClient_RestConfig(t *testing.T) {
	c := k8s.NewClient(inMemoryLoader("https://example.com:6443"))

	got, err := c.RestConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Host != "https://example.com:6443" {
		t.Errorf("unexpected host %q", got.Host)
	}
	got.Host = "mutated"
	if again, _ := c.RestConfig(); again.Host != "https://example.com:6443" {
		t.Error("RestConfig() should return a fresh config on every call")
	}
}

// TestClient_FromConfig verifies a Client built from a bare rest.Config:
// RestConfig returns a copy of it, and the kubeconfig-only methods report
// that no kubeconfig is available rather than panicking.
func TestClient_FromConfig(t *testing.T) {
	cfg := &rest.Config{Host: "https://example.com:6443", BearerToken: "t"}
	c := k8s.NewClientFromConfig(cfg)

	got, err := c.RestConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Host != cfg.Host || got.BearerToken != cfg.BearerToken {
		t.Errorf("unexpected config %+v", got)
	}
	got.Host = "mutated"
	if cfg.Host != "https://example.com:6443" {
		t.Error("RestConfig() must return a copy, not the caller's config")
	}

	if _, err := c.Clientset(); err != nil {
		t.Errorf("unexpected error creating clientset: %v", err)
	}
	if _, err := c.RawConfig(); err == nil {
		t.Error("RawConfig() should fail without a kubeconfig")
	}
	if _, err := c.DefaultNamespace(); err == nil {
		t.Error("DefaultNamespace() should fail without a kubeconfig")
	}
}

func TestClientClientset(t *testing.T) {
	c := k8s.NewClient(inMemoryLoader("https://example.com:6443"))

	cs, err := c.Clientset()
	if err != nil {
		t.Fatalf("unexpected error creating clientset: %v", err)
	}
	if cs == nil {
		t.Fatal("expected non-nil clientset")
	}
}

func TestNewClientWithInvalidConfig(t *testing.T) {
	cc := &fakeClientConfig{err: fmt.Errorf("no kubeconfig")}
	c := k8s.NewClient(cc)

	_, err := c.RestConfig()
	if err == nil {
		t.Fatal("expected error when RestConfig fails")
	}
}

func TestNewClientWithValidConfig(t *testing.T) {
	cfg := &rest.Config{Host: "https://example.com:6443"}
	cc := &fakeClientConfig{cfg: cfg}
	c := k8s.NewClient(cc)

	got, err := c.RestConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Host != "https://example.com:6443" {
		t.Errorf("expected host https://example.com:6443, got %s", got.Host)
	}
}

type fakeClientConfig struct {
	cfg *rest.Config
	err error
}

func (f *fakeClientConfig) RawConfig() (clientcmdapi.Config, error) {
	return clientcmdapi.Config{}, nil
}

func (f *fakeClientConfig) ClientConfig() (*rest.Config, error) {
	return f.cfg, f.err
}

func (f *fakeClientConfig) Namespace() (string, bool, error) {
	return "default", false, nil
}

func (f *fakeClientConfig) ConfigAccess() clientcmd.ConfigAccess {
	return nil
}

// TestClient_IsOpenShift verifies detection against a fake API server: true
// when the route.openshift.io/v1 group exists, false otherwise.
func TestClient_IsOpenShift(t *testing.T) {
	for _, openshift := range []bool{true, false} {
		FakeCluster(t, openshift)
		if got, err := k8s.NewClientFromKubeconfig().IsOpenShift(); got != openshift || err != nil {
			t.Errorf("fake cluster openshift=%v, IsOpenShift() = %v, %v", openshift, got, err)
		}
	}
}

// TestClient_IsOpenShiftUnreachable verifies an unreachable cluster reports
// an error rather than claiming "not OpenShift".
func TestClient_IsOpenShiftUnreachable(t *testing.T) {
	UnreachableCluster(t)
	if ok, err := k8s.NewClientFromKubeconfig().IsOpenShift(); err == nil || ok {
		t.Errorf("expected an error for an unreachable cluster, got %v, %v", ok, err)
	}
}

// TestClient_Namespace verifies namespace resolution from the active context.
func TestClient_Namespace(t *testing.T) {
	FakeCluster(t, false)
	c := k8s.NewClientFromKubeconfig()

	ns, err := c.DefaultNamespace()
	if err != nil {
		t.Fatal(err)
	}
	if ns != "default" {
		t.Errorf("expected namespace 'default', got %q", ns)
	}

	if _, ns, err = c.ClientAndNamespace(""); err != nil || ns != "default" {
		t.Errorf("expected 'default', got %q (err %v)", ns, err)
	}
	if _, ns, err = c.ClientAndNamespace("other"); err != nil || ns != "other" {
		t.Errorf("expected 'other', got %q (err %v)", ns, err)
	}
}

// TestClient_DefaultOpenShiftRegistry verifies the registry path uses the
// active namespace.
func TestClient_DefaultOpenShiftRegistry(t *testing.T) {
	FakeCluster(t, true)
	got := k8s.NewClientFromKubeconfig().DefaultOpenShiftRegistry()
	want := "image-registry.openshift-image-registry.svc:5000/default"
	if got != want {
		t.Errorf("expected %q, got %q", want, got)
	}
}

// TestClient_OpenShiftDockerCredentialLoaders verifies the loader serves the
// active user's token for the internal registry only.
func TestClient_OpenShiftDockerCredentialLoaders(t *testing.T) {
	FakeCluster(t, true)
	loaders := k8s.NewClientFromKubeconfig().OpenShiftDockerCredentialLoaders()
	if len(loaders) != 1 {
		t.Fatalf("expected one loader, got %d", len(loaders))
	}
	creds, err := loaders[0]("image-registry.openshift-image-registry.svc:5000")
	if err != nil {
		t.Fatal(err)
	}
	if creds.Username != "openshift" || creds.Password != "fake-token" {
		t.Errorf("unexpected credentials %+v", creds)
	}
	if _, err = loaders[0]("docker.io"); err == nil {
		t.Error("expected error for a foreign registry")
	}
}

// TestNilClient_MethodsReturnErrors ensures every method of a nil *Client
// returns an error instead of panicking (see errNilClient).
func TestNilClient_MethodsReturnErrors(t *testing.T) {
	var c *k8s.Client

	if _, err := c.RestConfig(); err == nil {
		t.Error("RestConfig() on a nil client should fail")
	}
	if _, err := c.RawConfig(); err == nil {
		t.Error("RawConfig() on a nil client should fail")
	}
	if _, err := c.DefaultNamespace(); err == nil {
		t.Error("DefaultNamespace() on a nil client should fail")
	}
	if _, err := c.IsOpenShift(); err == nil {
		t.Error("IsOpenShift() on a nil client should fail")
	}
	// Derived methods reach the fields only through the four above.
	if _, err := c.Clientset(); err == nil {
		t.Error("Clientset() on a nil client should fail")
	}
	if _, err := c.DynamicClient(); err == nil {
		t.Error("DynamicClient() on a nil client should fail")
	}
	if _, _, err := c.ClientAndNamespace(""); err == nil {
		t.Error("ClientAndNamespace() on a nil client should fail")
	}
	if _, err := c.OpenShiftServiceCA(t.Context()); err == nil {
		t.Error("OpenShiftServiceCA() on a nil client should fail")
	}
	if loaders := c.OpenShiftDockerCredentialLoaders(); loaders != nil {
		t.Error("OpenShiftDockerCredentialLoaders() on a nil client should yield no loaders")
	}

	// A Client with neither a kubeconfig loader nor a rest config fails the
	// same way, one level up.
	if _, err := k8s.NewClient(nil).RestConfig(); err == nil {
		t.Error("RestConfig() on a Client without configuration should fail")
	}
}
