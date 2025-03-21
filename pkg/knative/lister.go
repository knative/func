package knative

import (
	"context"

	corev1 "k8s.io/api/core/v1"
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
func (l *Lister) List(ctx context.Context, namespace string) (items []fn.ListItem, err error) {
	client, err := NewServingClient(namespace)
	if err != nil {
		return
	}

	lst, err := client.ListServices(ctx)
	if err != nil {
		return
	}

	services := lst.Items[:]

	for _, service := range services {

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
		}

		items = append(items, listItem)
	}
	return
}
