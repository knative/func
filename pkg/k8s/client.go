package k8s

import (
	"fmt"
	"sync"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

const (
	DefaultWaitingTimeout     = 120 * time.Second
	DefaultErrorWindowTimeout = 2 * time.Second
)

// Client is the single entry point for cluster access. It is constructed
// once, at the top of the call chain (the CLI), and passed down to every
// component which talks to the cluster. Components must NOT construct their
// own cluster configuration.
type Client struct {
	cc clientcmd.ClientConfig

	cfg     *rest.Config
	cfgErr  error
	cfgOnce sync.Once

	// isOpenShift answers IsOpenShift: memoized detection, or a preset.
	isOpenShift func() (bool, error)
}

// ClientOpt configures a Client.
type ClientOpt func(*Client)

// WithOpenShift sets the OpenShift detection result up front, so the Client
// never contacts the cluster to find out. Intended for tests.
func WithOpenShift(v bool) ClientOpt {
	return func(c *Client) {
		c.isOpenShift = func() (bool, error) { return v, nil }
	}
}

func applyClientOpts(c *Client, opts []ClientOpt) *Client {
	if c.isOpenShift == nil {
		c.isOpenShift = sync.OnceValues(func() (bool, error) { return detectOpenShift(c) })
	}
	for _, o := range opts {
		o(c)
	}
	return c
}

// NewClient returns a Client backed by the given client configuration.
func NewClient(cc clientcmd.ClientConfig, opts ...ClientOpt) *Client {
	return applyClientOpts(&Client{cc: cc}, opts)
}

// NewClientFromKubeconfig returns a Client which resolves its configuration
// the way kubectl does: KUBECONFIG, ~/.kube/config, in-cluster.
func NewClientFromKubeconfig(opts ...ClientOpt) *Client {
	return NewClient(GetClientConfig(), opts...)
}

// NewClientFromConfig returns a Client backed by an already-resolved rest
// config. Used in tests that do not want to go through kubeconfig.
func NewClientFromConfig(cfg *rest.Config, opts ...ClientOpt) *Client {
	return applyClientOpts(&Client{cfg: cfg}, opts)
}

// Loader returns the deferred kubeconfig loader, if this Client was built
// from kubeconfig. Nil when built from NewClientFromConfig.
func (c *Client) Loader() clientcmd.ClientConfig {
	return c.cc
}

// RestConfig returns the resolved rest configuration (host, token, certs).
func (c *Client) RestConfig() (*rest.Config, error) {
	c.cfgOnce.Do(func() {
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

// RawConfig returns the merged kubeconfig.
func (c *Client) RawConfig() (clientcmdapi.Config, error) {
	if c.cc == nil {
		return clientcmdapi.Config{}, fmt.Errorf("no kubernetes client configuration available")
	}
	return c.cc.RawConfig()
}

// Clientset returns a typed clientset.
func (c *Client) Clientset() (*kubernetes.Clientset, error) {
	cfg, err := c.RestConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to create new kubernetes client: %w", err)
	}
	return kubernetes.NewForConfig(cfg)
}

// DynamicClient returns a dynamic client.
func (c *Client) DynamicClient() (dynamic.Interface, error) {
	cfg, err := c.RestConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to create new kubernetes client: %w", err)
	}
	return dynamic.NewForConfig(cfg)
}

// DefaultNamespace returns the namespace of the active context.
func (c *Client) DefaultNamespace() (string, error) {
	if c.cc == nil {
		return "", fmt.Errorf("no kubernetes client configuration available")
	}
	ns, _, err := c.cc.Namespace()
	return ns, err
}

// ClientAndNamespace returns a clientset and the namespace to use: ns if
// given, else the namespace of the active context.
func (c *Client) ClientAndNamespace(ns string) (*kubernetes.Clientset, string, error) {
	var err error
	if ns == "" {
		if ns, err = c.DefaultNamespace(); err != nil {
			return nil, ns, err
		}
	}
	cs, err := c.Clientset()
	return cs, ns, err
}

// IsOpenShift reports whether the cluster serves the OpenShift Route API.
// A non-nil error means the cluster could not be asked and the bool is
// meaningless. Detection runs at most once per Client, on first use.
func (c *Client) IsOpenShift() (bool, error) {
	return c.isOpenShift()
}

// detectOpenShift asks the API server for the route.openshift.io/v1 group,
// which works even under restrictive RBAC. Any error, including an
// unreachable cluster, counts as "not OpenShift".
func detectOpenShift(c *Client) (bool, error) {
	cs, err := c.Clientset()
	if err != nil {
		return false, err
	}
	_, err = cs.Discovery().ServerResourcesForGroupVersion("route.openshift.io/v1")
	switch {
	case err == nil:
		return true, nil
	case apierrors.IsNotFound(err):
		// The cluster answered: it does not serve this API.
		return false, nil
	default:
		return false, err
	}
}

// The functions below read the kubeconfig ad hoc. They remain only for
// callers not yet migrated to Client and will be removed once none are left.
// Do not add new callers.

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
