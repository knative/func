package keda

import (
	"context"
	"fmt"

	"github.com/kedacore/http-add-on/operator/apis/http/v1alpha1"
	"github.com/kedacore/http-add-on/operator/generated/clientset/versioned"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	fn "knative.dev/func/pkg/functions"
	"knative.dev/func/pkg/k8s"
	"knative.dev/func/pkg/k8s/labels"
)

type Lister struct {
	kc      *k8s.Client
	verbose bool
}

func NewLister(kc *k8s.Client, verbose bool) fn.Lister {
	return &Lister{
		kc:      kc,
		verbose: verbose,
	}
}

func (l *Lister) List(ctx context.Context, namespace string) ([]fn.ListItem, error) {
	if l.kc == nil {
		return nil, fmt.Errorf("kubernetes client is not initialized")
	}
	clientset, err := l.kc.Clientset()
	if err != nil {
		return nil, fmt.Errorf("unable to create k8s client: %v", err)
	}

	restConfig, err := l.kc.ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("unable to get kubernetes client config: %v", err)
	}

	httpScaledObjectClientset, err := versioned.NewForConfig(restConfig)
	if err != nil {
		return nil, fmt.Errorf("unable to create HTTPScaledObject client: %v", err)
	}

	dynClient, err := dynamic.NewForConfig(restConfig)
	if err != nil {
		return nil, fmt.Errorf("unable to create dynamic client: %v", err)
	}

	serviceClient := clientset.CoreV1().Services(namespace)

	services, err := serviceClient.List(ctx, metav1.ListOptions{
		LabelSelector: "function.knative.dev/name",
	})
	if err != nil {
		return nil, fmt.Errorf("unable to list services: %v", err)
	}

	listItems := make([]fn.ListItem, 0, len(services.Items))
	for _, service := range services.Items {
		if !UsesKedaDeployer(service.Annotations) {
			continue
		}

		runtime := service.Labels[labels.FunctionRuntimeKey]
		item, err := l.get(ctx, clientset, httpScaledObjectClientset, dynClient, service.Name,
			service.Namespace, runtime, service.Annotations[k8s.RouteHostnameAnnotation])
		if err != nil {
			return nil, fmt.Errorf("unable to get details about function: %v", err)
		}

		listItems = append(listItems, item)
	}

	return listItems, nil
}

// Get a function, optionally specifying a namespace. exposedHost is the
// hostname Deploy recorded on the function's Service, empty when the function
// is cluster-local; List reads it there rather than looking the exposing
// object up again.
func (l *Lister) get(ctx context.Context, clientset *kubernetes.Clientset, httpScaledObjectClientset *versioned.Clientset, dynClient dynamic.Interface, name, namespace, runtime, exposedHost string) (fn.ListItem, error) {
	httpScaledObject, err := httpScaledObjectClientset.HttpV1alpha1().HTTPScaledObjects(namespace).Get(ctx, name, metav1.GetOptions{})
	hasHTTPTrigger := true
	if err != nil {
		if !errors.IsNotFound(err) {
			return fn.ListItem{}, fmt.Errorf("unable to get HTTPScaledObject: %v", err)
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
		// should be reported instead of silently listed as healthy.
		if _, err := dynClient.Resource(scaledObjectGVR).Namespace(namespace).Get(ctx, scaledObjectName(name), metav1.GetOptions{}); err != nil {
			if errors.IsNotFound(err) {
				return fn.ListItem{}, fmt.Errorf(
					"function %q uses the keda deployer but has neither an HTTPScaledObject nor a Kafka ScaledObject: the scaler may have failed to create or been deleted externally", name)
			}
			return fn.ListItem{}, fmt.Errorf("unable to get ScaledObject: %v", err)
		}
	}

	deployment, err := clientset.AppsV1().Deployments(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return fn.ListItem{}, fmt.Errorf("unable to get deployment: %v", err)
	}
	replicas := int(deployment.Status.ReadyReplicas)

	var ready v1.ConditionStatus
	var url string
	if hasHTTPTrigger {
		ready = v1.ConditionUnknown
		if meta.IsStatusConditionTrue(httpScaledObject.Status.Conditions, v1alpha1.ConditionTypeReady) {
			ready = v1.ConditionTrue
		} else if meta.IsStatusConditionFalse(httpScaledObject.Status.Conditions, v1alpha1.ConditionTypeReady) {
			ready = v1.ConditionFalse
		}
		url, _ = functionURLs(httpScaledObject.Spec.Hosts, exposedHost)
	} else {
		// No HTTP trigger: fall back to the Deployment's own readiness
		// condition and the cluster-local Service URL, same as Describe.
		ready = v1.ConditionUnknown
		for _, cond := range deployment.Status.Conditions {
			if cond.Type == appsv1.DeploymentAvailable {
				ready = cond.Status
				break
			}
		}
		url = fmt.Sprintf("http://%s.%s.svc", name, namespace)
	}

	listItem := fn.ListItem{
		Name:      name,
		Namespace: namespace,
		Runtime:   runtime,
		URL:       url,
		Ready:     string(ready),
		Deployer:  KedaDeployerName,
		Replicas:  replicas,
	}

	return listItem, nil
}
