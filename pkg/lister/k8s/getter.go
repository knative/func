package k8s

import (
	"context"
	"fmt"

	v1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"knative.dev/func/pkg/deployer"
	fn "knative.dev/func/pkg/functions"
	"knative.dev/func/pkg/k8s"
)

type Getter struct {
	verbose bool
}

func NewGetter(verbose bool) *Getter {
	return &Getter{verbose: verbose}
}

// Get a function, optionally specifying a namespace.
func (l *Getter) Get(ctx context.Context, name, namespace string) (fn.ListItem, error) {
	clientset, err := k8s.NewKubernetesClientset()
	if err != nil {
		return fn.ListItem{}, fmt.Errorf("could not setup kubernetes clientset: %w", err)
	}

	deploymentClient := clientset.AppsV1().Deployments(namespace)
	serviceClient := clientset.CoreV1().Services(namespace)

	deployment, err := deploymentClient.Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return fn.ListItem{}, fmt.Errorf("could not get deployment: %w", err)
	}

	// get status
	ready := corev1.ConditionUnknown
	for _, con := range deployment.Status.Conditions {
		if con.Type == v1.DeploymentAvailable {
			ready = con.Status
			break
		}
	}

	service, err := serviceClient.Get(ctx, deployment.Name, metav1.GetOptions{})
	if err != nil {
		return fn.ListItem{}, fmt.Errorf("could not get service: %w", err)
	}

	runtimeLabel := ""
	listItem := fn.ListItem{
		Name:       service.Name,
		Namespace:  service.Namespace,
		Runtime:    runtimeLabel,
		URL:        fmt.Sprintf("http://%s.%s.svc", service.Name, service.Namespace), // TODO: use correct scheme
		Ready:      string(ready),
		DeployType: deployer.KubernetesDeployerName,
	}

	return listItem, nil
}
