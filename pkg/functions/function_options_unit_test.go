package functions

import (
	"testing"

	"knative.dev/pkg/ptr"
)

func Test_validateOptions(t *testing.T) {

	tests := []struct {
		name    string
		options Options
		errs    int
	}{
		{
			"correct 'resources.requests.cpu'",
			Options{
				Resources: &ResourcesOptions{
					Requests: &ResourcesRequestsOptions{
						CPU: ptr.String("1000m"),
					},
				},
			},
			0,
		},
		{
			"incorrect 'resources.requests.cpu'",
			Options{
				Resources: &ResourcesOptions{
					Requests: &ResourcesRequestsOptions{
						CPU: ptr.String("foo"),
					},
				},
			},
			1,
		},
		{
			"correct 'resources.requests.memory'",
			Options{
				Resources: &ResourcesOptions{
					Requests: &ResourcesRequestsOptions{
						Memory: ptr.String("100Mi"),
					},
				},
			},
			0,
		},
		{
			"incorrect 'resources.requests.memory'",
			Options{
				Resources: &ResourcesOptions{
					Requests: &ResourcesRequestsOptions{
						Memory: ptr.String("foo"),
					},
				},
			},
			1,
		},
		{
			"correct 'resources.limits.cpu'",
			Options{
				Resources: &ResourcesOptions{
					Limits: &ResourcesLimitsOptions{
						CPU: ptr.String("1000m"),
					},
				},
			},
			0,
		},
		{
			"incorrect 'resources.limits.cpu'",
			Options{
				Resources: &ResourcesOptions{
					Limits: &ResourcesLimitsOptions{
						CPU: ptr.String("foo"),
					},
				},
			},
			1,
		},
		{
			"correct 'resources.limits.memory'",
			Options{
				Resources: &ResourcesOptions{
					Limits: &ResourcesLimitsOptions{
						Memory: ptr.String("100Mi"),
					},
				},
			},
			0,
		},
		{
			"incorrect 'resources.limits.memory'",
			Options{
				Resources: &ResourcesOptions{
					Limits: &ResourcesLimitsOptions{
						Memory: ptr.String("foo"),
					},
				},
			},
			1,
		},
		{
			"correct 'resources.limits.concurrency'",
			Options{
				Resources: &ResourcesOptions{
					Limits: &ResourcesLimitsOptions{
						Concurrency: ptr.Int64(50),
					},
				},
			},
			0,
		},
		{
			"correct 'resources.limits.concurrency' - 0",
			Options{
				Resources: &ResourcesOptions{
					Limits: &ResourcesLimitsOptions{
						Concurrency: ptr.Int64(0),
					},
				},
			},
			0,
		},
		{
			"incorrect 'resources.limits.concurrency' - negative value",
			Options{
				Resources: &ResourcesOptions{
					Limits: &ResourcesLimitsOptions{
						Concurrency: ptr.Int64(-10),
					},
				},
			},
			1,
		},
		{
			"correct all resource options",
			Options{
				Resources: &ResourcesOptions{
					Requests: &ResourcesRequestsOptions{
						CPU:    ptr.String("1000m"),
						Memory: ptr.String("100Mi"),
					},
					Limits: &ResourcesLimitsOptions{
						CPU:         ptr.String("1000m"),
						Memory:      ptr.String("100Mi"),
						Concurrency: ptr.Int64(10),
					},
				},
			},
			0,
		},
		{
			"incorrect all resource options",
			Options{
				Resources: &ResourcesOptions{
					Requests: &ResourcesRequestsOptions{
						CPU:    ptr.String("foo"),
						Memory: ptr.String("foo"),
					},
					Limits: &ResourcesLimitsOptions{
						CPU:         ptr.String("foo"),
						Memory:      ptr.String("foo"),
						Concurrency: ptr.Int64(-1),
					},
				},
			},
			5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := validateOptions(tt.options); len(got) != tt.errs {
				t.Errorf("validateOptions() = %v\n got %d errors but want %d", got, len(got), tt.errs)
			}
		})
	}
}

