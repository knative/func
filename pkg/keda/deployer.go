package keda

import (
	"context"
	"fmt"
	"strings"
	"time"

	httpv1alpha1 "github.com/kedacore/http-add-on/operator/apis/http/v1alpha1"
	v1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/utils/ptr"
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
	// Refuse an unbuildable name before the raw deployer creates a Deployment
	// and a Service the failure would leave behind. The Route's name is checked
	// the same way below, once the exposure conditions are known.
	if err := d.validateBridgeName(f); err != nil {
		return fn.DeploymentResult{}, err
	}

	k8sClientset, err := k8s.NewKubernetesClientset()
	if err != nil {
		return fn.DeploymentResult{}, fmt.Errorf("failed to create K8sClientset: %v", err)
	}

	// Resolved once per deploy and threaded down: every caller wants the same
	// answer, and it is needed before the raw deploy so the refusal below can
	// run before anything is created.
	interceptorNS, interceptorSeen := interceptorNamespace(ctx, k8sClientset)

	// Refuse rather than build a Route to a Service that may not be there:
	// such a Route is admitted and then serves nothing. Refusing before the
	// raw deploy leaves nothing half-built. The two refusals share the NO but
	// not the WHY: "not found" and "could not look" send an operator to
	// different fixes.
	if d.exposer != nil && fn.ActiveExpose(f.Expose) {
		// The Route's name needs the namespace the function will land in;
		// k8s.DeployNamespace is the same rule the raw deployer uses, so this
		// cannot validate a name the deploy will not use.
		exposeNS, err := k8s.DeployNamespace(f)
		if err != nil {
			return fn.DeploymentResult{}, err
		}
		if err := validateExposureName(f, exposeNS); err != nil {
			return fn.DeploymentResult{}, err
		}

		switch interceptorSeen {
		case interceptorAbsent:
			return fn.DeploymentResult{}, fmt.Errorf(
				"cannot expose function %q: the keda interceptor Service %q was not found in %s, "+
					"so its Route would point at nothing",
				f.Name, interceptorServiceName,
				strings.Join(interceptorNamespaceCandidates(), " or "))
		case interceptorUndetermined:
			return fn.DeploymentResult{}, fmt.Errorf(
				"cannot expose function %q: could not determine whether the keda interceptor Service %q "+
					"exists in %s, so its Route might point at nothing; this is usually a permissions "+
					"problem rather than a missing interceptor, and needs read access to that namespace",
				f.Name, interceptorServiceName,
				strings.Join(interceptorNamespaceCandidates(), " or "))
		}
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

	if err := d.ensureInterceptorBridgeService(ctx, k8sClientset, f, namespace, interceptorNS, deployment); err != nil {
		return fn.DeploymentResult{}, fmt.Errorf("failed to ensure proxy service exists: %w", err)
	}

	hosts := []string{
		fmt.Sprintf("%s.%s.svc", d.interceptorBridgeServiceName(f), namespace),
		d.interceptorBridgeServiceName(f),
	}
	url := fmt.Sprintf("http://%s:8080", hosts[0]) // TODO: check on HTTPS too

	// External exposure is reconciled around the HTTPScaledObject: creating
	// comes first, because the interceptor 404s any Host header the HSO does
	// not register; removing comes last, so a Forbidden in the interceptor's
	// namespace cannot leave a function deployed but unscaled. A nil exposer
	// means cluster-local, and neither half runs.
	var dynClient dynamic.Interface
	var exposedHost, appliedExpose string
	if d.exposer != nil {
		if dynClient, err = k8s.NewDynamicClient(); err != nil {
			return fn.DeploymentResult{}, fmt.Errorf("failed to create dynamic client: %w", err)
		}
		if fn.ActiveExpose(f.Expose) {
			// The interceptor refusal is NOT here. It is hoisted above the raw
			// deploy, since a refusal at this point leaves the function
			// half-built. Only the Route's name is checked here, because it is
			// the one check that needs the resolved namespace.
			exposedHost, err = d.exposer.Expose(ctx, dynClient, d.interceptorExposure(f, namespace, interceptorNS))
			if err != nil {
				return fn.DeploymentResult{}, fmt.Errorf("failed to expose function externally: %w", err)
			}
			hosts = append(hosts, exposedHost)
			// ocproute terminates TLS at the edge and redirects http.
			url = fmt.Sprintf("https://%s", exposedHost)
			appliedExpose = f.Expose
		}
	}

	if err := d.ensureHTTPScaledObject(ctx, f, namespace, deployment, appService, hosts); err != nil {
		return fn.DeploymentResult{}, fmt.Errorf("failed to ensure http scaled object exists: %w", err)
	}

	if d.exposer != nil {
		// Switching exposure off removes the Route BEFORE clearing the record,
		// so a failed removal leaves the record for the retry to act on; the
		// reverse order stranded the Route until delete. The record outlives
		// the Route, matching the raw deployer and the remover, at the price
		// of a dead hostname in describe until the retry lands. recordedNS is
		// read from appService, fetched before the exposure work above.
		if appliedExpose == "" {
			if recordedNS := appService.Annotations[k8s.RouteNamespaceAnnotation]; recordedNS != "" {
				if err := d.exposer.Unexpose(ctx, dynClient, interceptorExposureRef(f.Name, namespace, recordedNS)); err != nil {
					return fn.DeploymentResult{}, fmt.Errorf("failed to remove external exposure: %w", err)
				}
			}
		}

		// The location is recorded together with the hostname, by the code
		// that just created the Route and is certain where it went; removal
		// reads it instead of resolving the interceptor again. Cleared only
		// once the Route above is gone.
		routeNS := ""
		if exposedHost != "" {
			routeNS = interceptorNS
		}
		if err := k8s.SetRouteHostname(ctx, k8sClientset, namespace, f.Name, exposedHost, routeNS); err != nil {
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

func (d *Deployer) httpScaledObject(f fn.Function, namespace string, deployment *v1.Deployment, service *corev1.Service, hosts []string) (*httpv1alpha1.HTTPScaledObject, error) {
	if len(service.Spec.Ports) == 0 {
		return nil, fmt.Errorf("service %s has no ports defined", service.Name)
	}

	labels, err := deployer.GenerateCommonLabels(f, d.decorator)
	if err != nil {
		return nil, fmt.Errorf("failed to generate common labels: %w", err)
	}

	annotations := deployer.GenerateCommonAnnotations(f, d.decorator, false /*we don't care about dapr for the HttpScaledObject*/, KedaDeployerName)

	minScale := int32(1)
	maxScale := int32(10)
	if scaleOptions := f.Deploy.Options.Scale; scaleOptions != nil {
		if scaleOptions.Min != nil {
			minScale = int32(*scaleOptions.Min)
		}
		if scaleOptions.Max != nil {
			maxScale = int32(*scaleOptions.Max)
		}
	}

	return &httpv1alpha1.HTTPScaledObject{
		ObjectMeta: metav1.ObjectMeta{
			Name:        f.Name,
			Namespace:   namespace,
			Labels:      labels,
			Annotations: annotations,
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: "apps/v1",
					Kind:       "Deployment",
					Name:       deployment.Name,
					UID:        deployment.UID,
					Controller: ptr.To(true),
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
				Min: &minScale,
				Max: &maxScale,
			},
			CooldownPeriod: ptr.To(int32(300)),
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

func (d *Deployer) interceptorBridgeServiceName(f fn.Function) string {
	return f.Name + interceptorBridgeSuffix
}

func (d *Deployer) interceptorBridgeService(f fn.Function, namespace, interceptorNS string, deployment *v1.Deployment) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      d.interceptorBridgeServiceName(f),
			Namespace: namespace,
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: "apps/v1",
					Kind:       "Deployment",
					Name:       deployment.Name,
					UID:        deployment.UID,
					Controller: ptr.To(true),
				},
			},
		},
		Spec: corev1.ServiceSpec{
			Type:         corev1.ServiceTypeExternalName,
			ExternalName: fmt.Sprintf("%s.%s.svc.cluster.local", interceptorServiceName, interceptorNS),
		},
	}
}

// ensureInterceptorBridgeService makes sure to create the service which serves as the entrypoint to the function
// this service will server as an external-name service and forward the request to the keda interceptor-proxy by
// preserving the host name. This service name is also used in the HTTPScaledObject as host name to allow the
// interceptor to match the request with the correct target/scaledObject.
func (d *Deployer) ensureInterceptorBridgeService(ctx context.Context, clientset *kubernetes.Clientset, f fn.Function, namespace, interceptorNS string, deployment *v1.Deployment) error {
	expected := d.interceptorBridgeService(f, namespace, interceptorNS, deployment)
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

		if _, err = clientset.CoreV1().Services(namespace).Update(ctx, expected, metav1.UpdateOptions{}); err != nil {
			return fmt.Errorf("failed to update service to interceptor proxy: %w", err)
		}

		return nil
	}

	return nil
}

func (d *Deployer) ensureHTTPScaledObject(ctx context.Context, f fn.Function, namespace string, deployment *v1.Deployment, service *corev1.Service, hosts []string) error {
	expected, err := d.httpScaledObject(f, namespace, deployment, service, hosts)
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

			if err := WaitForHTTPScaledObjectAvailable(ctx, httpScaledObjectClientset, namespace, expected.Name, k8s.DefaultWaitingTimeout); err != nil {
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

		if err := WaitForHTTPScaledObjectAvailable(ctx, httpScaledObjectClientset, namespace, expected.Name, k8s.DefaultWaitingTimeout); err != nil {
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
