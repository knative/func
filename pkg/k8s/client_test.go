package k8s

import (
	"fmt"
	"testing"

	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

func TestNewClientFromConfig(t *testing.T) {
	cfg := &rest.Config{Host: "https://example.com:6443"}
	c := NewClientFromConfig(cfg)

	got, err := c.ClientConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != cfg {
		t.Error("ClientConfig() should return the same config that was passed in")
	}
}

func TestClientClientset(t *testing.T) {
	cfg := &rest.Config{Host: "https://example.com:6443"}
	c := NewClientFromConfig(cfg)

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
	c := NewClient(cc)

	_, err := c.ClientConfig()
	if err == nil {
		t.Fatal("expected error when ClientConfig fails")
	}
}

func TestNewClientWithValidConfig(t *testing.T) {
	cfg := &rest.Config{Host: "https://example.com:6443"}
	cc := &fakeClientConfig{cfg: cfg}
	c := NewClient(cc)

	got, err := c.ClientConfig()
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