func Test_ValidateScale(t *testing.T) {
	tests := []struct {
		name     string
		scale    *ScaleOptions
		deployer string
		errs     int
	}{
		{
			"nil scale is valid",
			nil, "", 0,
		},
		{
			"correct min",
			&ScaleOptions{Min: ptr.Int64(1)},
			"", 0,
		},
		{
			"correct max",
			&ScaleOptions{Max: ptr.Int64(10)},
			"", 0,
		},
		{
			"correct min & max",
			&ScaleOptions{Min: ptr.Int64(0), Max: ptr.Int64(10)},
			"", 0,
		},
		{
			"incorrect min & max",
			&ScaleOptions{Min: ptr.Int64(100), Max: ptr.Int64(10)},
			"", 1,
		},
		{
			"negative min",
			&ScaleOptions{Min: ptr.Int64(-10)},
			"", 1,
		},
		{
			"negative max",
			&ScaleOptions{Max: ptr.Int64(-10)},
			"", 1,
		},
		{
			// 2147483648 == math.MaxInt32 + 1: does not fit the int32 the
			// deployers narrow replica counts to (would wrap to 0).
			"min above int32 max",
			&ScaleOptions{Min: ptr.Int64(2147483648), Max: ptr.Int64(2147483648)},
			"", 2, // one for min, one for max
		},
		{
			"max above int32 max",
			&ScaleOptions{Max: ptr.Int64(2147483648)},
			"", 1,
		},
		{
			"valid kpa options",
			&ScaleOptions{
				KPA: &KPAScaleOptions{
					Metric:      ptr.String("rps"),
					Target:      ptr.Float64(50),
					Utilization: ptr.Float64(80),
				},
			},
			"knative", 0,
		},
		{
			"invalid kpa metric",
			&ScaleOptions{
				KPA: &KPAScaleOptions{Metric: ptr.String("bad")},
			},
			"knative", 1,
		},
		{
			"kpa target too low",
			&ScaleOptions{
				KPA: &KPAScaleOptions{Target: ptr.Float64(0)},
			},
			"knative", 1,
		},
		{
			"kpa utilization out of range",
			&ScaleOptions{
				KPA: &KPAScaleOptions{Utilization: ptr.Float64(110)},
			},
			"knative", 1,
		},
		{
			// scale.kpa on a non-knative deployer is no longer a validation
			// error -- it is ignored with a warning at deploy time. The kpa
			// values are still validated, so a valid metric passes here.
			"kpa on non-knative deployer is accepted (ignored with warning)",
			&ScaleOptions{
				KPA: &KPAScaleOptions{Metric: ptr.String("concurrency")},
			},
			"raw", 0,
		},
		{
			// ...but the kpa values are still validated regardless of deployer.
			"kpa with invalid metric is rejected even on non-knative deployer",
			&ScaleOptions{
				KPA: &KPAScaleOptions{Metric: ptr.String("bad")},
			},
			"raw", 1,
		},
		{
			"keda max 0 is invalid: not a valid HPA maxReplicas",
			&ScaleOptions{Max: ptr.Int64(0)},
			"keda", 1,
		},
		{
			"knative max 0 means no limit, still valid",
			&ScaleOptions{
				Max: ptr.Int64(0),
				KPA: &KPAScaleOptions{Metric: ptr.String("concurrency")},
			},
			"knative", 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ValidateScale(tt.scale, tt.deployer, nil); len(got) != tt.errs {
				t.Errorf("ValidateScale() = %v\n got %d errors but want %d", got, len(got), tt.errs)
			}
		})
	}
}

