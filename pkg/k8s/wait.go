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
	return wait.PollUntilContextTimeout(ctx, 1*time.Second, timeout, true, func(ctx context.Context) (bool, error) {
		deployment, err := clientset.AppsV1().Deployments(namespace).Get(ctx, deploymentName, metav1.GetOptions{})
		if err != nil {
			return false, err
		}

		return checkIfDeploymentIsAvailable(ctx, clientset, deployment)
	})
}

func WaitForDeploymentAvailableBySelector(ctx context.Context, clientset *kubernetes.Clientset, namespace, selector string, timeout time.Duration) error {
	return wait.PollUntilContextTimeout(ctx, 1*time.Second, timeout, true, func(ctx context.Context) (bool, error) {
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

				// Get the current ReplicaSet for this deployment
				replicaSets, err := clientset.AppsV1().ReplicaSets(deployment.Namespace).List(ctx, metav1.ListOptions{
					LabelSelector: metav1.FormatLabelSelector(deployment.Spec.Selector),
				})
				if err != nil {
					return false, err
				}

				// Find the current active ReplicaSet (the one with desired replicas > 0)
				var currentPodTemplateHash string
				for _, rs := range replicaSets.Items {
					if rs.Spec.Replicas != nil && *rs.Spec.Replicas > 0 {
						// The pod-template-hash label identifies pods from this ReplicaSet
						if hash, ok := rs.Labels["pod-template-hash"]; ok {
							currentPodTemplateHash = hash
							break
						}
					}
				}

				if currentPodTemplateHash == "" {
					return false, fmt.Errorf("could not find current pod-template-hash for deployment %s", deployment.Name)
				}

				// Verify all pods are from the current ReplicaSet and are running
				labelSelector := metav1.FormatLabelSelector(deployment.Spec.Selector)
				pods, err := clientset.CoreV1().Pods(deployment.Namespace).List(ctx, metav1.ListOptions{
					LabelSelector: labelSelector,
				})
				if err != nil {
					return false, err
				}

				// Count ready pods from current ReplicaSet only
				readyPods := 0
				for _, pod := range pods.Items {
					// Check if pod belongs to current ReplicaSet
					podHash, hasPodHash := pod.Labels["pod-template-hash"]
					if !hasPodHash || podHash != currentPodTemplateHash {
						// Pod is from an old ReplicaSet - deployment not fully rolled out
						if pod.DeletionTimestamp == nil {
							// Old pod still exists and not being deleted
							return false, nil
						}
						continue
					}

					// Check if pod is ready
					for _, podCondition := range pod.Status.Conditions {
						if podCondition.Type == corev1.PodReady && podCondition.Status == corev1.ConditionTrue {
							readyPods++
							break
						}
					}
				}

				// Ensure we have the desired number of running pods from current ReplicaSet
				if int32(readyPods) == desiredReplicas {
					return true, nil
				}
			}
		}
	}

	return false, nil
}
