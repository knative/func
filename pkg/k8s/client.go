package k8s

import (
	"errors"
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
//
// A Client is backed by exactly one of:
//   - a kubeconfig loader (NewClient, NewClientFromKubeconfig), which also
//     knows contexts and the active namespace;
//   - a bare rest.Config (NewClientFromConfig), for callers such as services
//     which hold a host and a token and have no kubeconfig at all.
type Client struct {
	// cc is the kubeconfig loader: merged files, contexts, active namespace.
	// Nil when the Client was built from a rest.Config.
	cc clientcmd.ClientConfig

	// cfg is the rest configuration given to NewClientFromConfig.
	// Nil when the Client was built from a kubeconfig loader.
	cfg *rest.Config

	// isOpenShift answers IsOpenShift; detection runs once per Client.
	isOpenShift func() (bool, error)
}

// errNoKubeconfig is returned by the methods which need a kubeconfig when the
// Client was built from a bare rest.Config.
var errNoKubeconfig = errors.New("client was built from a rest config; no kubeconfig available")

// errNoConfiguration is returned when a Client was built with neither a
// kubeconfig loader nor a rest config (NewClient(nil)).
var errNoConfiguration = errors.New("client has no kubeconfig loader and no rest config")

// errNilClient is returned by every method called on a nil *Client.
//
// Go runs a pointer-receiver method on a nil pointer; only a field access
// panics (https://go.dev/ref/spec#Selectors). RestConfig, RawConfig,
// DefaultNamespace and IsOpenShift are the only methods that read the
// struct's fields, and they check the receiver first. Everything else reaches
// the fields through them, so a nil client anywhere yields this error, not a
// panic.
var errNilClient = errors.New("kubernetes client is nil")

// NewClient returns a Client backed by the given kubeconfig loader.
func NewClient(cc clientcmd.ClientConfig) *Client {
	return newClient(&Client{cc: cc})
}

// NewClientFromKubeconfig returns a Client which resolves its configuration
// the way kubectl does: KUBECONFIG, ~/.kube/config, in-cluster.
func NewClientFromKubeconfig() *Client {
	return NewClient(kubeconfigClientConfig())
}

// NewClientFromConfig returns a Client backed by an already-resolved rest
// configuration, for callers which have a host and credentials but no
// kubeconfig. Methods which need a kubeconfig (RawConfig, DefaultNamespace)
// return an error on such a Client.
func NewClientFromConfig(cfg *rest.Config) *Client {
	return newClient(&Client{cfg: cfg})
}

func newClient(c *Client) *Client {
	c.isOpenShift = sync.OnceValues(func() (bool, error) { return detectOpenShift(c) })
	return c
}

// RestConfig returns the rest configuration (host, token, certs). Every call
// returns a value the caller may mutate: a copy for a config-backed Client,
// and for a kubeconfig-backed one the loader builds a new rest.Config on
// each call.
func (c *Client) RestConfig() (*rest.Config, error) {
	if c == nil {
		return nil, errNilClient
	}
	if c.cfg != nil {
		return rest.CopyConfig(c.cfg), nil
	}
	if c.cc == nil {
		return nil, errNoConfiguration
	}
	cfg, err := c.cc.ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to create kubernetes client config: %w", err)
	}
	return cfg, nil
}

// RawConfig returns the merged kubeconfig.
func (c *Client) RawConfig() (clientcmdapi.Config, error) {
	if c == nil {
		return clientcmdapi.Config{}, errNilClient
	}
	if c.cc == nil {
		return clientcmdapi.Config{}, errNoKubeconfig
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
	if c == nil {
		return "", errNilClient
	}
	if c.cc == nil {
		return "", errNoKubeconfig
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
	if c == nil {
		return false, errNilClient
	}
	return c.isOpenShift()
}

// detectOpenShift asks the API server for the route.openshift.io/v1 group,
// which works even under restrictive RBAC. NotFound means the cluster answered
// "not OpenShift"; any other error means it could not be asked.
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

// kubeconfigClientConfig resolves cluster configuration the way kubectl
// does: KUBECONFIG, ~/.kube/config, in-cluster.
func kubeconfigClientConfig() clientcmd.ClientConfig {
	return clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		clientcmd.NewDefaultClientConfigLoadingRules(),
		&clientcmd.ConfigOverrides{})
}
