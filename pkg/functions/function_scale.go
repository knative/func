package functions

import (
	"fmt"
	"math"
)

// ValidateScale validates the top-level scale configuration against the chosen
// deployer. It handles the deployer-agnostic min/max bounds and the KPA
// (knative) scaling sub-key.
func ValidateScale(scale *ScaleOptions, deployer string) (errors []string) {
	if scale == nil {
		return
	}

	if scale.Min != nil && *scale.Min < 0 {
		errors = append(errors, fmt.Sprintf("scale.min has invalid value: %d, must be >= 0", *scale.Min))
	}
	if scale.Max != nil && *scale.Max < 0 {
		errors = append(errors, fmt.Sprintf("scale.max has invalid value: %d, must be >= 0", *scale.Max))
	}
	// scale.min/max are int64 in func.yaml, but Kubernetes replica counts are
	// int32 and every deployer narrows to it (see replicaBounds in the keda
	// deployer and generateDeployment in the raw deployer). Reject anything that
	// would not survive that narrowing before it silently wraps -- e.g. 1<<32
	// casts to int32(0).
	if scale.Min != nil && *scale.Min > math.MaxInt32 {
		errors = append(errors, fmt.Sprintf("scale.min has invalid value: %d, must be <= %d", *scale.Min, math.MaxInt32))
	}
	if scale.Max != nil && *scale.Max > math.MaxInt32 {
		errors = append(errors, fmt.Sprintf("scale.max has invalid value: %d, must be <= %d", *scale.Max, math.MaxInt32))
	}
	if scale.Min != nil && scale.Max != nil && *scale.Max < *scale.Min {
		errors = append(errors, "scale.max must be >= scale.min")
	}
	if deployer == "keda" && scale.Max != nil && *scale.Max == 0 {
		// 0 means "no limit" for the knative/kpa deployer, but keda's
		// HTTPScaledObject/ScaledObject map it straight to the HPA's
		// maxReplicas, which must be >= 1. Leave scale.max unset to get
		// keda's own default instead.
		errors = append(errors, "scale.max must be >= 1 when deployer is keda: 0 (\"no limit\") is not a valid value, leave scale.max unset to use keda's default")
	}

	// scale.kpa is only consumed by the knative deployer (setServiceOptions);
	// raw and keda ignore it. That mismatch is not a validation error -- it is
	// surfaced as an ignored-with-warning case at deploy time (see
	// warnScaleKpaIgnore in cmd/deploy.go), mirroring how expose is handled for
	// deployers that ignore it. The values themselves are still validated so a
	// bad metric/target/utilization is caught regardless of deployer.
	if scale.KPA != nil {
		errors = append(errors, validateKPAScale(scale.KPA)...)
	}

	return
}

func validateKPAScale(kpa *KPAScaleOptions) (errors []string) {
	if kpa.Metric != nil && *kpa.Metric != "concurrency" && *kpa.Metric != "rps" {
		errors = append(errors, fmt.Sprintf("scale.kpa.metric has invalid value: %s, allowed: concurrency, rps", *kpa.Metric))
	}
	if kpa.Target != nil && *kpa.Target < 0.01 {
		errors = append(errors, fmt.Sprintf("scale.kpa.target must be >= 0.01, got %f", *kpa.Target))
	}
	if kpa.Utilization != nil && (*kpa.Utilization < 1 || *kpa.Utilization > 100) {
		errors = append(errors, fmt.Sprintf("scale.kpa.utilization must be 1-100, got %f", *kpa.Utilization))
	}
	return
}
