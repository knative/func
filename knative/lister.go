package knative

import (
	clientservingv1 "knative.dev/client/pkg/serving/v1"

	"github.com/boson-project/faas/k8s"
)

const (
	labelKey   = "bosonFunction"
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

func (l *Lister) List() (names []string, err error) {

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
		n, err := k8s.FromK8sAllowedName(service.Name)
		if err != nil {
			return names, err
		}
		names = append(names, n)
	}
	return
}
