package keda

import (
	"context"
	"fmt"
	"time"

	httpv1alpha1 "github.com/kedacore/http-add-on/operator/apis/http/v1alpha1"
	v1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"knative.dev/func/pkg/deployer"
	"knative.dev/func/pkg/deployers"
	fn "knative.dev/func/pkg/functions"
	"knative.dev/func/pkg/k8s"
)

const (
	KedaDeployerName = deployers.Keda
)

type DeployerOpt func(*Deployer)

type Deployer struct {
	k8s.Deployer

	verbose   bool
	decorator deployer.DeployDecorator
	exposer   deployer.Exposer
}

func NewDeployer(opts ...DeployerOpt) *Deployer {
	d := &Deployer{
		Deployer: *k8s.NewDeployer(
			// init with the kedaDeployerDecorator to have the correct deployer labels&annotations
			k8s.WithDeployerDecorator(&kedaDeployerDecorator{}),
		),
	}

	for _, opt := range opts {
		opt(d)
	}
	return d
}

func WithDeployerVerbose(verbose bool) DeployerOpt {
	return func(d *Deployer) {
		d.verbose = verbose
		k8s.WithDeployerVerbose(verbose)(&d.Deployer)
	}
}

func WithExposer(exposer deployer.Exposer) DeployerOpt {
	return func(d *Deployer) {
		d.exposer = exposer
	}
}

func WithDeployerDecorator(decorator deployer.DeployDecorator) DeployerOpt {
	// use the custom keda decorator, which wraps the given decorator,
	// but with the keda specific annotations
	kedaDecorator := &kedaDeployerDecorator{
		wrapper: decorator,
	}

	return func(d *Deployer) {
		d.decorator = kedaDecorator
		k8s.WithDeployerDecorator(kedaDecorator)(&d.Deployer)
	}
}

var _ deployer.DeployDecorator = &kedaDeployerDecorator{}

type kedaDeployerDecorator struct {
	wrapper deployer.DeployDecorator
}

func (k *kedaDeployerDecorator) UpdateAnnotations(function fn.Function, annotations map[string]string) map[string]string {
	if k.wrapper != nil {
		annotations = k.wrapper.UpdateAnnotations(function, annotations)
	}

	// set correct deployer name
	annotations[deployer.DeployerNameAnnotation] = KedaDeployerName

	return annotations
}

func (k *kedaDeployerDecorator) UpdateLabels(function fn.Function, labels map[string]string) map[string]string {
	if k.wrapper != nil {
		labels = k.wrapper.UpdateLabels(function, labels)
	}

	return labels
}

func (d *Deployer) Deploy(ctx context.Context, f fn.Function) (fn.DeploymentResult, error) {
	if err := validateBridgeName(f.Name); err != nil {
		return fn.DeploymentResult{}, err
	}

	k8sClientset, err := k8s.NewKubernetesClientset()
	if err != nil {
		return fn.DeploymentResult{}, fmt.Errorf("failed to create K8sClientset: %v", err)
	}
	dynClient, err := k8s.NewDynamicClient()
	if err != nil {
		return fn.DeploymentResult{}, fmt.Errorf("failed to create dynamic client: %w", err)
	}

	// Resolved once per deploy and threaded down
	interceptorNS, exposeRefusal := interceptorNamespace(ctx, k8sClientset)

	// DNS label checks before we create anything on cluster
	if err := d.validateExposure(f, exposeRefusal); err != nil {
		return fn.DeploymentResult{}, err
	}

	// execute raw deployment deployer
	deployResult, err := d.Deployer.Deploy(ctx, f)
	if err != nil {
		return fn.DeploymentResult{}, fmt.Errorf("failed to deploy function via raw deployer: %w", err)
	}

	// create additional required keda resources
	namespace := deployResult.Namespace

	deployment, err := k8sClientset.AppsV1().Deployments(namespace).Get(ctx, f.Name, metav1.GetOptions{})
	if err != nil {
		return fn.DeploymentResult{}, fmt.Errorf("failed to get deployment %s/%s: %v", namespace, f.Name, err)
	}

	appService, err := k8sClientset.CoreV1().Services(namespace).Get(ctx, f.Name, metav1.GetOptions{})
	if err != nil {
		return fn.DeploymentResult{}, fmt.Errorf("failed to get service %s/%s: %v", namespace, f.Name, err)
	}

	ref := deployer.NewExposureRef(f.Name, namespace, interceptorNS)
	if err := ensureInterceptorBridgeService(ctx, k8sClientset, ref, deployment); err != nil {
		return fn.DeploymentResult{}, fmt.Errorf("failed to ensure proxy service exists: %w", err)
	}

	labels, err := deployer.GenerateCommonLabels(f, d.decorator)
	if err != nil {
		return fn.DeploymentResult{}, fmt.Errorf("failed to generate common labels: %w", err)
	}
	annotations := deployer.GenerateCommonAnnotations(f, d.decorator, false, KedaDeployerName)

	minScale, maxScale := replicaBounds(f)
	target := deployTarget{
		clientset:   k8sClientset,
		dynClient:   dynClient,
		ref:         ref,
		deployment:  deployment,
		appService:  appService,
		labels:      labels,
		annotations: annotations,
		minScale:    minScale,
		maxScale:    maxScale,
	}
	var url string
	appliedExpose := ""
	if d.exposer != nil && fn.ActiveExpose(f.Expose) {
		if url, err = d.deployExposed(ctx, target); err != nil {
			return fn.DeploymentResult{}, err
		}
		appliedExpose = f.Expose
	} else {
		if url, err = d.deployClusterLocal(ctx, target); err != nil {
			return fn.DeploymentResult{}, err
		}
	}

	return fn.DeploymentResult{
		Status:    deployResult.Status,
		URL:       url,
		Namespace: deployResult.Namespace,
		Deployer:  KedaDeployerName,
		Expose:    appliedExpose,
	}, nil
}

