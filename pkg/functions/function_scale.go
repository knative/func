package functions

import (
	"fmt"
	"math"
)

// KEDA scaler types recorded as observed state (DeploySpec.ScalerType) and
// derived from intent (Function.Scale.KEDA.Triggers).
const (
	ScalerTypeHTTP  = "http"
	ScalerTypeKafka = "kafka"
)

// IntendedScalerType derives the KEDA scaler type a deploy of f would provision:
// "kafka" when a kafka trigger is configured, otherwise "http" (the keda
// default). Returns "" for non-keda deployers, which have no scaler concept.
// The deployer is read from intent (f.Deployer) falling back to the observed
// value (f.Deploy.Deployer) so a redeploy that omits the deployer flag still
// resolves the scaler type the function is actually deployed with.
func IntendedScalerType(f Function) string {
	deployer := f.Deployer
	if deployer == "" {
		deployer = f.Deploy.Deployer
	}
	if deployer != "keda" {
		return ""
	}
	if f.Scale != nil && f.Scale.KEDA != nil {
		for _, t := range f.Scale.KEDA.Triggers {
			if t.Type == ScalerTypeKafka {
				return ScalerTypeKafka
			}
		}
	}
	return ScalerTypeHTTP
}

// ValidateScalerSwitch reports whether switching the KEDA scaler type from the
// currently-deployed "from" to the requested "to" is allowed. Switching in place
// (http <-> kafka) would orphan the old scaler on the Deployment, so it is
// refused; the user must `func delete` first. An empty from (nothing recorded)
// or empty to (deployer has no scaler) is not a switch. Mirrors
// deployers.ValidateSwitch.
func ValidateScalerSwitch(from, to string) error {
	if from == "" || to == "" || from == to {
		return nil
	}
	return fmt.Errorf("function was deployed with the %q scaler; redeploying with %q is not supported. Run func delete first, then redeploy", from, to)
}

// ValidateScale validates the top-level scale configuration against the chosen
// deployer. It handles the deployer-agnostic min/max bounds and the KEDA and KPA
// scaling sub-keys. kafka is the run.kafka config (may be nil) so a kafka trigger
// can be checked against it.
func ValidateScale(scale *ScaleOptions, deployer string, kafka *KafkaConfig) (errors []string) {
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

	// scale.keda and scale.kpa target different deployers and cannot both apply.
	if scale.KEDA != nil && scale.KPA != nil {
		errors = append(errors, "scale.keda and scale.kpa are mutually exclusive")
		return
	}
	// scale.keda requires the keda deployer. Unlike scale.kpa on a non-knative
	// deployer (benign: min/max still apply, ignored with a warning at deploy
	// time), silently dropping a scale.keda block would discard the user's whole
	// scaling intent (e.g. kafka consumer-lag scaling), so this is a hard error.
	if scale.KEDA != nil && deployer != "keda" {
		errors = append(errors, "scale.keda requires deployer: keda")
	}

	if scale.KEDA != nil {
		errors = append(errors, validateKEDAScale(scale.KEDA, kafka)...)
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

// validateKEDAScale validates the scale.keda block: pollingInterval/cooldownPeriod
// bounds and each trigger. kafka is the run.kafka config (may be nil); a kafka
// trigger requires it to be configured.
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
			// The lag fields belong to the kafka scaler; reject them on an http
			// trigger rather than silently ignoring them, so a misplaced field is
			// caught instead of quietly having no effect.
			if t.LagThreshold != nil {
				errors = append(errors, fmt.Sprintf("scale.keda.triggers[%d].lagThreshold is only valid for a kafka trigger", i))
			}
			if t.ActivationLagThreshold != nil {
				errors = append(errors, fmt.Sprintf("scale.keda.triggers[%d].activationLagThreshold is only valid for a kafka trigger", i))
			}
		case "kafka":
			sawKafka = true
			if kafka == nil {
				errors = append(errors, fmt.Sprintf("scale.keda.triggers[%d] has type kafka but run.kafka is not configured", i))
			}
			// targetValue is the http scaler's request-rate target; it has no
			// meaning for a kafka trigger (which uses lagThreshold).
			if t.TargetValue != nil {
				errors = append(errors, fmt.Sprintf("scale.keda.triggers[%d].targetValue is only valid for an http trigger", i))
			}
			if t.LagThreshold != nil && *t.LagThreshold < 1 {
				errors = append(errors, fmt.Sprintf("scale.keda.triggers[%d].lagThreshold must be >= 1", i))
			}
			if t.ActivationLagThreshold != nil && *t.ActivationLagThreshold < 0 {
				errors = append(errors, fmt.Sprintf("scale.keda.triggers[%d].activationLagThreshold must not be negative", i))
			}
		default:
			errors = append(errors, fmt.Sprintf("scale.keda.triggers[%d].type has invalid value %q, allowed: http, kafka", i, t.Type))
		}
	}
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
