package wasm

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	wasmv1alpha1 "github.com/cardil/knative-serving-wasm/pkg/apis/wasm/v1alpha1"
	wasmclientset "github.com/cardil/knative-serving-wasm/pkg/client/clientset/versioned"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"knative.dev/pkg/apis"

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

	// waitTimeout is how long to poll for WasmModule readiness after
	// create/update. Set to 0 to skip polling (useful in unit tests).
	waitTimeout time.Duration

	// pollInterval is how often to check the WasmModule status.
	pollInterval time.Duration
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

// WithDeployerWaitTimeout sets the readiness polling timeout.
// Use 0 to skip polling entirely (useful in unit tests where the fake
// clientset never updates WasmModule status).
func WithDeployerWaitTimeout(t time.Duration) DeployerOpt {
	return func(d *Deployer) {
		d.waitTimeout = t
	}
}

// NewDeployer creates a new WASM deployer with the given options.
func NewDeployer(opts ...DeployerOpt) *Deployer {
	d := &Deployer{
		clientsetProvider: defaultClientsetProvider,
		waitTimeout:       k8s.DefaultWaitingTimeout,
		pollInterval:      DefaultPollInterval,
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

	// Wait for WasmModule to become ready and extract URL.
	url, waitErr := d.waitForReady(ctx, cs, namespace, f.Name)
	if waitErr != nil {
		return fn.DeploymentResult{}, fmt.Errorf("WasmModule %q failed to become ready: %w", f.Name, waitErr)
	}

	return fn.DeploymentResult{
		Status:    status,
		URL:       url,
		Namespace: namespace,
	}, nil
}

// waitForReady polls the WasmModule until it becomes Ready=True or the
// timeout expires.  It mirrors the pattern used by the Knative deployer:
// three goroutines run in parallel — log streaming, runner image-pull check,
// and WasmModule status polling — and a select loop picks the first result.
//
// If waitTimeout == 0 the method returns immediately with a best-effort URL
// from a single GET (backward-compatible with unit tests that use fake
// clientsets which never update status).
//
// The log streaming and image-pull-check goroutines are guarded by the ksvc
// label selector ("serving.knative.dev/service={name}").  When the
// knative-serving-wasm controller migrates to a shared runner architecture
// (no per-module ksvc), those goroutines will find no pods and silently
// no-op, while the WasmModule status-polling goroutine continues to work
// because the controller always sets Ready=True/False regardless of
// architecture.
func (d *Deployer) waitForReady(
	ctx context.Context,
	cs wasmclientset.Interface,
	namespace, name string,
) (string, error) {
	// Skip polling — return best-effort URL (unit-test path).
	if d.waitTimeout == 0 {
		wm, err := cs.WasmV1alpha1().WasmModules(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return "", nil //nolint:nilerr // best-effort
		}
		if wm.Status.Address != nil && wm.Status.Address.URL != nil {
			return wm.Status.Address.URL.String(), nil
		}
		return "", nil
	}

	// Create a deadline context for the entire wait.
	waitCtx, cancel := context.WithTimeout(ctx, d.waitTimeout)
	defer cancel()

	// Log buffer — always collect logs; print to stderr on failure.
	var logBuf k8s.SynchronizedBuffer
	var logOut io.Writer = &logBuf
	if d.verbose {
		logOut = os.Stderr
	}

	// Channel types for goroutine results.
	type statusResult struct {
		url string
		err error
	}

	chStatus := make(chan statusResult, 1)
	chPodErr := make(chan string, 1) // carries a human-readable reason

	// Goroutine 1: stream logs from runner pods.
	// Uses the ksvc label selector set by the current 1:1 controller
	// architecture.  In the shared-runner model there will be no
	// per-module ksvc, so this goroutine silently produces no output.
	logSelector := "serving.knative.dev/service=" + name
	go func() {
		_ = k8s.GetPodLogsBySelector(waitCtx, namespace, logSelector, "user-container", "", nil, logOut)
	}()

	// Goroutine 2: watch runner pods for fatal container states.
	//
	// Failure policy (mirrors Knative deployer behaviour):
	//   ImagePullBackOff  → fail immediately; registry unreachable from cluster
	//   CrashLoopBackOff  → fail immediately; recurrent crash confirmed by k8s
	//   Terminated (exit≠0, first run) → record logs + reason; don't signal yet
	//   Terminated (exit≠0, restart > 0) → fail; second consecutive crash
	//
	// A single first-run crash is tolerated because the ksvc may restart the
	// pod once during initialisation. Signalling on the second crash (or when
	// k8s itself reports CrashLoopBackOff) avoids false-positive failures on
	// transient starts while still detecting reliably-broken modules.
	go func() {
		k8sClient, err := k8s.NewKubernetesClientset()
		if err != nil {
			return
		}
		var firstFailureReason string
		for {
			select {
			case <-waitCtx.Done():
				return
			case <-time.After(d.pollInterval):
			}
			pods, err := k8sClient.CoreV1().Pods(namespace).List(waitCtx, metav1.ListOptions{
				LabelSelector: logSelector,
			})
			if err != nil || len(pods.Items) == 0 {
				continue
			}
			for _, pod := range pods.Items {
				for _, cs := range pod.Status.ContainerStatuses {
					if cs.Name != "user-container" {
						continue
					}
					var reason string
					var failNow bool
					switch {
					case cs.State.Waiting != nil && cs.State.Waiting.Reason == "ImagePullBackOff":
						reason = "runner image cannot be pulled from inside the cluster (ImagePullBackOff)"
						failNow = true
					case cs.State.Waiting != nil && cs.State.Waiting.Reason == "CrashLoopBackOff":
						reason = firstFailureReason
						if reason == "" {
							reason = fmt.Sprintf("runner is crash-looping (CrashLoopBackOff) after %d restart(s)", cs.RestartCount)
						}
						failNow = true
					case cs.State.Terminated != nil && cs.State.Terminated.ExitCode != 0:
						r := fmt.Sprintf("runner exited with code %d", cs.State.Terminated.ExitCode)
						if cs.RestartCount > 0 {
							// Second (or later) consecutive crash — fail now.
							if firstFailureReason != "" {
								reason = firstFailureReason
							} else {
								reason = r
							}
							failNow = true
						} else if firstFailureReason == "" {
							// First crash — collect logs and remember the reason.
							firstFailureReason = r
							if logBytes, lerr := k8sClient.CoreV1().Pods(namespace).
								GetLogs(pod.Name, &corev1.PodLogOptions{
									Container: "user-container",
								}).DoRaw(waitCtx); lerr == nil && len(logBytes) > 0 {
								fmt.Fprintf(logOut, "\nRunner output:\n%s\n", strings.TrimSpace(string(logBytes)))
							}
						}
					}
					if reason != "" && failNow {
						select {
						case chPodErr <- reason:
						default:
						}
						return
					}
				}
			}
		}
	}()

	// Goroutine 3: poll WasmModule status.
	// This is the primary signal in both current (1:1 ksvc) and future
	// (shared runner) architectures: the controller always sets Ready.
	go func() {
		ticker := time.NewTicker(d.pollInterval)
		defer ticker.Stop()
		for {
			select {
			case <-waitCtx.Done():
				chStatus <- statusResult{err: waitCtx.Err()}
				return
			case <-ticker.C:
				wm, err := cs.WasmV1alpha1().WasmModules(namespace).Get(waitCtx, name, metav1.GetOptions{})
				if err != nil {
					// Transient API error — keep trying.
					continue
				}
				cond := wm.Status.GetCondition(apis.ConditionReady)
				if cond == nil {
					// Not yet reconciled.
					continue
				}
				switch cond.Status {
				case corev1.ConditionTrue:
					url := ""
					if wm.Status.Address != nil && wm.Status.Address.URL != nil {
						url = wm.Status.Address.URL.String()
					}
					chStatus <- statusResult{url: url}
					return
				case corev1.ConditionFalse:
					// Transient conditions that resolve on their own:
					//   "ServiceUnavailable" — controller waits for backing ksvc
					//   "RevisionMissing"    — Knative Configuration hasn't
					//                          created its first Revision yet
					//                          (cold-start / cluster warm-up)
					// Continue polling — the controller will re-reconcile
					// and transition to Ready=True once the ksvc is up.
					// Any other reason is treated as a terminal failure.
					if cond.Reason == "ServiceUnavailable" || cond.Reason == "RevisionMissing" {
						continue
					}
					chStatus <- statusResult{
						err: fmt.Errorf("reason=%s: %s", cond.Reason, cond.Message),
					}
					return
				default:
					// Unknown — still reconciling.
				}
			}
		}
	}()

	// Select loop: pick the first result.
	select {
	case reason := <-chPodErr:
		dumpLogs(&logBuf)
		return "", fmt.Errorf("deploy error: %s", reason)

	case res := <-chStatus:
		if res.err != nil {
			dumpLogs(&logBuf)
			if isDeadlineExceeded(res.err) {
				return "", fmt.Errorf(
					"timed out waiting for WasmModule %q to become ready after %v; "+
						"run: kubectl describe wasmmodule/%s -n %s",
					name, d.waitTimeout, name, namespace)
			}
			return "", res.err
		}
		return res.url, nil
	}
}

// dumpLogs writes the buffered runner logs to stderr (called on failure when
// not already streaming verbosely).
func dumpLogs(buf *k8s.SynchronizedBuffer) {
	logs := buf.String()
	if logs != "" {
		fmt.Fprintln(os.Stderr, "\nRunner output:")
		fmt.Fprintln(os.Stderr, logs)
	}
}

// isDeadlineExceeded returns true for context deadline exceeded errors.
func isDeadlineExceeded(err error) bool {
	return err == context.DeadlineExceeded || strings.Contains(err.Error(), "context deadline exceeded")
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

	// Command-line arguments from run.args.
	spec.Args = f.Run.Args

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
