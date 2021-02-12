package knative

import (
	"fmt"
	"time"

	"github.com/boson-project/func/k8s"
)

const RemoveTimeout = 120 * time.Second

func NewRemover(namespaceOverride string) (remover *Remover, err error) {
	remover = &Remover{}
	namespace, err := GetNamespace(namespaceOverride)
	if err != nil {
		return
	}
	remover.Namespace = namespace

	return
}

type Remover struct {
	Namespace string
	Verbose   bool
}

func (remover *Remover) Remove(name string) (err error) {

	serviceName, err := k8s.ToK8sAllowedName(name)
	if err != nil {
		return
	}

	client, err := NewServingClient(remover.Namespace)
	if err != nil {
		return
	}

	fmt.Printf("Removing Knative Service: %v\n", serviceName)

	err = client.DeleteService(serviceName, RemoveTimeout)
	if err != nil {
		err = fmt.Errorf("knative remover failed to delete the service: %v", err)
	}

	return
}
