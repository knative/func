package deployer

import (
	fn "knative.dev/func/pkg/functions"
)

const (
	DeployerNameAnnotation = "function.knative.dev/deployer"

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
func GenerateCommonAnnotations(f fn.Function, decorator DeployDecorator, daprInstalled bool, deployerName string) map[string]string {
	aa := make(map[string]string)

	// Add Dapr annotations if Dapr is installed
	if daprInstalled {
		for k, v := range GenerateDaprAnnotations(f.Name) {
			aa[k] = v
		}
	}

	if len(deployerName) > 0 {
		aa[DeployerNameAnnotation] = deployerName
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
