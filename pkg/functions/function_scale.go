package functions

import (
	"fmt"
	"math"
)

// ValidateScale validates the top-level scale configuration against the chosen
// deployer and Kafka config. It replaces the previous validateScaleDeployer,
// validateKEDAScale, and validateKPAScale functions with a single entry point.
func ValidateScale(scale *ScaleOptions, deployer string, kafka *KafkaConfig) (errors []string) {
	// A nil scale.keda (or no scale at all) is valid for deployer: keda: it
	// means "use the default http scaler" and the keda deployer supplies a
	// plain http trigger (see triggers() in the keda package). Only an
	// explicitly-written scale.keda with an empty triggers list is an error,
	// which validateKEDAScale rejects below.
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
	// int32 and both deployers narrow to it (see replicaBounds in the keda
	// deployer). Reject anything that would not survive that narrowing before
	// it silently wraps -- e.g. 1<<32 casts to int32(0).
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

	if scale.KEDA != nil && scale.KPA != nil {
		errors = append(errors, "scale.keda and scale.kpa are mutually exclusive")
		return
	}
	if scale.KEDA != nil && deployer != "keda" {
		errors = append(errors, "scale.keda requires deployer: keda")
	}
	if scale.KPA != nil && deployer != "knative" && deployer != "" {
		errors = append(errors, "scale.kpa requires deployer: knative")
	}

	if scale.KEDA != nil {
		errors = append(errors, validateKEDAScale(scale.KEDA, kafka)...)
	}
	if scale.KPA != nil {
		errors = append(errors, validateKPAScale(scale.KPA)...)
	}

	return
}

func validateKEDAScale(keda *KEDAScaleOptions, kafka *KafkaConfig) (errors []string) {
	if len(keda.Triggers) == 0 {
		errors = append(errors, "scale.keda.triggers must not be empty when scale.keda is set")
		return
	}

	if keda.PollingInterval != nil && *keda.PollingInterval < 1 {
		errors = append(errors, "scale.keda.pollingInterval must be >= 1")
	}
	if keda.CooldownPeriod != nil && *keda.CooldownPeriod < 1 {
		errors = append(errors, "scale.keda.cooldownPeriod must be >= 1")
	}

	var sawHTTP, sawKafka bool
	seenTypes := map[string]bool{}
	for i, t := range keda.Triggers {
		// The deployer only ever materializes one HTTPScaledObject and one
		// Kafka ScaledObject regardless of how many triggers of that type
		// are configured -- kafkaTrigger() and the HTTP targetValue lookup
		// both take just the first match. A second trigger of the same type
		// would silently have its settings ignored, so reject it here
		// instead.
		if seenTypes[t.Type] && (t.Type == "http" || t.Type == "kafka") {
			errors = append(errors, fmt.Sprintf("scale.keda.triggers[%d].type %q is repeated: only one trigger of each type is supported", i, t.Type))
		}
		seenTypes[t.Type] = true

		switch t.Type {
		case "http":
			sawHTTP = true
			if t.TargetValue != nil && *t.TargetValue < 1 {
				errors = append(errors, fmt.Sprintf("scale.keda.triggers[%d].targetValue must be >= 1", i))
			}
		case "kafka":
			sawKafka = true
			if kafka == nil {
				errors = append(errors, fmt.Sprintf("scale.keda.triggers[%d] has type kafka but run.kafka is not configured", i))
			}
			if t.LagThreshold != nil && *t.LagThreshold < 1 {
				errors = append(errors, fmt.Sprintf("scale.keda.triggers[%d].lagThreshold must be >= 1", i))
			}
			if t.ActivationLagThreshold != nil && *t.ActivationLagThreshold < 0 {
				errors = append(errors, fmt.Sprintf("scale.keda.triggers[%d].activationLagThreshold must not be negative", i))
			}
		case "cron":
			// "cron" is a valid value in func.yaml's schema, reserved for a
			// future deployer implementation, but the keda deployer does not
			// implement it yet: accepting it here would deploy successfully
			// and silently create no scaler at all.
			errors = append(errors, fmt.Sprintf("scale.keda.triggers[%d].type cron is not yet supported", i))
		default:
			errors = append(errors, fmt.Sprintf("scale.keda.triggers[%d].type has invalid value %q, allowed: http, kafka", i, t.Type))
		}
	}

	// The keda deployer creates a separate HTTPScaledObject for "http" and a
	// separate ScaledObject for "kafka", both targeting the same Deployment.
	// KEDA only allows one scaler per workload, so combining them is
	// rejected up front instead of failing later, mid-deploy, against the
	// Kubernetes API.
	if sawHTTP && sawKafka {
		errors = append(errors, "scale.keda.triggers must not combine type http with type kafka: they cannot scale the same Deployment together, not yet supported")
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
