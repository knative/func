package k8s

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	v1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	fn "knative.dev/func/pkg/functions"
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

	clientset, err := NewKubernetesClientset()
	if err != nil {
		return fn.Instance{}, fmt.Errorf("unable to create k8s client: %v", err)
	}

	deploymentClient := clientset.AppsV1().Deployments(namespace)
	serviceClient := clientset.CoreV1().Services(namespace)

	service, err := serviceClient.Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			// Service doesn't exist - we don't handle this
			return fn.Instance{}, fn.ErrNotHandled
		}

		// Other errors (permissions, network, etc.) - real error
		return fn.Instance{}, fmt.Errorf("failed to check if service uses raw K8s deployer: %w", err)
	}

	if !UsesRawDeployer(service.Annotations) {
		return fn.Instance{}, fn.ErrNotHandled
	}

	// We're responsible, for this function --> proceed...

	deployment, err := deploymentClient.Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return fn.Instance{}, fmt.Errorf("unable to get deployment %q: %v", name, err)
	}

	ready := corev1.ConditionUnknown
	for _, con := range deployment.Status.Conditions {
		if con.Type == v1.DeploymentAvailable {
			ready = con.Status
			break
		}
	}

	primaryRouteURL := fmt.Sprintf("http://%s.%s.svc", name, namespace) // TODO: get correct scheme?

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
		Deployer:  KubernetesDeployerName,
		Labels:    deployment.Labels,
		Route:     primaryRouteURL,
		Routes:    []string{primaryRouteURL},
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
