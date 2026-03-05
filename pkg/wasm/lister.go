package wasm

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"knative.dev/pkg/apis"

	"knative.dev/func/pkg/deployer"
	fn "knative.dev/func/pkg/functions"
)

// Lister implements fn.Lister for WASI/WASM functions.
// It lists WasmModule CRs in the given namespace.
type Lister struct {
	verbose bool

	// clientsetProvider is injectable for testing.
	clientsetProvider ClientsetProvider
}

// ListerOpt is a functional option for Lister.
type ListerOpt func(*Lister)

// WithListerVerbose enables verbose logging.
func WithListerVerbose(verbose bool) ListerOpt {
	return func(l *Lister) {
		l.verbose = verbose
	}
}

// WithListerClientsetProvider injects a custom clientset provider (for testing).
func WithListerClientsetProvider(p ClientsetProvider) ListerOpt {
	return func(l *Lister) {
		l.clientsetProvider = p
	}
}

// NewLister creates a new WASM lister with the given options.
func NewLister(opts ...ListerOpt) *Lister {
	l := &Lister{
		clientsetProvider: defaultClientsetProvider,
	}
	for _, o := range opts {
		o(l)
	}
	return l
}

// List returns the WasmModule CRs deployed in the given namespace.
// Implements fn.Lister.
func (l *Lister) List(ctx context.Context, namespace string) ([]fn.ListItem, error) {
	cs, err := l.clientsetProvider()
	if err != nil {
		return nil, err
	}

	list, err := cs.WasmV1alpha1().WasmModules(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		if isCRDNotFoundError(err) {
			// CRD not installed → no WasmModules to list.
			return nil, nil
		}
		return nil, fmt.Errorf("listing WasmModules: %w", err)
	}

	items := make([]fn.ListItem, 0, len(list.Items))
	for _, wm := range list.Items {
		// Only surface WasmModules managed by this deployer.
		if wm.Annotations[deployer.DeployerNameAnnotation] != DeployerName && wm.Annotations[deployer.DeployerNameAnnotation] != "" {
			continue
		}

		ready := corev1.ConditionUnknown
		for _, cond := range wm.Status.Conditions {
			if cond.Type == apis.ConditionReady {
				ready = cond.Status
				break
			}
		}

		url := ""
		if wm.Status.Address != nil && wm.Status.Address.URL != nil {
			url = wm.Status.Address.URL.String()
		}

		runtime := wm.Labels["function.knative.dev/runtime"]

		items = append(items, fn.ListItem{
			Name:      wm.Name,
			Namespace: wm.Namespace,
			Runtime:   runtime,
			URL:       url,
			Ready:     string(ready),
			Deployer:  DeployerName,
		})
	}

	return items, nil
}