// validateExposure refuses, before anything is created, an exposure this
// deploy could not honor: a Route name Kubernetes would reject, or an
// interceptor that cannot be confirmed to exist (exposeRefusal, resolved by
// interceptorNamespace). Nothing to check when no exposure is wanted.
func (d *Deployer) validateExposure(f fn.Function, exposeRefusal error) error {
	if d.exposer == nil || !fn.ActiveExpose(f.Expose) {
		return nil
	}
	// The Route's name needs the namespace the function will land in;
	// k8s.DeployNamespace is the same rule the raw deployer uses, so this
	// cannot validate a name the deploy will not use.
	exposeNS, err := k8s.DeployNamespace(f)
	if err != nil {
		return err
	}
	if err := validateExposureName(f, exposeNS); err != nil {
		return err
	}
	// Refuse rather than build a Route to a Service that may not be there:
	// such a Route is admitted and then serves nothing. The two refusals
	// share the NO but not the WHY: "not found" and "could not look" send an
	// operator to different fixes.
	if exposeRefusal != nil {
		return fmt.Errorf("cannot expose function %q: %w", f.Name, exposeRefusal)
	}
	return nil
}

// deployTarget is everything one keda deploy resolved and fetched before
// choosing a path: the clients, the function's placement, replica bounds,
// and the live objects the HSO hangs off
type deployTarget struct {
	clientset   kubernetes.Interface
	dynClient   dynamic.Interface
	ref         deployer.ExposureRef
	deployment  *v1.Deployment
	appService  *corev1.Service
	labels      map[string]string
	annotations map[string]string
	minScale    int32
	maxScale    int32
}

// bridgeHosts are the cluster-local names the HSO registers for f: requests
// through the bridge Service reach the interceptor carrying one of these.
func bridgeHosts(ref deployer.ExposureRef) []string {
	return []string{
		fmt.Sprintf("%s.%s.svc", interceptorBridgeServiceName(ref.FunctionName), ref.FunctionNamespace),
		interceptorBridgeServiceName(ref.FunctionName),
	}
}

// deployExposed settles an exposed function in the order Route -> HSO ->
// record. The Route goes first because the router mints the hostname and the
// HSO write is where that hostname gets registered: the interceptor 404s any
// Host header no HSO registers. The record is last: teardown and describe
// read it, never the cluster. A record that cannot be written takes the
// just-created Route back down. A kill between create and record still
// orphans, and delete will not collect that Route; the next exposed deploy
// reclaims it, because Expose finds an existing Route by the function's
// labels. Only reached with an Exposer and an active intent.
func (d *Deployer) deployExposed(ctx context.Context, t deployTarget) (string, error) {
	exposedHost, err := d.exposer.Expose(ctx, t.dynClient, interceptorExposure(t.ref, t.labels, t.annotations))
	if err != nil {
		return "", fmt.Errorf("failed to expose function externally: %w", err)
	}

	hosts := append(bridgeHosts(t.ref), exposedHost)
	if err := ensureHTTPScaledObject(ctx, t, hosts); err != nil {
		return "", fmt.Errorf("failed to ensure http scaled object exists: %w", err)
	}

	// reconcile annotations to function service about exposure
	if err := k8s.RecordExposure(ctx, t.clientset, t.ref, exposedHost); err != nil {
		if rbErr := d.exposer.Unexpose(ctx, t.dynClient, t.ref); rbErr != nil {
			return "", fmt.Errorf("recording the exposure failed: %w; rolling the Route back failed too: %v", err, rbErr)
		}
		return "", fmt.Errorf("recording the exposure failed, the Route was rolled back: %w", err)
	}

	// ocproute terminates TLS at the edge and redirects http.
	return fmt.Sprintf("https://%s", exposedHost), nil
}

