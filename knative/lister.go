package knative

import (
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

	namespace, err := GetNamespace(namespaceOverride)
	if err != nil {
		return
	}
	l.Namespace = namespace

	return
}

func (l *Lister) List() (items []bosonFunc.ListItem, err error) {

	client, err := NewServingClient(l.Namespace)
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

		listItem := bosonFunc.ListItem{
			Name:      name,
			Namespace: service.Namespace,
			Runtime:   service.Labels["boson.dev/runtime"],
			KService:  service.Name,
			URL:       service.Status.URL.String(),
			Ready:     string(ready),
		}

		items = append(items, listItem)
	}
	return
}
