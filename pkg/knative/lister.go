package knative

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	clientservingv1 "knative.dev/client/pkg/serving/v1"
	"knative.dev/pkg/apis"

	fn "knative.dev/func/pkg/functions"
	"knative.dev/func/pkg/k8s/labels"
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

	lst, err := client.ListServices(ctx, clientservingv1.WithLabel(labels.FunctionKey, labels.FunctionValue))
	if err != nil {
		return
	}

	// --- handle usage of deprecated function labels (`boson.dev/function`)
	lstDeprecated, err := client.ListServices(ctx, clientservingv1.WithLabel(labels.DeprecatedFunctionKey, labels.FunctionValue))
	if err != nil {
		return
	}

	services := lst.Items[:]
	for i, depLabelF := range lstDeprecated.Items {
		found := false
		for _, f := range lst.Items {
			if depLabelF.Name == f.Name && depLabelF.Namespace == f.Namespace {
				found = true
				break
			}
		}
		if !found {
			services = append(services, lstDeprecated.Items[i])
		}
	}
	// --- end of handling usage of deprecated function labels

	for _, service := range services {

		// get status
		ready := corev1.ConditionUnknown
		for _, con := range service.Status.Conditions {
			if con.Type == apis.ConditionReady {
				ready = con.Status
				break
			}
		}

		// --- handle usage of deprecated runtime labels (`boson.dev/runtime`)
		runtimeLabel := ""
		if val, ok := service.Labels[labels.FunctionRuntimeKey]; ok {
			runtimeLabel = val
		} else {
			runtimeLabel = service.Labels[labels.DeprecatedFunctionRuntimeKey]
		}
		// --- end of handling usage of deprecated runtime labels

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
