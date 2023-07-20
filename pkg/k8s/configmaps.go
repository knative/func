package k8s

import (
	"context"
	"errors"
	"net"
	"syscall"

	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sclientcmd "k8s.io/client-go/tools/clientcmd"
)

func GetConfigMap(ctx context.Context, name, namespaceOverride string) (*corev1.ConfigMap, error) {
	client, namespace, err := NewClientAndResolvedNamespace(namespaceOverride)
	if err != nil {
		return nil, err
	}

	return client.CoreV1().ConfigMaps(namespace).Get(ctx, name, metav1.GetOptions{})
}

// ListConfigMapsNamesIfConnected lists names of ConfigMaps present and the current k8s context
// returns empty list, if not connected to any cluster
func ListConfigMapsNamesIfConnected(ctx context.Context, namespaceOverride string) (names []string, err error) {
	names, err = listConfigMapsNames(ctx, namespaceOverride)
	if err != nil {
		// not logged our authorized to access resources
		if k8serrors.IsForbidden(err) || k8serrors.IsUnauthorized(err) || k8serrors.IsInvalid(err) || k8serrors.IsTimeout(err) {
			return []string{}, nil
		}

		// non existent k8s cluster
		var dnsErr *net.DNSError
		if errors.As(err, &dnsErr) {
			if dnsErr.IsNotFound || dnsErr.IsTemporary || dnsErr.IsTimeout {
				return []string{}, nil
			}
		}

		// connection refused
		if errors.Is(err, syscall.ECONNREFUSED) {
			return []string{}, nil
		}

		// invalid configuration: no configuration has been provided
		if k8sclientcmd.IsEmptyConfig(err) {
			return []string{}, nil
		}
	}

	return
}

func listConfigMapsNames(ctx context.Context, namespaceOverride string) (names []string, err error) {
	client, namespace, err := NewClientAndResolvedNamespace(namespaceOverride)
	if err != nil {
		return
	}

	cms, err := client.CoreV1().ConfigMaps(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return
	}

	for _, cm := range cms.Items {
		names = append(names, cm.Name)
	}

	return
}
