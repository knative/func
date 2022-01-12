package k8s

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func CreateServiceAccountWithSecret(ctx context.Context, name, namespaceOverride string, labels map[string]string, secretName string) (err error) {
	client, namespace, err := NewClientAndResolvedNamespace(namespaceOverride)
	if err != nil {
		return
	}

	sa := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels:    labels,
		},
		Secrets: []corev1.ObjectReference{
			{
				APIVersion: "v1",
				Kind:       "Secret",
				Name:       secretName,
			},
		},
	}

	_, err = client.CoreV1().ServiceAccounts(namespace).Create(ctx, sa, metav1.CreateOptions{})
	return
}
