package keda

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/kedacore/http-add-on/operator/apis/http/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	fn "knative.dev/func/pkg/functions"
	"knative.dev/func/pkg/k8s"
)

type Describer struct {
	verbose   bool
	transport http.RoundTripper
}

type DescriberOpt func(*Describer)

func WithDescriberTransport(transport http.RoundTripper) DescriberOpt {
	return func(d *Describer) {
		d.transport = transport
	}
}

func NewDescriber(verbose bool, opts ...DescriberOpt) *Describer {
	d := &Describer{verbose: verbose}
	for _, o := range opts {
		o(d)
	}
	return d
}

// Describe a function by name.
func (d *Describer) Describe(ctx context.Context, name, namespace string) (fn.Instance, error) {
	if namespace == "" {
		return fn.Instance{}, fmt.Errorf("function namespace is required when describing %q", name)
	}

	clientset, err := k8s.NewKubernetesClientset()
	if err != nil {
		return fn.Instance{}, fmt.Errorf("unable to create k8s client: %v", err)
	}

	serviceClient := clientset.CoreV1().Services(namespace)

	service, err := serviceClient.Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			// Service doesn't exist - we don't handle this
			return fn.Instance{}, fn.ErrNotHandled
		}

		// Other errors (permissions, network, etc.) - real error
		return fn.Instance{}, fmt.Errorf("failed to check if service uses keda deployer: %w", err)
	}

	if !UsesKedaDeployer(service.Annotations) {
		return fn.Instance{}, fn.ErrNotHandled
	}

	// We're responsible, for this function --> proceed...

	httpScaledObjectClientset, err := NewHTTPScaledObjectClientset()
	if err != nil {
		return fn.Instance{}, fmt.Errorf("unable to create HTTPScaledObject client: %v", err)
	}

	httpScaledObject, err := httpScaledObjectClientset.HttpV1alpha1().HTTPScaledObjects(namespace).Get(ctx, name, metav1.GetOptions{})
	hasHTTPTrigger := true
	if err != nil {
		if !errors.IsNotFound(err) {
			return fn.Instance{}, fmt.Errorf("unable to get HTTPScaledObject: %w", err)
		}
		// A Kafka-only (or otherwise no-http-trigger) function never gets an
		// HTTPScaledObject at all -- that's expected, not a failure.
		hasHTTPTrigger = false
	}

	if !hasHTTPTrigger {
		// Absence of an HTTPScaledObject isn't proof this is a Kafka-only
		// function -- an http-triggered function's scaler could have been
		// deleted externally, failed to create, or be mid-transition.
		// Corroborate with the Kafka ScaledObject before assuming
		// Kafka-only; if neither scaler exists, something's broken and
		// should be reported, not silently treated as healthy.
		dynClient, err := k8s.NewDynamicClient()
		if err != nil {
			return fn.Instance{}, fmt.Errorf("unable to create dynamic client: %w", err)
		}
		if _, err := dynClient.Resource(scaledObjectGVR).Namespace(namespace).Get(ctx, scaledObjectName(name), metav1.GetOptions{}); err != nil {
			if errors.IsNotFound(err) {
				return fn.Instance{}, fmt.Errorf(
					"function %q uses the keda deployer but has neither an HTTPScaledObject nor a Kafka ScaledObject: the scaler may have failed to create or been deleted externally", name)
			}
			return fn.Instance{}, fmt.Errorf("unable to get ScaledObject: %w", err)
		}
	}

	deploymentClient := clientset.AppsV1().Deployments(namespace)
	deployment, err := deploymentClient.Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return fn.Instance{}, fmt.Errorf("unable to get deployment %q: %v", name, err)
	}

	var ready v1.ConditionStatus
	var primaryRouteURL string
	var routes []string
	expose := ""

	if hasHTTPTrigger {
		ready = v1.ConditionUnknown
		if meta.IsStatusConditionTrue(httpScaledObject.Status.Conditions, v1alpha1.ConditionTypeReady) {
			ready = v1.ConditionTrue
		} else if meta.IsStatusConditionFalse(httpScaledObject.Status.Conditions, v1alpha1.ConditionTypeReady) {
			ready = v1.ConditionFalse
		}

		if len(httpScaledObject.Spec.Hosts) == 0 {
			return fn.Instance{}, fmt.Errorf("HTTPScaledObject %q does not have any hosts", name)
		}

		// Deploy recorded the externally exposed hostname on the function's
		// own Service, so no second lookup is needed to tell the exposed
		// host apart from the bridge hosts it sits beside in Spec.Hosts.
		hostname := service.Annotations[k8s.RouteHostnameAnnotation]
		primaryRouteURL, routes = functionURLs(httpScaledObject.Spec.Hosts, hostname)
		if hostname != "" {
			expose = fn.ExposeRoute
		}
	} else {
		// No HTTP trigger: there's no interceptor/bridge host list to
		// report, so fall back to the Deployment's own readiness condition
		// and the cluster-local Service URL -- the same baseline the raw
		// k8s describer reports for an unexposed function.
		ready = v1.ConditionUnknown
		for _, cond := range deployment.Status.Conditions {
			if cond.Type == appsv1.DeploymentAvailable {
				ready = cond.Status
				break
			}
		}
		primaryRouteURL = fmt.Sprintf("http://%s.%s.svc", name, namespace)
		routes = []string{primaryRouteURL}
	}

	// get image
	image := ""
	for _, container := range deployment.Spec.Template.Spec.Containers {
		if container.Name == "user-container" {
			image = container.Image
		}
	}

	middlewareVersion := ""
	commit := ""
	if image != "" && d.transport != nil {
		labels, err := fn.ImageLabels(image, d.transport)
		if err == nil {
			middlewareVersion = labels[fn.MiddlewareVersionLabelKey]
			commit = labels[fn.CommitLabelKey]
		}
	}

	description := fn.Instance{
		Name:      name,
		Namespace: namespace,
		Deployer:  KedaDeployerName,
		Expose:    expose,
		Labels:    deployment.Labels,
		Route:     primaryRouteURL,
		Routes:    routes,
		Image:     image,
		Middleware: fn.Middleware{
			Version: middlewareVersion,
		},
		Revision:   commit,
		Generation: deployment.Generation,
		Ready:      strings.ToLower(string(ready)),
	}

	return description, nil
}
