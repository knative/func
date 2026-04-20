package k8s

import (
	"encoding/base64"
	"fmt"

	fn "knative.dev/func/pkg/functions"
)

// ExtractClusterAuth reads the active kubeconfig context and returns the
// cluster URL, ClusterVerify and UserAuth suitable for storage in local.yaml.
// Certificate/key data from the kubeconfig ([]byte) is base64-encoded into the
// string fields expected by ClusterVerify / UserAuth.
func ExtractClusterAuth() (clusterURL string, clusterTLS fn.ClusterVerify, user fn.UserAuth, err error) {
	// Load and resolve the active kubeconfig context
	rawConfig, err := GetClientConfig().RawConfig()
	if err != nil {
		return "", fn.ClusterVerify{}, fn.UserAuth{}, fmt.Errorf("failed to load kubeconfig: %w", err)
	}

	ctxName := rawConfig.CurrentContext
	if ctxName == "" {
		return "", fn.ClusterVerify{}, fn.UserAuth{}, fmt.Errorf("no current context set in kubeconfig")
	}

	ctx, ok := rawConfig.Contexts[ctxName]
	if !ok {
		return "", fn.ClusterVerify{}, fn.UserAuth{}, fmt.Errorf("context %q not found in kubeconfig", ctxName)
	}

	// Look up cluster and user entries referenced by the context
	cluster, ok := rawConfig.Clusters[ctx.Cluster]
	if !ok {
		return "", fn.ClusterVerify{}, fn.UserAuth{}, fmt.Errorf("cluster %q (from context %q) not found in kubeconfig", ctx.Cluster, ctxName)
	}

	authInfo, ok := rawConfig.AuthInfos[ctx.AuthInfo]
	if !ok {
		return "", fn.ClusterVerify{}, fn.UserAuth{}, fmt.Errorf("user %q (from context %q) not found in kubeconfig", ctx.AuthInfo, ctxName)
	}

	// Extract cluster verification settings
	clusterURL = cluster.Server
	if len(cluster.CertificateAuthorityData) > 0 {
		clusterTLS.CertificateAuthorityData = base64.StdEncoding.EncodeToString(cluster.CertificateAuthorityData)
	}
	clusterTLS.InsecureSkipTLSVerify = cluster.InsecureSkipTLSVerify

	// Extract all auth credentials present — client-go handles precedence
	if authInfo.Token != "" {
		user.Token = authInfo.Token
	}
	if len(authInfo.ClientCertificateData) > 0 && len(authInfo.ClientKeyData) > 0 {
		user.ClientCertificateData = base64.StdEncoding.EncodeToString(authInfo.ClientCertificateData)
		user.ClientKeyData = base64.StdEncoding.EncodeToString(authInfo.ClientKeyData)
	}
	if authInfo.Exec != nil {
		execAuth := &fn.ExecAuth{
			Command:    authInfo.Exec.Command,
			Args:       authInfo.Exec.Args,
			APIVersion: authInfo.Exec.APIVersion,
		}
		for _, e := range authInfo.Exec.Env {
			execAuth.Env = append(execAuth.Env, fn.ExecEnv{
				Name:  e.Name,
				Value: e.Value,
			})
		}
		user.Exec = execAuth
	}

	return clusterURL, clusterTLS, user, nil
}
