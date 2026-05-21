package k8s

import (
	"encoding/base64"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"

	fn "knative.dev/func/pkg/functions"
)

const (
	DefaultWaitingTimeout     = 120 * time.Second
	DefaultErrorWindowTimeout = 2 * time.Second
)

// Client wraps a clientcmd.ClientConfig and provides convenience methods
// for creating Kubernetes clients. All cluster access in the codebase should
// go through a Client instance rather than calling package-level functions.
type Client struct {
	cc            clientcmd.ClientConfig
	openshift     bool
	openShiftOnce sync.Once
}

// NewClient creates a Client from the given ClientConfig.
func NewClient(cc clientcmd.ClientConfig) *Client {
	return &Client{cc: cc}
}

// NewClientWithOpenShift creates a Client with a pre-set OpenShift detection
// result. For testing only.
func NewClientWithOpenShift(cc clientcmd.ClientConfig, isOpenShift bool) *Client {
	c := &Client{cc: cc, openshift: isOpenShift}
	c.openShiftOnce.Do(func() {}) // prevent real detection
	return c
}

// ClientConfig returns the underlying rest.Config.
func (c *Client) ClientConfig() (*rest.Config, error) {
	return c.cc.ClientConfig()
}

// Clientset creates a new kubernetes.Clientset.
func (c *Client) Clientset() (*kubernetes.Clientset, error) {
	cfg, err := c.cc.ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to create new kubernetes client: %w", err)
	}
	return kubernetes.NewForConfig(cfg)
}

// ClientAndNamespace creates a Clientset and resolves the namespace,
// falling back to the default namespace from the config if ns is empty.
func (c *Client) ClientAndNamespace(ns string) (*kubernetes.Clientset, string, error) {
	var err error
	if ns == "" {
		ns, err = c.DefaultNamespace()
		if err != nil {
			return nil, ns, err
		}
	}
	client, err := c.Clientset()
	return client, ns, err
}

// DynamicClient creates a new dynamic.Interface.
func (c *Client) DynamicClient() (dynamic.Interface, error) {
	cfg, err := c.cc.ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to create new kubernetes client: %w", err)
	}
	return dynamic.NewForConfig(cfg)
}

// DefaultNamespace returns the default namespace from the config.
func (c *Client) DefaultNamespace() (string, error) {
	ns, _, err := c.cc.Namespace()
	return ns, err
}

// RawConfig returns the raw kubeconfig API config.
func (c *Client) RawConfig() (clientcmdapi.Config, error) {
	return c.cc.RawConfig()
}

// Auth exports the already-resolved rest.Config as fn types suitable for
// storage in local.yaml. After a successful deploy the client already holds
// the merged result of kubeconfig + overrides, so there is no need to
// re-parse the raw kubeconfig.
func (c *Client) Auth() (clusterURL string, cluster fn.ClusterTLS, user fn.UserAuth, err error) {
	cfg, err := c.cc.ClientConfig()
	if err != nil {
		return
	}

	clusterURL = cfg.Host

	if len(cfg.CAData) > 0 {
		cluster.CertificateAuthorityData = base64.StdEncoding.EncodeToString(cfg.CAData)
	}
	cluster.CertificateAuthority = cfg.CAFile
	cluster.InsecureSkipTLSVerify = cfg.Insecure

	user.Token = cfg.BearerToken
	if len(cfg.CertData) > 0 {
		user.ClientCertificateData = base64.StdEncoding.EncodeToString(cfg.CertData)
	}
	if len(cfg.KeyData) > 0 {
		user.ClientKeyData = base64.StdEncoding.EncodeToString(cfg.KeyData)
	}
	user.ClientCertificate = cfg.CertFile
	user.ClientKey = cfg.KeyFile

	return
}

// BuildClientConfig creates the k8s client config with possible overrides to
// the cluster url or its auth fields. Base64-encoded string fields from
// local.yaml are decoded to []byte for the clientcmd API.
//
// When a cluster URL is provided but no stored auth exists for it, the
// kubeconfig is searched for a context whose cluster matches the URL.
// If the active context matches, its auth is used. Otherwise exactly one
// matching context is required; zero or multiple matches produce an error.
func BuildClientConfig(url, token, namespace string, local fn.Local) (clientcmd.ClientConfig, error) {
	// For proper identification we need 3 concrete items in order to not rely
	// on the kubeconfig active context

	co := clientcmd.ConfigOverrides{}

	// if cluster url is detected
	if url != "" {
		// 1) authentication, certificates, keys (or token further below)
		if entry := local.FindAuth(url); entry != nil {
			co = authEntryOverrides(entry) // add overrides from local.yaml
		} else if token == "" {
			resolved, err := resolveKubeconfigAuth(url)
			if err != nil {
				return nil, err
			}
			co = resolved // use kubeconfig context (active or exactly 1 match)
		}
		// 2) cluster url
		co.ClusterInfo.Server = url
	}
	if token != "" {
		co.AuthInfo.Token = token
	}

	// 3) namespace within that cluster
	if namespace != "" {
		co.Context.Namespace = namespace
	}
	return clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		clientcmd.NewDefaultClientConfigLoadingRules(),
		&co), nil
}

