package wasm

import (
	"context"
	"fmt"
	"strings"

	wasmv1alpha1 "github.com/cardil/knative-serving-wasm/pkg/apis/wasm/v1alpha1"
	wasmclientset "github.com/cardil/knative-serving-wasm/pkg/client/clientset/versioned"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"knative.dev/func/pkg/deployer"
	fn "knative.dev/func/pkg/functions"
	"knative.dev/func/pkg/k8s"
)

// Deployer implements fn.Deployer for WASI/WASM functions.
// It creates or updates WasmModule custom resources on the cluster.
type Deployer struct {
	verbose   bool
	decorator deployer.DeployDecorator

	// clientsetProvider is injectable for testing.
	clientsetProvider ClientsetProvider
}

// DeployerOpt is a functional option for Deployer.
type DeployerOpt func(*Deployer)

// WithDeployerVerbose enables verbose logging.
func WithDeployerVerbose(verbose bool) DeployerOpt {
	return func(d *Deployer) {
		d.verbose = verbose
	}
}

// WithDeployerDecorator sets the deployment decorator.
func WithDeployerDecorator(dec deployer.DeployDecorator) DeployerOpt {
	return func(d *Deployer) {
		d.decorator = dec
	}
}

// WithDeployerClientsetProvider injects a custom clientset provider (for testing).
func WithDeployerClientsetProvider(p ClientsetProvider) DeployerOpt {
	return func(d *Deployer) {
		d.clientsetProvider = p
	}
}

// NewDeployer creates a new WASM deployer with the given options.
func NewDeployer(opts ...DeployerOpt) *Deployer {
	d := &Deployer{
		clientsetProvider: defaultClientsetProvider,
	}
	for _, o := range opts {
		o(d)
	}
	return d
}

// Deploy creates or updates a WasmModule CR for the given function.
// Implements fn.Deployer.
func (d *Deployer) Deploy(ctx context.Context, f fn.Function) (fn.DeploymentResult, error) {
	// Resolve the target namespace.
	namespace := f.Namespace
	if namespace == "" {
		namespace = f.Deploy.Namespace
	}
	if namespace == "" {
		return fn.DeploymentResult{}, fn.ErrNamespaceRequired
	}

	// Prefer the deployed image; fall back to the built image.
	if f.Deploy.Image == "" {
		f.Deploy.Image = f.Build.Image
	}
	if f.Deploy.Image == "" {
		return fn.DeploymentResult{}, fmt.Errorf("%w: function has no image set", ErrNoImageRef)
	}

	cs, err := d.clientsetProvider()
	if err != nil {
		return fn.DeploymentResult{}, err
	}

	wasmClient := cs.WasmV1alpha1().WasmModules(namespace)

	// Check if WasmModule already exists.
	existing, getErr := wasmClient.Get(ctx, f.Name, metav1.GetOptions{})
	if getErr != nil && !errors.IsNotFound(getErr) {
		if isCRDNotFoundError(getErr) {
			return fn.DeploymentResult{}, fmt.Errorf("%w: %w", ErrCRDNotFound, getErr)
		}
		return fn.DeploymentResult{}, fmt.Errorf("failed to get WasmModule %q: %w", f.Name, getErr)
	}

	module, err := d.buildWasmModule(f, namespace)
	if err != nil {
		return fn.DeploymentResult{}, fmt.Errorf("failed to build WasmModule spec: %w", err)
	}

	var status fn.Status

	if errors.IsNotFound(getErr) {
		// Create new WasmModule.
		if _, err = wasmClient.Create(ctx, module, metav1.CreateOptions{}); err != nil {
			if isCRDNotFoundError(err) {
				return fn.DeploymentResult{}, fmt.Errorf("%w: %w", ErrCRDNotFound, err)
			}
			return fn.DeploymentResult{}, fmt.Errorf("failed to create WasmModule %q: %w", f.Name, err)
		}
		status = fn.Deployed
		if d.verbose {
			fmt.Printf("Created WasmModule %q in namespace %q\n", f.Name, namespace)
		}
	} else {
		// Update existing WasmModule; preserve resource version.
		module.ResourceVersion = existing.ResourceVersion
		if _, err = wasmClient.Update(ctx, module, metav1.UpdateOptions{}); err != nil {
			return fn.DeploymentResult{}, fmt.Errorf("failed to update WasmModule %q: %w", f.Name, err)
		}
		status = fn.Updated
		if d.verbose {
			fmt.Printf("Updated WasmModule %q in namespace %q\n", f.Name, namespace)
		}
	}

	// Retrieve URL from status (may be empty on first deploy until controller reconciles).
	url := moduleURL(cs, ctx, namespace, f.Name)

	return fn.DeploymentResult{
		Status:    status,
		URL:       url,
		Namespace: namespace,
	}, nil
}