// deployClusterLocal settles a cluster-local function in the order HSO ->
// removal -> record. The HSO shrink kills external traffic first: dropping
// the exposed hostname from the host list makes the interceptor 404 it, so a
// Forbidden in the interceptor's namespace (while removing the now-dead
// Route in clearExposure) fails the deploy with the function scalable and
// effectively unexposed.
func (d *Deployer) deployClusterLocal(ctx context.Context, t deployTarget) (string, error) {
	hosts := bridgeHosts(t.ref)
	if err := ensureHTTPScaledObject(ctx, t, hosts); err != nil {
		return "", fmt.Errorf("failed to ensure http scaled object exists: %w", err)
	}

	if err := d.clearExposure(ctx, t, t.appService.Annotations[k8s.RouteNamespaceAnnotation]); err != nil {
		return "", err
	}

	return fmt.Sprintf("http://%s:8080", hosts[0]), nil // TODO: check on HTTPS too
}

// clearExposure Unexposes the recorded Route, then clears the Service
// record. Unexpose first so a failure leaves the record for retry. Nil
// exposer is a no-op. recordedNS is a parameter so tests can omit it.
func (d *Deployer) clearExposure(ctx context.Context, t deployTarget, recordedNS string) error {
	if d.exposer == nil {
		return nil
	}

	if recordedNS != "" {
		ref := t.ref
		ref.Namespace = recordedNS
		if err := d.exposer.Unexpose(ctx, t.dynClient, ref); err != nil {
			return fmt.Errorf("failed to remove external exposure: %w", err)
		}
	}

	// hostname == "" -> remove the record
	if err := k8s.RecordExposure(ctx, t.clientset, t.ref, ""); err != nil {
		return fmt.Errorf("failed to clear the exposure record: %w", err)
	}
	return nil
}

const (
	// defaultMinReplicas / defaultMaxReplicas are the HTTPScaledObject
	// replica bounds when the function does not set scale.min / scale.max.
	defaultMinReplicas int32 = 1
	defaultMaxReplicas int32 = 10
)

// replicaBounds is scale.min and scale.max from the function, or the
// defaults above when either is unset. The HTTPScaledObject spec requires
// both; these fallbacks are keda's, not shared with the raw or knative
// deployers.
func replicaBounds(f fn.Function) (min, max int32) {
	min, max = defaultMinReplicas, defaultMaxReplicas
	if scale := f.Deploy.Options.Scale; scale != nil {
		if scale.Min != nil {
			min = int32(*scale.Min)
		}
		if scale.Max != nil {
			max = int32(*scale.Max)
		}
	}
	return
}

func httpScaledObject(t deployTarget, hosts []string) (*httpv1alpha1.HTTPScaledObject, error) {
	deployment := t.deployment
	service := t.appService
	if len(service.Spec.Ports) == 0 {
		return nil, fmt.Errorf("service %s has no ports defined", service.Name)
	}

	return &httpv1alpha1.HTTPScaledObject{
		ObjectMeta: metav1.ObjectMeta{
			Name:        t.ref.FunctionName,
			Namespace:   t.ref.FunctionNamespace,
			Labels:      t.labels,
			Annotations: t.annotations,
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: "apps/v1",
					Kind:       "Deployment",
					Name:       deployment.Name,
					UID:        deployment.UID,
					Controller: new(true),
				},
			},
		},
		Spec: httpv1alpha1.HTTPScaledObjectSpec{
			Hosts: hosts,
			ScaleTargetRef: httpv1alpha1.ScaleTargetRef{
				APIVersion: "apps/v1",
				Kind:       "Deployment",
				Name:       deployment.Name,
				Service:    service.Name,
				Port:       service.Spec.Ports[0].Port,
			},
			Replicas: &httpv1alpha1.ReplicaStruct{
				Min: new(t.minScale),
				Max: new(t.maxScale),
			},
			CooldownPeriod: new(int32(300)),
			ScalingMetric: &httpv1alpha1.ScalingMetricSpec{
				Rate: &httpv1alpha1.RateMetricSpec{
					TargetValue: 100,
					Window: metav1.Duration{
						Duration: time.Minute,
					},
					Granularity: metav1.Duration{
						Duration: time.Second,
					},
				},
			},
		},
	}, nil
}

