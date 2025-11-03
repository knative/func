package deployer

import (
	"context"
	"fmt"
	"os"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clienteventingv1 "knative.dev/client/pkg/eventing/v1"
	eventingv1 "knative.dev/eventing/pkg/apis/eventing/v1"
	duckv1 "knative.dev/pkg/apis/duck/v1"
	"knative.dev/pkg/kmeta"

	fn "knative.dev/func/pkg/functions"
)

const (
	DeployTypeAnnotation = "function.knative.dev/deploy-type"

	// Dapr constants
	DaprEnabled          = "true"
	DaprMetricsPort      = "9092"
	DaprEnableAPILogging = "true"
)

// DeployDecorator is an interface for customizing deployment metadata
type DeployDecorator interface {
	UpdateAnnotations(fn.Function, map[string]string) map[string]string
	UpdateLabels(fn.Function, map[string]string) map[string]string
}

// GenerateCommonLabels creates labels common to both Knative and K8s deployments
func GenerateCommonLabels(f fn.Function, decorator DeployDecorator) (map[string]string, error) {
	ll, err := f.LabelsMap()
	if err != nil {
		return nil, err
	}

	// Standard function labels
	ll["boson.dev/function"] = "true"
	ll["function.knative.dev/name"] = f.Name
	ll["function.knative.dev/runtime"] = f.Runtime

	if f.Domain != "" {
		ll["func.domain"] = f.Domain
	}

	if decorator != nil {
		ll = decorator.UpdateLabels(f, ll)
	}

	return ll, nil
}

// GenerateCommonAnnotations creates annotations common to both Knative and K8s deployments
func GenerateCommonAnnotations(f fn.Function, decorator DeployDecorator, daprInstalled bool, deployType string) map[string]string {
	aa := make(map[string]string)

	// Add Dapr annotations if Dapr is installed
	if daprInstalled {
		for k, v := range GenerateDaprAnnotations(f.Name) {
			aa[k] = v
		}
	}

	if len(deployType) > 0 {
		aa[DeployTypeAnnotation] = deployType
	}

	// Add user-defined annotations
	for k, v := range f.Deploy.Annotations {
		aa[k] = v
	}

	// Apply decorator
	if decorator != nil {
		aa = decorator.UpdateAnnotations(f, aa)
	}

	return aa
}

// GenerateDaprAnnotations generates annotations for Dapr support
// These annotations, if included and Dapr control plane is installed in
// the target cluster, will result in a sidecar exposing the Dapr HTTP API
// on localhost:3500 and metrics on 9092
func GenerateDaprAnnotations(appID string) map[string]string {
	aa := make(map[string]string)
	aa["dapr.io/app-id"] = appID
	aa["dapr.io/enabled"] = DaprEnabled
	aa["dapr.io/metrics-port"] = DaprMetricsPort
	aa["dapr.io/app-port"] = "8080"
	aa["dapr.io/enable-api-logging"] = DaprEnableAPILogging
	return aa
}

func CreateTriggers(ctx context.Context, f fn.Function, obj kmeta.Accessor, eventingClient clienteventingv1.KnEventingClient) error {
	fmt.Fprintf(os.Stderr, "🎯 Creating Triggers on the cluster\n")

	for i, sub := range f.Deploy.Subscriptions {
		// create the filter:
		attributes := make(map[string]string)
		for key, value := range sub.Filters {
			attributes[key] = value
		}

		err := eventingClient.CreateTrigger(ctx, &eventingv1.Trigger{
			ObjectMeta: metav1.ObjectMeta{
				Name: fmt.Sprintf("%s-function-trigger-%d", obj.GetName(), i),
				OwnerReferences: []metav1.OwnerReference{
					{
						APIVersion: obj.GroupVersionKind().Version,
						Kind:       obj.GroupVersionKind().Kind,
						Name:       obj.GetName(),
						UID:        obj.GetUID(),
					},
				},
			},
			Spec: eventingv1.TriggerSpec{
				Broker: sub.Source,

				Subscriber: duckv1.Destination{
					Ref: &duckv1.KReference{
						APIVersion: obj.GroupVersionKind().Version,
						Kind:       obj.GroupVersionKind().Kind,
						Name:       obj.GetName(),
					}},

				Filter: &eventingv1.TriggerFilter{
					Attributes: attributes,
				},
			},
		})
		if err != nil && !errors.IsAlreadyExists(err) {
			err = fmt.Errorf("knative deployer failed to create the Trigger: %v", err)
			return err
		}
	}
	return nil
}
