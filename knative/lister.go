package knative

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	clientservingv1 "knative.dev/client/pkg/serving/v1"
	"knative.dev/pkg/apis"

	fn "knative.dev/kn-plugin-func"
	"knative.dev/kn-plugin-func/k8s"
)

const (
	labelFunctionKey           = "function.knative.dev"
	deprecatedLabelFunctionKey = "boson.dev/function"
	labelFunctionValue         = "true"
	labelRuntimeKey            = "function.knative.dev/runtime"
	deprecatedLabelRuntimeKey  = "boson.dev/runtime"
)

type Lister struct {
	Verbose   bool
	Namespace string
}

func NewLister(namespaceOverride string) (l *Lister, err error) {
	l = &Lister{}

	namespace, err := k8s.GetNamespace(namespaceOverride)
	if err != nil {
		return
	}
	l.Namespace = namespace

	return
}

func (l *Lister) List(ctx context.Context) (items []fn.ListItem, err error) {

	client, err := NewServingClient(l.Namespace)
	if err != nil {
		return
	}

	lst, err := client.ListServices(ctx, clientservingv1.WithLabel(labelFunctionKey, labelFunctionValue))
	if err != nil {
		return
	}

	// --- handle usage of deprecated function labels (`boson.dev/function`)
	lstDeprecated, err := client.ListServices(ctx, clientservingv1.WithLabel(deprecatedLabelFunctionKey, labelFunctionValue))
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
		if val, ok := f.Labels[labelRuntimeKey]; ok {
			runtimeLabel = val
		} else {
			runtimeLabel = f.Labels[deprecatedLabelRuntimeKey]
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