func interceptorBridgeServiceName(name string) string {
	return name + interceptorBridgeSuffix
}

func interceptorBridgeService(ref deployer.ExposureRef, deployment *v1.Deployment) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      interceptorBridgeServiceName(ref.FunctionName),
			Namespace: ref.FunctionNamespace,
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: "apps/v1",
					Kind:       "Deployment",
					Name:       deployment.Name,
					UID:        deployment.UID,
					Controller: new(true),
				},
			},
		},
		Spec: corev1.ServiceSpec{
			Type:         corev1.ServiceTypeExternalName,
			ExternalName: fmt.Sprintf("%s.%s.svc.cluster.local", interceptorServiceName, ref.Namespace),
		},
	}
}

// ensureInterceptorBridgeService makes sure to create the service which serves
// as the entrypoint to the function this service will server as an external-name
// service and forward the request to the keda interceptor-proxy by preserving
// the host name. This service name is also used in the HTTPScaledObject as
// host name to allow the interceptor to match the request with the correct
// target/scaledObject.
func ensureInterceptorBridgeService(ctx context.Context,
	clientset *kubernetes.Clientset, ref deployer.ExposureRef, deployment *v1.Deployment) error {

	expected := interceptorBridgeService(ref, deployment)
	existing, err := clientset.CoreV1().Services(expected.Namespace).Get(ctx, expected.Name, metav1.GetOptions{})
	if err != nil {
		if k8serrors.IsNotFound(err) {
			if _, err := clientset.CoreV1().Services(expected.Namespace).Create(ctx, expected, metav1.CreateOptions{}); err != nil {
				return fmt.Errorf("failed to create service to interceptor proxy: %w", err)
			}

			return nil
		}

		return fmt.Errorf("failed to get service to interceptor proxy: %w", err)
	}

	// check if we need to update
	if !equality.Semantic.DeepEqual(existing.Spec, expected.Spec) {
		// Preserve resource version for update
		expected.ResourceVersion = existing.ResourceVersion

		if _, err = clientset.CoreV1().Services(ref.FunctionNamespace).Update(ctx, expected, metav1.UpdateOptions{}); err != nil {
			return fmt.Errorf("failed to update service to interceptor proxy: %w", err)
		}

		return nil
	}

	return nil
}

func ensureHTTPScaledObject(ctx context.Context, t deployTarget, hosts []string) error {
	expected, err := httpScaledObject(t, hosts)
	if err != nil {
		return fmt.Errorf("failed to generate http scaled object: %w", err)
	}

	httpScaledObjectClientset, err := NewHTTPScaledObjectClientset()
	if err != nil {
		return fmt.Errorf("failed to create HTTPScaledObject clientset: %v", err)
	}

	existing, err := httpScaledObjectClientset.HttpV1alpha1().HTTPScaledObjects(expected.Namespace).Get(ctx, expected.Name, metav1.GetOptions{})
	if err != nil {
		if k8serrors.IsNotFound(err) {
			if _, err := httpScaledObjectClientset.HttpV1alpha1().HTTPScaledObjects(expected.Namespace).Create(ctx, expected, metav1.CreateOptions{}); err != nil {
				return fmt.Errorf("failed to create HTTPScaledObject: %w", err)
			}

			if err := WaitForHTTPScaledObjectAvailable(ctx, httpScaledObjectClientset, t.ref.FunctionNamespace, expected.Name, k8s.DefaultWaitingTimeout); err != nil {
				return fmt.Errorf("HTTPScaledObject did not become ready: %w", err)
			}

			return nil
		}

		return fmt.Errorf("failed to get HTTPScaledObject: %w", err)
	}

	// check if we need to update
	if !equality.Semantic.DeepEqual(existing.Spec, expected.Spec) {
		// Preserve resource version for update
		expected.ResourceVersion = existing.ResourceVersion

		if _, err = httpScaledObjectClientset.HttpV1alpha1().HTTPScaledObjects(expected.Namespace).Update(ctx, expected, metav1.UpdateOptions{}); err != nil {
			return fmt.Errorf("failed to update HTTPScaledObject: %w", err)
		}

		if err := WaitForHTTPScaledObjectAvailable(ctx, httpScaledObjectClientset, t.ref.FunctionNamespace, expected.Name, k8s.DefaultWaitingTimeout); err != nil {
			return fmt.Errorf("HTTPScaledObject did not become ready: %w", err)
		}

		return nil
	}

	return nil
}

func UsesKedaDeployer(annotations map[string]string) bool {
	deployer, ok := annotations[deployer.DeployerNameAnnotation]

	return ok && deployer == KedaDeployerName
}
