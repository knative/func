package keda

import (
	"context"
	"fmt"
	"strings"

	"github.com/kedacore/http-add-on/operator/apis/http/v1alpha1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	fn "knative.dev/func/pkg/functions"
	"knative.dev/func/pkg/k8s"
)

type Describer struct {
	verbose bool
}

func NewDescriber(verbose bool) *Describer {
	return &Describer{
		verbose: verbose,
	}
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
	if err != nil {
		return fn.Instance{}, fmt.Errorf("unable to get HTTPScaledObject: %w", err)
	}

	ready := v1.ConditionUnknown
	if meta.IsStatusConditionTrue(httpScaledObject.Status.Conditions, v1alpha1.ConditionTypeReady) {
		ready = v1.ConditionTrue
	} else if meta.IsStatusConditionFalse(httpScaledObject.Status.Conditions, v1alpha1.ConditionTypeReady) {
		ready = v1.ConditionFalse
	}

	if len(httpScaledObject.Spec.Hosts) == 0 {
		return fn.Instance{}, fmt.Errorf("HTTPScaledObject %q does not have any hosts", name)
	}

	routes := make([]string, 0, len(httpScaledObject.Spec.Hosts))
	for _, host := range httpScaledObject.Spec.Hosts {
		routes = append(routes, fmt.Sprintf("http://%s:8080", host))
	}
	primaryRouteURL := routes[0]

	deploymentClient := clientset.AppsV1().Deployments(namespace)
	deployment, err := deploymentClient.Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return fn.Instance{}, fmt.Errorf("unable to get deployment %q: %v", name, err)
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
	if image != "" {
		v, err := fn.MiddlewareVersion(image)
		if err == nil {
			middlewareVersion = v
		}
		c, err := fn.ImageCommit(image)
		if err == nil {
			commit = c
		}
	}

	description := fn.Instance{
		Name:      name,
		Namespace: namespace,
		Deployer:  KedaDeployerName,
		Labels:    deployment.Labels,
		Route:     primaryRouteURL,
		Routes:    routes,
		Image:     image,
		Middleware: fn.Middleware{
			Version: middlewareVersion,
		},
		Commit:     commit,
		Generation: deployment.Generation,
		Ready:      strings.ToLower(string(ready)),
	}

	return description, nil
}
