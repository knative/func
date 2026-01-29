package keda

import (
	"context"
	"fmt"
	"time"

	"github.com/kedacore/http-add-on/operator/apis/http/v1alpha1"
	"github.com/kedacore/http-add-on/operator/generated/clientset/versioned"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
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
