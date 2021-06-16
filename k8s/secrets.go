package k8s

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func GetSecret(ctx context.Context, name, namespaceOverride string) (*corev1.Secret, error) {

	namespace, err := GetNamespace(namespaceOverride)
	if err != nil {
		return nil, err
	}

	client, err := NewKubernetesClientset(namespace)
	if err != nil {
		return nil, err
	}

	return client.CoreV1().Secrets(namespace).Get(ctx, name, metav1.GetOptions{})
}

func ListSecretsNames(ctx context.Context, namespaceOverride string) (names []string, err error) {

	namespace, err := GetNamespace(namespaceOverride)
	if err != nil {
		return
	}

	client, err := NewKubernetesClientset(namespace)
	if err != nil {
		return
	}

	secrets, err := client.CoreV1().Secrets(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return
	}

	for _, s := range secrets.Items {
		names = append(names, s.Name)
	}

	return
}
