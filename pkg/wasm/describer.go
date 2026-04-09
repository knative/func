package wasm

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"knative.dev/pkg/apis"

	"knative.dev/func/pkg/deployer"
	fn "knative.dev/func/pkg/functions"
)

// Describer implements fn.Describer for WASI/WASM functions.
// It fetches WasmModule CRs from the cluster and maps them to fn.Instance.
type Describer struct {
	verbose bool

	// clientsetProvider is injectable for testing.
	clientsetProvider ClientsetProvider
}

// DescriberOpt is a functional option for Describer.
type DescriberOpt func(*Describer)

// WithDescriberVerbose enables verbose logging.
func WithDescriberVerbose(verbose bool) DescriberOpt {
	return func(d *Describer) {
		d.verbose = verbose
	}
}

// WithDescriberClientsetProvider injects a custom clientset provider (for testing).
func WithDescriberClientsetProvider(p ClientsetProvider) DescriberOpt {
	return func(d *Describer) {
		d.clientsetProvider = p
	}
}

// NewDescriber creates a new WASM describer with the given options.
func NewDescriber(opts ...DescriberOpt) *Describer {
	d := &Describer{
		clientsetProvider: defaultClientsetProvider,
	}
	for _, o := range opts {
		o(d)
	}
	return d
}

// Describe returns the runtime instance for a WasmModule CR.
// Implements fn.Describer.
func (d *Describer) Describe(ctx context.Context, name, namespace string) (fn.Instance, error) {
	if namespace == "" {
		return fn.Instance{}, fmt.Errorf("namespace is required when describing %q", name)
	}

	cs, err := d.clientsetProvider()
	if err != nil {
		return fn.Instance{}, err
	}

	wm, err := cs.WasmV1alpha1().WasmModules(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		if isCRDNotFoundError(err) {
			// CRD not installed → not our responsibility.
			return fn.Instance{}, fn.ErrNotHandled
		}
		if errors.IsNotFound(err) {
			// No such WasmModule → not our responsibility.
			return fn.Instance{}, fn.ErrNotHandled
		}
		return fn.Instance{}, fmt.Errorf("failed to get WasmModule %q: %w", name, err)
	}

	// Only describe WasmModules managed by this deployer.
	if wm.Annotations[deployer.DeployerNameAnnotation] != DeployerName {
		return fn.Instance{}, fn.ErrNotHandled
	}

	// Determine ready status.
	ready := ""
	for _, cond := range wm.Status.Conditions {
		if cond.Type == apis.ConditionReady {
			ready = string(cond.Status)
			break
		}
	}

	// Determine URL.
	url := ""
	if wm.Status.Address != nil && wm.Status.Address.URL != nil {
		url = wm.Status.Address.URL.String()
	}

	routes := []string{}
	if url != "" {
		routes = []string{url}
	}

	_ = ready // ready is informational; fn.Instance has no Ready field

	return fn.Instance{
		Name:      wm.Name,
		Namespace: wm.Namespace,
		Deployer:  DeployerName,
		Route:     url,
		Routes:    routes,
		Labels:    wm.Labels,
		Image:     wm.Spec.Image,
	}, nil
}
