package keda

import (
	"context"
	"fmt"
	"time"

	"github.com/kedacore/http-add-on/operator/apis/http/v1alpha1"
	"github.com/kedacore/http-add-on/operator/generated/clientset/versioned"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/dynamic"
)

func WaitForHTTPScaledObjectAvailable(ctx context.Context, clientset *versioned.Clientset, namespace string, name string, timeout time.Duration) error {
	return wait.PollUntilContextTimeout(ctx, 1*time.Second, timeout, true, func(ctx context.Context) (bool, error) {
		httpScaledObject, err := clientset.HttpV1alpha1().HTTPScaledObjects(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return false, fmt.Errorf("error getting http scaled object: %w", err)
		}

		isReady := meta.IsStatusConditionTrue(httpScaledObject.Status.Conditions, v1alpha1.ConditionTypeReady)
		return isReady, nil
	})
}

// WaitForScaledObjectMinReplicas waits until the named ScaledObject, the one
// KEDA acts on, has a minReplicaCount of at least min.
func WaitForScaledObjectMinReplicas(ctx context.Context, dynClient dynamic.Interface, namespace, name string, min int64, timeout time.Duration) error {
	return wait.PollUntilContextTimeout(ctx, 1*time.Second, timeout, true, func(ctx context.Context) (bool, error) {
		so, err := dynClient.Resource(scaledObjectGVR).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
		if k8serrors.IsNotFound(err) {
			return false, nil
		}
		if err != nil {
			return false, fmt.Errorf("error getting scaled object: %w", err)
		}
		cur, _, _ := unstructured.NestedInt64(so.Object, "spec", "minReplicaCount")
		return cur >= min, nil
	})
}
