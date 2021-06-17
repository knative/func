package knative

import (
	"context"
	corev1 "k8s.io/api/core/v1"
	clientservingv1 "knative.dev/client/pkg/serving/v1"
	"knative.dev/pkg/apis"

	bosonFunc "github.com/boson-project/func"
	"github.com/boson-project/func/k8s"
)

const (
	labelKey   = "boson.dev/function"
	labelValue = "true"
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

func (l *Lister) List(ctx context.Context) (items []bosonFunc.ListItem, err error) {

	client, err := NewServingClient(l.Namespace)
	if err != nil {
		return
	}

	lst, err := client.ListServices(ctx, clientservingv1.WithLabel(labelKey, labelValue))
	if err != nil {
		return
	}

	for _, service := range lst.Items {

		// get status
		ready := corev1.ConditionUnknown
		for _, con := range service.Status.Conditions {
			if con.Type == apis.ConditionReady {
				ready = con.Status
				break
			}
		}

		listItem := bosonFunc.ListItem{
			Name:      service.Name,
			Namespace: service.Namespace,
			Runtime:   service.Labels["boson.dev/runtime"],
			URL:       service.Status.URL.String(),
			Ready:     string(ready),
		}

		items = append(items, listItem)
	}
	return
}
