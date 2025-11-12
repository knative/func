package knative

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	"knative.dev/func/pkg/k8s"
	"knative.dev/pkg/apis"

	fn "knative.dev/func/pkg/functions"
)

type Lister struct {
	verbose bool
}

func NewLister(verbose bool) *Lister {
	return &Lister{verbose: verbose}
}

// List functions, optionally specifying a namespace.
func (l *Lister) List(ctx context.Context, namespace string) ([]fn.ListItem, error) {
	client, err := NewServingClient(namespace)
	if err != nil {
		return nil, err
	}

	// TODO: shouldn't this list only services for functions (-> having the function.knative.dev/name label)?!?

	lst, err := client.ListServices(ctx)
	if err != nil {
		if k8s.IsCRDNotFoundError(err) {
			// no services found --> nothing to return
			return nil, nil
		}
		return nil, err
	}

	items := make([]fn.ListItem, 0, len(lst.Items))
	for _, service := range lst.Items {
		if !UsesKnativeDeployer(service.Annotations) {
			continue
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
			Name:      service.Name,
			Namespace: service.Namespace,
			Runtime:   runtimeLabel,
			URL:       service.Status.URL.String(),
			Ready:     string(ready),
			Deployer:  KnativeDeployerName,
		}

		items = append(items, listItem)
	}

	return items, nil
}
