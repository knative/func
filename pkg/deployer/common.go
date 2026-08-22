package deployer

import (
	"maps"

	fn "knative.dev/func/pkg/functions"
	fnlabels "knative.dev/func/pkg/k8s/labels"
)

const (
	DeployerNameAnnotation = "function.knative.dev/deployer"

	// DomainLabel records the custom domain a function was deployed with.
	// On a Route it is the domain spec.host was built from, which ensure()
	// compares to detect a domain change (host updates are permission-gated,
	// so a change means recreating the Route).
	DomainLabel = "func.domain"

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

// IdentityLabels are the find/ownership stamp every object this deploy
// writes carries: the func marker, the function's name, and its runtime.
func IdentityLabels(name, runtime string) map[string]string {
	return map[string]string{
		fnlabels.FunctionKey:        "true",
		fnlabels.FunctionNameKey:    name,
		fnlabels.FunctionRuntimeKey: runtime,
	}
}

// IdentityAnnotations is the ownership stamp on annotations:
// function.knative.dev/deployer. Empty deployerName yields an empty map.
func IdentityAnnotations(deployerName string) map[string]string {
	if deployerName == "" {
		return map[string]string{}
	}
	return map[string]string{DeployerNameAnnotation: deployerName}
}

// GenerateCommonLabels is identity plus user labels, domain, and decorator.
// Identity keys win over the same keys in func.yaml.
func GenerateCommonLabels(f fn.Function, decorator DeployDecorator) (map[string]string, error) {
	ll, err := f.LabelsMap()
	if err != nil {
		return nil, err
	}
	maps.Copy(ll, IdentityLabels(f.Name, f.Runtime))

	if f.Domain != "" {
		ll[DomainLabel] = f.Domain
	}

	if decorator != nil {
		ll = decorator.UpdateLabels(f, ll)
	}

	return ll, nil
}

// SelectorLabels is the subset of ll that may be a pod selector: identity
// only. Deployment.spec.selector is immutable, so it may only carry values
// fixed for the lifetime of the function. Domain, user labels, and decorator
// labels stay on object metadata and the pod template, where they can change
// on redeploy. Older Deployments that pinned the full map keep that live
// selector via preserveDeploymentSelector.
func SelectorLabels(ll map[string]string) map[string]string {
	sl := make(map[string]string, 3)
	for _, k := range []string{
		fnlabels.FunctionKey,
		fnlabels.FunctionNameKey,
		fnlabels.FunctionRuntimeKey,
	} {
		if v, ok := ll[k]; ok {
			sl[k] = v
		}
	}
	return sl
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

	// Add user-defined annotations
	for k, v := range f.Deploy.Annotations {
		aa[k] = v
	}

	// Identity last among generated keys: the ownership stamp must not be
	// user-assignable. The decorator may still overwrite it (keda).
	maps.Copy(aa, IdentityAnnotations(deployerName))

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
