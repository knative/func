package k8s

import (
	"encoding/base64"
	"fmt"
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

var restConfigOverride *rest.Config
var namespaceOverride string

// SetClusterOverride bypasses kubeconfig entirely by building a rest.Config
// from the given cluster URL and auth credentials. All subsequent
// GetClientConfig() calls will use this config until the returned cleanup
// function is called.
//
// The namespace parameter sets the default namespace returned by
// staticClientConfig.Namespace(). This is needed because the in-cluster
// dialer creates pods in the namespace returned by GetDefaultNamespace(),
// and without it the pod lands in "default" — which on OpenShift is
// restricted by SCC and rejects containers running as root.
func SetClusterOverride(clusterURL, namespace string, cluster fn.ClusterVerify, user fn.UserAuth) (func(), error) {
	cfg, err := BuildRestConfig(clusterURL, cluster, user)
	if err != nil {
		return nil, err
	}
	restConfigOverride = cfg
	namespaceOverride = namespace
	return ClearClusterOverride, nil
}

// ClearClusterOverride removes the direct cluster override, reverting to
// kubeconfig-based configuration.
func ClearClusterOverride() {
	restConfigOverride = nil
	namespaceOverride = ""
}

// BuildRestConfig creates a *rest.Config from a cluster URL, ClusterVerify, and
// UserAuth. Base64-encoded fields are decoded.
//
// All credentials present in UserAuth are applied to the rest.Config. The
// Kubernetes API server will authenticate whichever succeeds — for example a
// stale token may fail while a client certificate still works. If no
// credentials are provided the config is still valid; the API server will
// return 401/403.
func BuildRestConfig(clusterURL string, cluster fn.ClusterVerify, user fn.UserAuth) (*rest.Config, error) {
	cfg := &rest.Config{
		Host: clusterURL,
	}

	if cluster.CertificateAuthorityData != "" {
		raw, err := base64.StdEncoding.DecodeString(cluster.CertificateAuthorityData)
		if err != nil {
			return nil, fmt.Errorf("failed to decode certificate-authority-data: %w", err)
		}
		cfg.CAData = raw
	}
	cfg.Insecure = cluster.InsecureSkipTLSVerify

	if user.Token != "" {
		cfg.BearerToken = user.Token
	}

	if user.ClientCertificateData != "" {
		raw, err := base64.StdEncoding.DecodeString(user.ClientCertificateData)
		if err != nil {
			return nil, fmt.Errorf("failed to decode client-certificate-data: %w", err)
		}
		cfg.CertData = raw
	}
	if user.ClientKeyData != "" {
		raw, err := base64.StdEncoding.DecodeString(user.ClientKeyData)
		if err != nil {
			return nil, fmt.Errorf("failed to decode client-key-data: %w", err)
		}
		cfg.KeyData = raw
	}

	if user.Exec != nil {
		execCfg := &clientcmdapi.ExecConfig{
			Command:    user.Exec.Command,
			Args:       user.Exec.Args,
			APIVersion: user.Exec.APIVersion,
		}
		for _, e := range user.Exec.Env {
			execCfg.Env = append(execCfg.Env, clientcmdapi.ExecEnvVar{
				Name:  e.Name,
				Value: e.Value,
			})
		}
		cfg.ExecProvider = execCfg
	}

	return cfg, nil
}

// staticClientConfig wraps a *rest.Config to implement clientcmd.ClientConfig.
// This allows code that expects a ClientConfig (e.g. the dialer) to work with
// a directly-constructed rest.Config.
type staticClientConfig struct {
	cfg *rest.Config
}

func (s *staticClientConfig) ClientConfig() (*rest.Config, error) {
	out := *s.cfg
	return &out, nil
}

func (s *staticClientConfig) Namespace() (string, bool, error) {
	if namespaceOverride != "" {
		return namespaceOverride, true, nil
	}
	return "default", true, nil
}

func (s *staticClientConfig) RawConfig() (clientcmdapi.Config, error) {
	const name = "static"
	ns := namespaceOverride
	if ns == "" {
		ns = "default"
	}
	return clientcmdapi.Config{
		CurrentContext: name,
		Contexts: map[string]*clientcmdapi.Context{
			name: {Cluster: name, AuthInfo: name, Namespace: ns},
		},
		Clusters: map[string]*clientcmdapi.Cluster{
			name: {
				Server:                   s.cfg.Host,
				CertificateAuthorityData: s.cfg.CAData,
				InsecureSkipTLSVerify:    s.cfg.Insecure,
			},
		},
		AuthInfos: map[string]*clientcmdapi.AuthInfo{
			name: {
				Token:                 s.cfg.BearerToken,
				ClientCertificateData: s.cfg.CertData,
				ClientKeyData:         s.cfg.KeyData,
				Exec:                  s.cfg.ExecProvider,
			},
		},
	}, nil
}

func (s *staticClientConfig) ConfigAccess() clientcmd.ConfigAccess {
	return nil
}

func GetClientConfig() clientcmd.ClientConfig {
	if restConfigOverride != nil {
		return &staticClientConfig{cfg: restConfigOverride}
	}

	return clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		clientcmd.NewDefaultClientConfigLoadingRules(),
		&clientcmd.ConfigOverrides{})
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
