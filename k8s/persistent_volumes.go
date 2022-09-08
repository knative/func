package k8s

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func GetPersistentVolumeClaim(ctx context.Context, name, namespaceOverride string) (*corev1.PersistentVolumeClaim, error) {
	client, namespace, err := NewClientAndResolvedNamespace(namespaceOverride)
	if err != nil {
		return nil, err
	}

	return client.CoreV1().PersistentVolumeClaims(namespace).Get(ctx, name, metav1.GetOptions{})
}

func CreatePersistentVolumeClaim(ctx context.Context, name, namespaceOverride string, labels map[string]string, annotations map[string]string, accessMode corev1.PersistentVolumeAccessMode, resourceRequest resource.Quantity) (err error) {
	client, namespace, err := NewClientAndResolvedNamespace(namespaceOverride)
	if err != nil {
		return
	}

	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Namespace:   namespace,
			Labels:      labels,
			Annotations: annotations,
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{accessMode},
			Resources: corev1.ResourceRequirements{
				Requests: corev1.ResourceList{},
			},
		},
	}
	pvc.Spec.Resources.Requests[corev1.ResourceStorage] = resourceRequest

	_, err = client.CoreV1().PersistentVolumeClaims(namespace).Create(ctx, pvc, metav1.CreateOptions{})
	return
}

func DeletePersistentVolumeClaims(ctx context.Context, namespaceOverride string, listOptions metav1.ListOptions) (err error) {
	client, namespace, err := NewClientAndResolvedNamespace(namespaceOverride)
	if err != nil {
		return
	}

	return client.CoreV1().PersistentVolumeClaims(namespace).DeleteCollection(ctx, metav1.DeleteOptions{}, listOptions)
}
