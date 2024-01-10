package knative

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	clientservingv1 "knative.dev/client-pkg/pkg/serving/v1"
	"knative.dev/pkg/apis"

	fn "knative.dev/func/pkg/functions"
	"knative.dev/func/pkg/k8s"
	"knative.dev/func/pkg/k8s/labels"
)

type Lister struct {
	Namespace string
	verbose   bool
}

func NewLister(namespaceOverride string, verbose bool) *Lister {
	return &Lister{
		Namespace: namespaceOverride,
		verbose:   verbose,
	}
}

func (l *Lister) List(ctx context.Context) (items []fn.ListItem, err error) {
	if l.Namespace == "" {
		l.Namespace, err = k8s.GetDefaultNamespace()
		if err != nil {
			return nil, err
		}
	}

	client, err := NewServingClient(l.Namespace)
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

	listOfFunctions := lst.Items[:]
	for i, depLabelF := range lstDeprecated.Items {
		found := false
		for _, f := range lst.Items {
			if depLabelF.Name == f.Name && depLabelF.Namespace == f.Namespace {
				found = true
				break
			}
		}
		if !found {
			listOfFunctions = append(listOfFunctions, lstDeprecated.Items[i])
		}
	}
	// --- end of handling usage of deprecated function labels

	for _, f := range listOfFunctions {

		// get status
		ready := corev1.ConditionUnknown
		for _, con := range f.Status.Conditions {
			if con.Type == apis.ConditionReady {
				ready = con.Status
				break
			}
		}

		// --- handle usage of deprecated runtime labels (`boson.dev/runtime`)
		runtimeLabel := ""
		if val, ok := f.Labels[labels.FunctionRuntimeKey]; ok {
			runtimeLabel = val
		} else {
			runtimeLabel = f.Labels[labels.DeprecatedFunctionRuntimeKey]
		}
		// --- end of handling usage of deprecated runtime labels

		listItem := fn.ListItem{
			Name:      f.Name,
			Namespace: f.Namespace,
			Runtime:   runtimeLabel,
			URL:       f.Status.URL.String(),
			Ready:     string(ready),
		}

		items = append(items, listItem)
	}
	return
}
