package knative

import (
	corev1 "k8s.io/api/core/v1"
	clientservingv1 "knative.dev/client/pkg/serving/v1"
	"knative.dev/pkg/apis"

	"github.com/boson-project/faas"
	"github.com/boson-project/faas/k8s"
)

const (
	labelKey   = "boson.dev/function"
	labelValue = "true"
)

type Lister struct {
	Verbose   bool
	namespace string
}

func NewLister(namespaceOverride string) (l *Lister, err error) {
	l = &Lister{}

	namespace, err := GetNamespace(namespaceOverride)
	if err != nil {
		return
	}
	l.namespace = namespace

	return
}

func (l *Lister) List() (items []faas.ListItem, err error) {

	client, err := NewServingClient(l.namespace)
	if err != nil {
		return
	}

	lst, err := client.ListServices(clientservingv1.WithLabel(labelKey, labelValue))
	if err != nil {
		return
	}

	for _, service := range lst.Items {

		// Convert the "subdomain-encoded" (i.e. kube-service-friendly) name
		// back out to a fully qualified service name.
		name, err := k8s.FromK8sAllowedName(service.Name)
		if err != nil {
			return items, err
		}

		// get status
		ready := corev1.ConditionUnknown
		for _, con := range service.Status.Conditions {
			if con.Type == apis.ConditionReady {
				ready = con.Status
				break
			}
		}

		listItem := faas.ListItem{
			Name:     name,
			Runtime:  service.Labels["boson.dev/runtime"],
			KService: service.Name,
			URL:      service.Status.URL.String(),
			Ready:    string(ready),
		}

		items = append(items, listItem)
	}
	return
}
