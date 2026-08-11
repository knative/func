package knative

import (
	"context"
	"fmt"
	"os"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	clientservingv1 "knative.dev/client/pkg/serving/v1"
	"knative.dev/func/pkg/k8s"
	servingv1 "knative.dev/serving/pkg/client/clientset/versioned/typed/serving/v1"

	fn "knative.dev/func/pkg/functions"
	"knative.dev/pkg/apis"
)

type Lister struct {
	kc      *k8s.Client
	verbose bool
}

func NewLister(kc *k8s.Client, verbose bool) *Lister {
	return &Lister{kc: kc, verbose: verbose}
}

// List functions, optionally specifying a namespace.
func (l *Lister) List(ctx context.Context, namespace string) ([]fn.ListItem, error) {
	if l.kc == nil {
		return nil, fmt.Errorf("kubernetes client is not initialized")
	}

	restConfig, err := l.kc.ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to get kubernetes client config: %v", err)
	}

	servingClient, err := servingv1.NewForConfig(restConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create serving client: %v", err)
	}

	client := clientservingv1.NewKnServingClient(servingClient, namespace)

	// TODO: shouldn't this list only services for functions (-> having the function.knative.dev/name label)?!?

	lst, err := client.ListServices(ctx)
	if err != nil {
		if IsCRDNotFoundError(err) {
			// no services found --> nothing to return
			return nil, nil
		}
		if errors.IsForbidden(err) {
			// namespace is empty when listing across all namespaces (--all-namespaces)
			grant := fmt.Sprintf("access to services.serving.knative.dev in namespace %q", namespace)
			if namespace == "" {
				grant = "cluster-wide access to services.serving.knative.dev"
			}
			fmt.Fprintf(os.Stderr, "Warning: cannot access Knative services (permission denied) - skipping; "+
				"grant %s to include functions deployed by the Knative deployer; "+
				"if you do not use the Knative deployer you can safely ignore this message\n", grant)
			return nil, nil
		}
		return nil, err
	}

	items := make([]fn.ListItem, 0, len(lst.Items))
	for _, service := range lst.Items {
		if !UsesKnativeDeployer(service.Annotations) {
			continue
		}

		// get status
		ready := corev1.ConditionUnknown
		for _, con := range service.Status.Conditions {
			if con.Type == apis.ConditionReady {
				ready = con.Status
				break
			}
		}

		runtimeLabel := ""

		listItem := fn.ListItem{
			Name:      service.Name,
			Namespace: service.Namespace,
			Runtime:   runtimeLabel,
			URL:       service.Status.URL.String(),
			Ready:     string(ready),
			Deployer:  KnativeDeployerName,
		}

		items = append(items, listItem)
	}

	return items, nil
}
