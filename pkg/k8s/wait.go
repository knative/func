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
// - All pods associated with the deployment are running
func WaitForDeploymentAvailable(ctx context.Context, clientset *kubernetes.Clientset, namespace, deploymentName string, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	return wait.PollUntilContextCancel(ctx, 1*time.Second, true, func(ctx context.Context) (bool, error) {
		deployment, err := clientset.AppsV1().Deployments(namespace).Get(ctx, deploymentName, metav1.GetOptions{})
		if err != nil {
			return false, err
		}

		return checkIfDeploymentIsAvailable(ctx, clientset, deployment)
	})
}

func WaitForDeploymentAvailableBySelector(ctx context.Context, clientset *kubernetes.Clientset, namespace, selector string, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	return wait.PollUntilContextCancel(ctx, 1*time.Second, true, func(ctx context.Context) (bool, error) {
		deployments, err := clientset.AppsV1().Deployments(namespace).List(ctx, metav1.ListOptions{
			LabelSelector: selector,
		})
		if err != nil {
			return false, err
		}

		for _, deployment := range deployments.Items {
			ready, err := checkIfDeploymentIsAvailable(ctx, clientset, &deployment)
			if err != nil || !ready {
				return ready, err
			}
		}

		return true, nil
	})
}

func checkIfDeploymentIsAvailable(ctx context.Context, clientset *kubernetes.Clientset, deployment *appsv1.Deployment) (bool, error) {
	// Check if the deployment has the desired number of replicas
	if deployment.Spec.Replicas == nil {
		return false, fmt.Errorf("deployment %s has nil replicas", deployment.Name)
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

				// Verify all pods are actually running
				labelSelector := metav1.FormatLabelSelector(deployment.Spec.Selector)
				pods, err := clientset.CoreV1().Pods(deployment.Namespace).List(ctx, metav1.ListOptions{
					LabelSelector: labelSelector,
				})
				if err != nil {
					return false, err
				}

				// Count ready pods
				readyPods := 0
				for _, pod := range pods.Items {
					for _, podCondition := range pod.Status.Conditions {
						if podCondition.Type == corev1.PodReady && podCondition.Status == corev1.ConditionTrue {
							readyPods++
							break
						}
					}
				}

				// Ensure we have the desired number of running pods
				if int32(readyPods) == desiredReplicas {
					return true, nil
				}
			}
		}
	}

	return false, nil
}