// authEntryOverrides builds ConfigOverrides from stored auth in local.yaml.
func authEntryOverrides(entry *fn.AuthEntry) clientcmd.ConfigOverrides {
	co := clientcmd.ConfigOverrides{}
	co.ClusterInfo.CertificateAuthorityData = decodeBase64(entry.Cluster.CertificateAuthorityData)
	co.ClusterInfo.CertificateAuthority = entry.Cluster.CertificateAuthority
	co.ClusterInfo.InsecureSkipTLSVerify = entry.Cluster.InsecureSkipTLSVerify

	co.AuthInfo.ClientCertificateData = decodeBase64(entry.User.ClientCertificateData)
	co.AuthInfo.ClientCertificate = entry.User.ClientCertificate
	co.AuthInfo.ClientKeyData = decodeBase64(entry.User.ClientKeyData)
	co.AuthInfo.ClientKey = entry.User.ClientKey
	co.AuthInfo.Token = entry.User.Token
	return co
}

// resolveKubeconfigAuth searches the kubeconfig for a context whose cluster
// URL matches the target and returns overrides with that context's auth.
//
// Priority:
//  1. Active context's cluster URL matches → use its auth.
//  2. Exactly one other context matches → use its auth.
//  3. Zero or multiple matches → error.
func resolveKubeconfigAuth(targetURL string) (clientcmd.ConfigOverrides, error) {
	loader := clientcmd.NewDefaultClientConfigLoadingRules()
	raw, err := loader.Load()
	if err != nil {
		return clientcmd.ConfigOverrides{}, fmt.Errorf("failed to load kubeconfig: %w", err)
	}

	target := strings.TrimRight(targetURL, "/")

	// Check active context first: if it already targets the cluster, prefer it
	// (even if other contexts also match).
	if ctxName := raw.CurrentContext; ctxName != "" {
		if ctx, ok := raw.Contexts[ctxName]; ok {
			if cluster, ok := raw.Clusters[ctx.Cluster]; ok && strings.TrimRight(cluster.Server, "/") == target {
				fmt.Fprintf(os.Stderr, "Using active kubeconfig context %q for cluster %s\n", ctxName, targetURL)
				return kubeconfigAuthOverrides(raw, ctx), nil
			}
		}
	}

	// Otherwise require exactly one other context whose cluster URL matches.
	var matchNames []string
	for name, ctx := range raw.Contexts {
		if cluster, ok := raw.Clusters[ctx.Cluster]; ok && strings.TrimRight(cluster.Server, "/") == target {
			matchNames = append(matchNames, name)
		}
	}

	switch len(matchNames) {
	case 0:
		return clientcmd.ConfigOverrides{}, fmt.Errorf("no kubeconfig context found for cluster URL %q; pass --cluster-token or ensure a kubeconfig context targets this cluster", targetURL)
	case 1:
		fmt.Fprintf(os.Stderr, "Using kubeconfig context %q for cluster %s\n", matchNames[0], targetURL)
		return kubeconfigAuthOverrides(raw, raw.Contexts[matchNames[0]]), nil
	default:
		return clientcmd.ConfigOverrides{}, fmt.Errorf("multiple kubeconfig contexts (%s) match cluster URL %q; disambiguate with --cluster-token, switch to correct context or a stored credential", strings.Join(matchNames, ", "), targetURL)
	}
}

// kubeconfigAuthOverrides builds ConfigOverrides from a kubeconfig context.
func kubeconfigAuthOverrides(raw *clientcmdapi.Config, ctx *clientcmdapi.Context) clientcmd.ConfigOverrides {
	co := clientcmd.ConfigOverrides{}
	co.Context.Namespace = ctx.Namespace
	if cluster, ok := raw.Clusters[ctx.Cluster]; ok {
		co.ClusterInfo.CertificateAuthorityData = cluster.CertificateAuthorityData
		co.ClusterInfo.CertificateAuthority = cluster.CertificateAuthority
		co.ClusterInfo.InsecureSkipTLSVerify = cluster.InsecureSkipTLSVerify
	}
	if authInfo, ok := raw.AuthInfos[ctx.AuthInfo]; ok {
		co.AuthInfo.ClientCertificateData = authInfo.ClientCertificateData
		co.AuthInfo.ClientCertificate = authInfo.ClientCertificate
		co.AuthInfo.ClientKeyData = authInfo.ClientKeyData
		co.AuthInfo.ClientKey = authInfo.ClientKey
		co.AuthInfo.Token = authInfo.Token
		co.AuthInfo.Exec = authInfo.Exec
	}
	return co
}

func decodeBase64(s string) []byte {
	if s == "" {
		return nil
	}
	b, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		// just warn here; if user edits local.yaml file manually they get a
		// warning here. Adding an error would mean propagating into every call
		// site of the client initialization.
		fmt.Fprintf(os.Stderr, "Warning: failed to decode base64 credential data from local.yaml: %v\n", err)
		return nil
	}
	return b
}
