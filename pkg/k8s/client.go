package k8s

import (
	"fmt"
	"time"

	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

const (
	DefaultWaitingTimeout     = 120 * time.Second
	DefaultErrorWindowTimeout = 2 * time.Second
)

type Client struct {
	cc  clientcmd.ClientConfig
	cfg *rest.Config
}

func NewClient(cc clientcmd.ClientConfig) *Client {
	return &Client{cc: cc}
}

func NewClientFromConfig(cfg *rest.Config) *Client {
	return &Client{cfg: cfg}
}

func (c *Client) ClientConfig() (*rest.Config, error) {
	if c.cfg != nil {
		return c.cfg, nil
	}
	if c.cc == nil {
		return nil, fmt.Errorf("no kubernetes client configuration available")
	}
	cfg, err := c.cc.ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to create kubernetes client config: %w", err)
	}
	c.cfg = cfg
	return c.cfg, nil
}

func (c *Client) Clientset() (*kubernetes.Clientset, error) {
	cfg, err := c.ClientConfig()
	if err != nil {
		return nil, err
	}
	return kubernetes.NewForConfig(cfg)
}

func NewClientAndResolvedNamespace(ns string) (*kubernetes.Clientset, string, error) {
	var err error
	if ns == "" {
		ns, err = GetDefaultNamespace()
		if err != nil {
			return nil, ns, err
		}
	}

	client, err := NewKubernetesClientset()
	return client, ns, err
}

func NewKubernetesClientset() (*kubernetes.Clientset, error) {
	restConfig, err := GetClientConfig().ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to create new kubernetes client: %w", err)
	}

	return kubernetes.NewForConfig(restConfig)
}

func NewDynamicClient() (dynamic.Interface, error) {
	restConfig, err := GetClientConfig().ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to create new kubernetes client: %w", err)
	}

	return dynamic.NewForConfig(restConfig)
}

// GetDefaultNamespace returns default namespace
func GetDefaultNamespace() (namespace string, err error) {
	namespace, _, err = GetClientConfig().Namespace()
	return
}

func GetClientConfig() clientcmd.ClientConfig {
	return clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		clientcmd.NewDefaultClientConfigLoadingRules(),
		&clientcmd.ConfigOverrides{})
}
