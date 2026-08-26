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

func TestNewClientFromConfig(t *testing.T) {
	cfg := &rest.Config{Host: "https://example.com:6443"}
	c := k8s.NewClientFromConfig(cfg)

	got, err := c.RestConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != cfg {
		t.Error("RestConfig() should return the same config that was passed in")
	}
}

func TestClientClientset(t *testing.T) {
	cfg := &rest.Config{Host: "https://example.com:6443"}
	c := k8s.NewClientFromConfig(cfg)

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

// TestClient_WithOpenShift verifies the preset answer wins and no detection
// runs, even though the fake cluster would answer the opposite.
func TestClient_WithOpenShift(t *testing.T) {
	FakeCluster(t, false)
	if ok, _ := k8s.NewClientFromKubeconfig(k8s.WithOpenShift(true)).IsOpenShift(); !ok {
		t.Error("expected preset true")
	}
	FakeCluster(t, true)
	if ok, _ := k8s.NewClientFromKubeconfig(k8s.WithOpenShift(false)).IsOpenShift(); ok {
		t.Error("expected preset false")
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
