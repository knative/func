package keda

import (
	"context"
	"fmt"
	"time"

	"github.com/kedacore/http-add-on/operator/generated/clientset/versioned"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
)

func WaitForHTTPScaledObjectAvailable(ctx context.Context, clientset *versioned.Clientset, namespace string, name string, timeout time.Duration) error {
	return wait.PollUntilContextTimeout(ctx, 1*time.Second, timeout, true, func(ctx context.Context) (bool, error) {
		httpScaledObject, err := clientset.HttpV1alpha1().HTTPScaledObjects(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return false, fmt.Errorf("error getting http scaled object: %w", err)
		}

		// Check if any Ready condition has status True
		// HTTPScaledObject may have multiple Ready conditions, we need to find one that is True
		// TODO: use Status.Conditions.GetReadyCondition() as soon as this is fixed in http-add-on
		for _, condition := range httpScaledObject.Status.Conditions {
			if condition.Type == "Ready" && condition.Status == "True" {
				return true, nil
			}
		}
		return false, nil
	})
}