// Test_ValidateScale_KEDA covers the scale.keda surface: the sub-key gating
// (keda requires deployer keda; keda and kpa may coexist, the irrelevant one
// is ignored) and the per-trigger validation, including a kafka trigger's
// dependency on run.kafka.
func Test_ValidateScale_KEDA(t *testing.T) {
	kafkaRun := &KafkaConfig{Brokers: "b:9092", Topic: "t", ConsumerGroup: "g"}
	httpTrigger := []KEDATrigger{{Type: "http"}}

	tests := []struct {
		name     string
		scale    *ScaleOptions
		deployer string
		kafka    *KafkaConfig
		errs     int
	}{
		{
			"valid http trigger",
			&ScaleOptions{KEDA: &KEDAScaleOptions{Triggers: httpTrigger}},
			"keda", nil, 0,
		},
		{
			"valid kafka trigger with run.kafka",
			&ScaleOptions{KEDA: &KEDAScaleOptions{Triggers: []KEDATrigger{{Type: "kafka"}}}},
			"keda", kafkaRun, 0,
		},
		{
			"kafka trigger without run.kafka",
			&ScaleOptions{KEDA: &KEDAScaleOptions{Triggers: []KEDATrigger{{Type: "kafka"}}}},
			"keda", nil, 1,
		},
		{
			"keda requires deployer keda",
			&ScaleOptions{KEDA: &KEDAScaleOptions{Triggers: httpTrigger}},
			"knative", nil, 1,
		},
		{
			"keda and kpa coexist on keda deployer",
			&ScaleOptions{
				KEDA: &KEDAScaleOptions{Triggers: httpTrigger},
				KPA:  &KPAScaleOptions{Metric: ptr.String("concurrency")},
			},
			"keda", nil, 0,
		},
		{
			"empty triggers is invalid",
			&ScaleOptions{KEDA: &KEDAScaleOptions{}},
			"keda", nil, 1,
		},
		{
			"pollingInterval below minimum",
			&ScaleOptions{KEDA: &KEDAScaleOptions{PollingInterval: ptr.Int32(0), Triggers: httpTrigger}},
			"keda", nil, 1,
		},
		{
			"cooldownPeriod below minimum",
			&ScaleOptions{KEDA: &KEDAScaleOptions{CooldownPeriod: ptr.Int32(0), Triggers: httpTrigger}},
			"keda", nil, 1,
		},
		{
			"http targetValue below minimum",
			&ScaleOptions{KEDA: &KEDAScaleOptions{Triggers: []KEDATrigger{{Type: "http", TargetValue: ptr.Int64(0)}}}},
			"keda", nil, 1,
		},
		{
			"kafka lagThreshold below minimum",
			&ScaleOptions{KEDA: &KEDAScaleOptions{Triggers: []KEDATrigger{{Type: "kafka", LagThreshold: ptr.Int64(0)}}}},
			"keda", kafkaRun, 1,
		},
		{
			"kafka negative activationLagThreshold",
			&ScaleOptions{KEDA: &KEDAScaleOptions{Triggers: []KEDATrigger{{Type: "kafka", ActivationLagThreshold: ptr.Int64(-1)}}}},
			"keda", kafkaRun, 1,
		},
		{
			"repeated http trigger",
			&ScaleOptions{KEDA: &KEDAScaleOptions{Triggers: []KEDATrigger{{Type: "http"}, {Type: "http"}}}},
			"keda", nil, 1,
		},
		{
			"http and kafka cannot be combined",
			&ScaleOptions{KEDA: &KEDAScaleOptions{Triggers: []KEDATrigger{{Type: "http"}, {Type: "kafka"}}}},
			"keda", kafkaRun, 1,
		},
		{
			// cron was dropped from the surface; it is now just an unknown type.
			"cron is no longer a valid type",
			&ScaleOptions{KEDA: &KEDAScaleOptions{Triggers: []KEDATrigger{{Type: "cron"}}}},
			"keda", nil, 1,
		},
		{
			"unknown trigger type",
			&ScaleOptions{KEDA: &KEDAScaleOptions{Triggers: []KEDATrigger{{Type: "bogus"}}}},
			"keda", nil, 1,
		},
		{
			// lagThreshold on an http trigger is a cross-field error.
			"lagThreshold rejected on http trigger",
			&ScaleOptions{KEDA: &KEDAScaleOptions{Triggers: []KEDATrigger{{Type: "http", LagThreshold: ptr.Int64(10)}}}},
			"keda", nil, 1,
		},
		{
			// targetValue on a kafka trigger is a cross-field error.
			"targetValue rejected on kafka trigger",
			&ScaleOptions{KEDA: &KEDAScaleOptions{Triggers: []KEDATrigger{{Type: "kafka", TargetValue: ptr.Int64(10)}}}},
			"keda", kafkaRun, 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ValidateScale(tt.scale, tt.deployer, tt.kafka); len(got) != tt.errs {
				t.Errorf("ValidateScale() = %v\n got %d errors but want %d", got, len(got), tt.errs)
			}
		})
	}
}

