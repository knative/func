package wasm

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"knative.dev/func/pkg/deployer"
	fn "knative.dev/func/pkg/functions"
)

// Remover implements fn.Remover for WASI/WASM functions.
// It deletes WasmModule CRs from the cluster.
type Remover struct {
	verbose bool

	// clientsetProvider is injectable for testing.
	clientsetProvider ClientsetProvider
}

// RemoverOpt is a functional option for Remover.
type RemoverOpt func(*Remover)

// WithRemoverVerbose enables verbose logging.
func WithRemoverVerbose(verbose bool) RemoverOpt {
	return func(r *Remover) {
		r.verbose = verbose
	}
}

// WithRemoverClientsetProvider injects a custom clientset provider (for testing).
func WithRemoverClientsetProvider(p ClientsetProvider) RemoverOpt {
	return func(r *Remover) {
		r.clientsetProvider = p
	}
}

// NewRemover creates a new WASM remover with the given options.
func NewRemover(opts ...RemoverOpt) *Remover {
	r := &Remover{
		clientsetProvider: defaultClientsetProvider,
	}
	for _, o := range opts {
		o(r)
	}
	return r
}

// Remove deletes the WasmModule CR with the given name from the cluster.
// Implements fn.Remover.
func (r *Remover) Remove(ctx context.Context, name, namespace string) error {
	if namespace == "" {
		return fn.ErrNamespaceRequired
	}

	cs, err := r.clientsetProvider()
	if err != nil {
		return err
	}

	wasmClient := cs.WasmV1alpha1().WasmModules(namespace)

	// Check the WasmModule exists and is managed by this deployer.
	wm, err := wasmClient.Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		if isCRDNotFoundError(err) {
			// CRD not installed → this deployer is not responsible.
			return fn.ErrNotHandled
		}
		if errors.IsNotFound(err) {
			// Not a WasmModule → not our responsibility.
			return fn.ErrNotHandled
		}
		return fmt.Errorf("failed to get WasmModule %q: %w", name, err)
	}

	// Only remove WasmModules managed by this deployer.
	if wm.Annotations[deployer.DeployerNameAnnotation] != DeployerName {
		return fn.ErrNotHandled
	}

	if err := wasmClient.Delete(ctx, name, metav1.DeleteOptions{}); err != nil {
		if errors.IsNotFound(err) {
			// Already gone – treat as success.
			return nil
		}
		return fmt.Errorf("failed to delete WasmModule %q: %w", name, err)
	}

	if r.verbose {
		fmt.Printf("Deleted WasmModule %q from namespace %q\n", name, namespace)
	}

	return nil
}
