package k8s

import (
	"context"
	"fmt"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
)

// WaitForDeploymentAvailable waits for a specific deployment to be fully available.
// A deployment is considered available when:
// - The number of available replicas matches the desired replicas
// - All replicas are updated to the latest version
// - There are no unavailable replicas
func WaitForDeploymentAvailable(ctx context.Context, clientset *kubernetes.Clientset, namespace, deploymentName string, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	return wait.PollUntilContextCancel(ctx, 1*time.Second, true, func(ctx context.Context) (bool, error) {
		deployment, err := clientset.AppsV1().Deployments(namespace).Get(ctx, deploymentName, metav1.GetOptions{})
		if err != nil {
			return false, err
		}

		// Check if the deployment has the desired number of replicas
		if deployment.Spec.Replicas == nil {
			return false, fmt.Errorf("deployment %s has nil replicas", deploymentName)
		}

		desiredReplicas := *deployment.Spec.Replicas

		// Check if deployment is available
		for _, condition := range deployment.Status.Conditions {
			if condition.Type == appsv1.DeploymentAvailable && condition.Status == corev1.ConditionTrue {
				// Also verify that all replicas are updated, ready, and available
				if deployment.Status.UpdatedReplicas == desiredReplicas &&
					deployment.Status.ReadyReplicas == desiredReplicas &&
					deployment.Status.AvailableReplicas == desiredReplicas &&
					deployment.Status.UnavailableReplicas == 0 {
					return true, nil
				}
			}
		}

		return false, nil
	})
}
