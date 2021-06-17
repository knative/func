package k8s

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func GetConfigMap(ctx context.Context, name, namespaceOverride string) (*corev1.ConfigMap, error) {

	namespace, err := GetNamespace(namespaceOverride)
	if err != nil {
		return nil, err
	}

	client, err := NewKubernetesClientset(namespace)
	if err != nil {
		return nil, err
	}

	return client.CoreV1().ConfigMaps(namespace).Get(ctx, name, metav1.GetOptions{})
}

func ListConfigMapsNames(ctx context.Context, namespaceOverride string) (names []string, err error) {

	namespace, err := GetNamespace(namespaceOverride)
	if err != nil {
		return
	}

	client, err := NewKubernetesClientset(namespace)
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
