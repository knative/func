package knative

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/clientcmd"
	servingv1client "knative.dev/serving/pkg/client/clientset/versioned/typed/serving/v1"

	"github.com/boson-project/faas/k8s"
)

const labelSelector = "bosonFunction"

type Lister struct {
	Verbose   bool
	namespace string
	client    *servingv1client.ServingV1Client
}

func NewLister(namespace string) (l *Lister, err error) {
	l = &Lister{}
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	clientConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, &clientcmd.ConfigOverrides{})
	if namespace == "" {
		namespace, _, _ = clientConfig.Namespace()
	}
	l.namespace = namespace
	config, err := clientConfig.ClientConfig()
	if err != nil {
		return
	}
	l.client, err = servingv1client.NewForConfig(config)
	if err != nil {
		return
	}
	return
}

func (l *Lister) List() (names []string, err error) {
	opts := metav1.ListOptions{LabelSelector: labelSelector}
	lst, err := l.client.Services(l.namespace).List(opts)
	if err != nil {
		return
	}
	for _, service := range lst.Items {
		// Convert the "subdomain-encoded" (i.e. kube-service-friendly) name
		// back out to a fully qualified service name.
		n, err := k8s.FromSubdomain(service.Name)
		if err != nil {
			return names, err
		}
		names = append(names, n)
	}
	return
}