// buildWasmModule converts a fn.Function to a WasmModule CR.
func (d *Deployer) buildWasmModule(f fn.Function, namespace string) (*wasmv1alpha1.WasmModule, error) {
	labels, err := deployer.GenerateCommonLabels(f, d.decorator)
	if err != nil {
		return nil, err
	}

	annotations := deployer.GenerateCommonAnnotations(f, d.decorator, false, DeployerName)

	spec, err := buildWasmModuleSpec(f)
	if err != nil {
		return nil, err
	}

	return &wasmv1alpha1.WasmModule{
		ObjectMeta: metav1.ObjectMeta{
			Name:        f.Name,
			Namespace:   namespace,
			Labels:      labels,
			Annotations: annotations,
		},
		Spec: spec,
	}, nil
}

// buildWasmModuleSpec maps fn.Function fields to WasmModuleSpec fields.
func buildWasmModuleSpec(f fn.Function) (wasmv1alpha1.WasmModuleSpec, error) {
	spec := wasmv1alpha1.WasmModuleSpec{
		Image: f.Deploy.Image,
	}

	// Environment variables from run.envs.
	envVars, _, err := k8s.ProcessEnvs(f.Run.Envs, nil, nil)
	if err != nil {
		return spec, fmt.Errorf("processing envs: %w", err)
	}
	spec.Env = envVars

	// Volumes and mounts from run.volumes.
	volumes, mounts, err := k8s.ProcessVolumes(f.Run.Volumes, nil, nil, nil)
	if err != nil {
		return spec, fmt.Errorf("processing volumes: %w", err)
	}
	spec.Volumes = volumes
	spec.VolumeMounts = mounts

	// Resource requirements from deploy.options.resources.
	spec.Resources = buildResourceRequirements(f.Deploy.Options)

	// WASI network permissions from deploy.network.
	spec.Network = BuildNetworkSpec(f.Deploy.Network)

	return spec, nil
}

// buildResourceRequirements maps fn.Options to corev1.ResourceRequirements.
func buildResourceRequirements(opts fn.Options) corev1.ResourceRequirements {
	reqs := corev1.ResourceRequirements{}
	if opts.Resources == nil {
		return reqs
	}

	if opts.Resources.Requests != nil {
		reqs.Requests = corev1.ResourceList{}
		if opts.Resources.Requests.CPU != nil {
			if q, err := resource.ParseQuantity(*opts.Resources.Requests.CPU); err == nil {
				reqs.Requests[corev1.ResourceCPU] = q
			}
		}
		if opts.Resources.Requests.Memory != nil {
			if q, err := resource.ParseQuantity(*opts.Resources.Requests.Memory); err == nil {
				reqs.Requests[corev1.ResourceMemory] = q
			}
		}
	}

	if opts.Resources.Limits != nil {
		reqs.Limits = corev1.ResourceList{}
		if opts.Resources.Limits.CPU != nil {
			if q, err := resource.ParseQuantity(*opts.Resources.Limits.CPU); err == nil {
				reqs.Limits[corev1.ResourceCPU] = q
			}
		}
		if opts.Resources.Limits.Memory != nil {
			if q, err := resource.ParseQuantity(*opts.Resources.Limits.Memory); err == nil {
				reqs.Limits[corev1.ResourceMemory] = q
			}
		}
	}

	return reqs
}

// BuildNetworkSpec maps fn.NetworkSpec to wasmv1alpha1.NetworkSpec.
func BuildNetworkSpec(n *fn.NetworkSpec) *wasmv1alpha1.NetworkSpec {
	if n == nil {
		return nil
	}

	ns := &wasmv1alpha1.NetworkSpec{
		Inherit:           n.Inherit,
		AllowIPNameLookup: n.AllowIpNameLookup,
	}

	if n.Tcp != nil {
		ns.TCP = &wasmv1alpha1.TCPSpec{
			Bind:    n.Tcp.Bind,
			Connect: n.Tcp.Connect,
		}
	}

	if n.Udp != nil {
		ns.UDP = &wasmv1alpha1.UDPSpec{
			Bind:     n.Udp.Bind,
			Connect:  n.Udp.Connect,
			Outgoing: n.Udp.Outgoing,
		}
	}

	return ns
}

// moduleURL retrieves the URL from the WasmModule status (best-effort).
func moduleURL(cs wasmclientset.Interface, ctx context.Context, namespace, name string) string {
	wm, err := cs.WasmV1alpha1().WasmModules(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return ""
	}
	if wm.Status.Address != nil && wm.Status.Address.URL != nil {
		return wm.Status.Address.URL.String()
	}
	return ""
}

// isCRDNotFoundError returns true when the error indicates the WasmModule CRD
// is not installed on the cluster.
func isCRDNotFoundError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "no matches for kind") ||
		strings.Contains(msg, "the server could not find the requested resource") ||
		strings.Contains(msg, "no kind is registered")
}
