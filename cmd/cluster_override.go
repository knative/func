package cmd

import (
	"fmt"
	"io"

	fn "knative.dev/func/pkg/functions"
	"knative.dev/func/pkg/k8s"
)

// setupClusterOverride looks up stored auth for clusterURL, applies a token
// override if clusterToken is non-empty, and configures the kubeconfig
// override.  Returns a cleanup function that must be deferred by the caller.
// Callers should guard the call with a clusterURL != "" check.
func setupClusterOverride(clusterURL, clusterToken, namespace string, local fn.Local, errOut io.Writer) (cleanup func(), err error) {
	var clusterTLS fn.ClusterVerify
	var user fn.UserAuth
	if entry := local.FindAuth(clusterURL); entry != nil {
		clusterTLS = entry.Cluster
		user = entry.User
	}
	if clusterToken != "" {
		user.Token = clusterToken
	}
	cleanup, err = k8s.SetClusterOverride(clusterURL, namespace, clusterTLS, user)
	if err != nil {
		return nil, fmt.Errorf("failed to set cluster override for %s: %w", clusterURL, err)
	}
	fmt.Fprintf(errOut, "Using cluster: %s\n", clusterURL)
	return cleanup, nil
}
