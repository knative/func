package keda

import (
	"context"
	"fmt"
	"slices"
	"time"

	httpv1alpha1 "github.com/kedacore/http-add-on/operator/apis/http/v1alpha1"
	v1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
}

func NewDeployer(opts ...DeployerOpt) *Deployer {
	d := &Deployer{
		Deployer: *k8s.NewDeployer(
			// init with the kedaDeployerDecorator to have the correct deployer labels&annotations
			k8s.WithDeployerDecorator(&kedaDeployerDecorator{}),
			// Traffic has to reach the function through the interceptor, so
			// the embedded raw deployer must not expose the function's own
			// Service. This deployer creates its own Route to the
			// interceptor instead, in route.go.
			k8s.WithDeployerExposureDisabled(),
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
	// execute raw deployment deployer
	deployResult, err := d.Deployer.Deploy(ctx, f)
	if err != nil {
		return fn.DeploymentResult{}, fmt.Errorf("failed to deploy function via raw deployer: %w", err)
	}

	// create additional required keda resources
	namespace := deployResult.Namespace

	k8sClientset, err := k8s.NewKubernetesClientset()
	if err != nil {
		return fn.DeploymentResult{}, fmt.Errorf("failed to create K8sClientset: %v", err)
	}

	deployment, err := k8sClientset.AppsV1().Deployments(namespace).Get(ctx, f.Name, metav1.GetOptions{})
	if err != nil {
		return fn.DeploymentResult{}, fmt.Errorf("failed to get deployment %s/%s: %v", namespace, f.Name, err)
	}

	appService, err := k8sClientset.CoreV1().Services(namespace).Get(ctx, f.Name, metav1.GetOptions{})
	if err != nil {
		return fn.DeploymentResult{}, fmt.Errorf("failed to get service %s/%s: %v", namespace, f.Name, err)
	}

	if err := d.ensureInterceptorBridgeService(ctx, k8sClientset, f, namespace, deployment); err != nil {
		return fn.DeploymentResult{}, fmt.Errorf("failed to ensure proxy service exists: %w", err)
	}

	hosts := []string{
		fmt.Sprintf("%s.%s.svc", d.interceptorBridgeServiceName(f), namespace),
		d.interceptorBridgeServiceName(f),
	}

	tech := f.Deploy.Expose

	dynClient, err := k8s.NewDynamicClient()
	if err != nil {
		return fn.DeploymentResult{}, fmt.Errorf("failed to create dynamic client: %w", err)
	}

	exposedURL := fmt.Sprintf("http://%s:8080", hosts[0])
	removeStaleRoute := false

	// "route" is an explicit request; unset means yes on OpenShift, no anywhere
	// else. Otherwise remove any Route an earlier deploy created. Both branches are
	// OpenShift-only because Routes exist nowhere else.
	// An explicit expose:route on non-OpenShift cluster never reaches here, it
	// would have been rejected in the embedded raw deployer - k8s.Deployer.resolveExposure()
	if tech == "route" || (tech == "" && k8s.IsOpenShift()) {
		host, err := ensureInterceptorRoute(ctx, dynClient, f, namespace, d.decorator)
		if err != nil {
			return fn.DeploymentResult{}, fmt.Errorf("failed to ensure interceptor Route: %w", err)
		}
		// The interceptor matches on the literal Host header, so the Route's
		// hostname has to be in hosts as well.
		if !slices.Contains(hosts, host) {
			hosts = append(hosts, host)
		}
		// The Route redirects http to https (see generateInterceptorRoute).
		exposedURL = fmt.Sprintf("https://%s", host)
	} else if k8s.IsOpenShift() {
		// Not exposed, so a Route from an earlier deploy must go. Deferred until
		// after the scaler exists: see the removal below.
		removeStaleRoute = true
	}

	if err := d.ensureHTTPScaledObject(ctx, f, namespace, deployment, appService, hosts); err != nil {
		return fn.DeploymentResult{}, fmt.Errorf("failed to ensure http scaled object exists: %w", err)
	}

	// Removing the stale Route runs LAST, after the HTTPScaledObject exists, for the
	// same reason remove() deletes it last: the call needs Route permissions in the
	// interceptor namespace, and a Forbidden there must not leave the function
	// deployed but unscaled. Ordered this way, the error is still fatal - an
	// explicit expose:none that cannot be honoured has to be reported - but the
	// function it leaves behind is complete rather than half-configured.
	//
	// A Route that outlives this call is inert, not a leak of exposure: the
	// interceptor matches the literal Host header against the hosts registered by
	// an HTTPScaledObject, and expose:none registers only the cluster-local bridge
	// hosts. Verified on OpenShift with the interceptor's own Route recreated by
	// hand: that hostname answers 404 while the bridge answers 200.
	if removeStaleRoute {
		if err := removeInterceptorRoute(ctx, dynClient, f.Name, namespace); err != nil {
			return fn.DeploymentResult{}, err
		}
	}

	return fn.DeploymentResult{
		Status:    deployResult.Status,
		URL:       exposedURL,
		Namespace: deployResult.Namespace,
		Deployer:  KedaDeployerName,
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
	return fmt.Sprintf("%s-interceptor-bridge", f.Name)
}

func (d *Deployer) interceptorBridgeService(f fn.Function, namespace string, deployment *v1.Deployment) *corev1.Service {
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
			ExternalName: fmt.Sprintf("%s.%s.svc.cluster.local", interceptorServiceName, interceptorNamespace()),
		},
	}
}

// ensureInterceptorBridgeService makes sure to create the service which serves as the entrypoint to the function
// this service will server as an external-name service and forward the request to the keda interceptor-proxy by
// preserving the host name. This service name is also used in the HTTPScaledObject as host name to allow the
// interceptor to match the request with the correct target/scaledObject.
func (d *Deployer) ensureInterceptorBridgeService(ctx context.Context, clientset *kubernetes.Clientset, f fn.Function, namespace string, deployment *v1.Deployment) error {
	expected := d.interceptorBridgeService(f, namespace, deployment)
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