// Test_ValidateScalerSwitch is the truth table for the refuse gate: only a
// change between two distinct, non-empty scaler types is refused; an empty
// from/to (nothing recorded / no scaler concept) or an unchanged type is
// allowed.
func Test_ValidateScalerSwitch(t *testing.T) {
	tests := []struct {
		name       string
		from, to   string
		wantRefuse bool
	}{
		{"nothing recorded, deploying http", "", "http", false},
		{"nothing recorded, deploying kafka", "", "kafka", false},
		{"http to empty (non-keda redeploy)", "http", "", false},
		{"http unchanged", "http", "http", false},
		{"kafka unchanged", "kafka", "kafka", false},
		{"http to kafka is refused", "http", "kafka", true},
		{"kafka to http is refused", "kafka", "http", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateScalerSwitch(tt.from, tt.to)
			if tt.wantRefuse && err == nil {
				t.Errorf("ValidateScalerSwitch(%q, %q) = nil, want an error", tt.from, tt.to)
			}
			if !tt.wantRefuse && err != nil {
				t.Errorf("ValidateScalerSwitch(%q, %q) = %v, want nil", tt.from, tt.to, err)
			}
		})
	}
}

// TestIntendedScalerType covers deriving the scaler type a deploy would
// provision: only meaningful for the keda deployer, kafka when a kafka trigger
// is present, http otherwise (the keda default). The deployer is read from
// intent falling back to observed state so a redeploy that omits the flag still
// resolves correctly.
func TestIntendedScalerType(t *testing.T) {
	tests := []struct {
		name string
		f    Function
		want string
	}{
		{"non-keda deployer has no scaler", Function{Deployer: "knative"}, ""},
		{"empty deployer has no scaler", Function{}, ""},
		{"keda with no scale defaults to http", Function{Deployer: "keda"}, "http"},
		{
			"keda with nil KEDA defaults to http",
			Function{Deployer: "keda", Scale: &ScaleOptions{}},
			"http",
		},
		{
			"keda with an http trigger is http",
			Function{Deployer: "keda", Scale: &ScaleOptions{KEDA: &KEDAScaleOptions{Triggers: []KEDATrigger{{Type: "http"}}}}},
			"http",
		},
		{
			"keda with a kafka trigger is kafka",
			Function{Deployer: "keda", Scale: &ScaleOptions{KEDA: &KEDAScaleOptions{Triggers: []KEDATrigger{{Type: "kafka"}}}}},
			"kafka",
		},
		{
			"deployer falls back to observed state when intent is empty",
			Function{Deploy: DeploySpec{Deployer: "keda"}, Scale: &ScaleOptions{KEDA: &KEDAScaleOptions{Triggers: []KEDATrigger{{Type: "kafka"}}}}},
			"kafka",
		},
		{
			"intent deployer wins over observed state",
			Function{Deployer: "knative", Deploy: DeploySpec{Deployer: "keda"}},
			"",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IntendedScalerType(tt.f); got != tt.want {
				t.Errorf("IntendedScalerType() = %q, want %q", got, tt.want)
			}
		})
	}
}
