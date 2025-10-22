package knative

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"knative.dev/func/pkg/deployer"
	"knative.dev/func/pkg/knative"
	"knative.dev/pkg/apis"

	fn "knative.dev/func/pkg/functions"
)

type Getter struct {
	verbose bool
}

func NewGetter(verbose bool) *Getter {
	return &Getter{verbose: verbose}
}

// Get a function, optionally specifying a namespace.
func (l *Getter) Get(ctx context.Context, name, namespace string) (fn.ListItem, error) {
	client, err := knative.NewServingClient(namespace)
	if err != nil {
		return fn.ListItem{}, fmt.Errorf("unable to create knative client: %v", err)
	}

	service, err := client.GetService(ctx, name)
	if err != nil {
		return fn.ListItem{}, fmt.Errorf("unable to get knative service: %v", err)
	}

	// get status
	ready := corev1.ConditionUnknown
	for _, con := range service.Status.Conditions {
		if con.Type == apis.ConditionReady {
			ready = con.Status
			break
		}
	}

	runtimeLabel := ""

	listItem := fn.ListItem{
		Name:       service.Name,
		Namespace:  service.Namespace,
		Runtime:    runtimeLabel,
		URL:        service.Status.URL.String(),
		Ready:      string(ready),
		DeployType: deployer.KnativeDeployerName,
	}

	return listItem, nil
}
