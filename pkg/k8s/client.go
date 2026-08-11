package k8s

import (
	"fmt"
	"sync"
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
	cc     clientcmd.ClientConfig
	cfg    *rest.Config
	cfgErr error
	o      sync.Once
}

func NewClient(cc clientcmd.ClientConfig) *Client {
	return &Client{cc: cc}
}

func NewClientFromConfig(cfg *rest.Config) *Client {
	return &Client{cfg: cfg}
}

func (c *Client) ClientConfig() (*rest.Config, error) {
	c.o.Do(func() {
		if c.cfg != nil {
			return
		}
		if c.cc == nil {
			c.cfgErr = fmt.Errorf("no kubernetes client configuration available")
			return
		}
		c.cfg, c.cfgErr = c.cc.ClientConfig()
		if c.cfgErr != nil {
			c.cfgErr = fmt.Errorf("failed to create kubernetes client config: %w", c.cfgErr)
		}
	})
	return c.cfg, c.cfgErr
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
