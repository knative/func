package knative

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/clientcmd"
	servingv1client "knative.dev/serving/pkg/client/clientset/versioned/typed/serving/v1"
)

const labelSelector = "bosonFunction"

type Lister struct {
	Verbose   bool
	namespace string
	client    *servingv1client.ServingV1Client
}

func NewLister(namespace string) (l *Lister, err error) {
	l = &Lister{}
	l.namespace = namespace
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	clientConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, &clientcmd.ConfigOverrides{})
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
	opts := metav1.ListOptions{LabelSelector: "bosonFunction"}
	lst, err := l.client.Services(l.namespace).List(opts)
	if err != nil {
		return
	}
	for _, service := range lst.Items {
		names = append(names, service.Name)
	}
	return
}
