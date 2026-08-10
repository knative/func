package knative

import (
	"context"
	"fmt"
	"os"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"knative.dev/pkg/apis"

	fn "knative.dev/func/pkg/functions"
)

type Lister struct {
	verbose bool
}

func NewLister(verbose bool) *Lister {
	return &Lister{verbose: verbose}
}

// List functions, optionally specifying a namespace.
func (l *Lister) List(ctx context.Context, namespace string) ([]fn.ListItem, error) {
	client, err := NewServingClient(namespace)
	if err != nil {
		return nil, err
	}

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
