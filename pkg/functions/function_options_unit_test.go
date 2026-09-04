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
			"correct 'scale.min'",
			Options{
				Scale: &ScaleOptions{
					Min: ptr.Int64(1),
				},
			},
			0,
		},
		{
			"correct 'scale.max'",
			Options{
				Scale: &ScaleOptions{
					Max: ptr.Int64(10),
				},
			},
			0,
		},
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
		kafka    *KafkaConfig
		errs     int
	}{
		{
			"nil scale is valid",
			nil, "", nil, 0,
		},
		{
			"correct min",
			&ScaleOptions{Min: ptr.Int64(1)},
			"", nil, 0,
		},
		{
			"correct max",
			&ScaleOptions{Max: ptr.Int64(10)},
			"", nil, 0,
		},
		{
			"correct min & max",
			&ScaleOptions{Min: ptr.Int64(0), Max: ptr.Int64(10)},
			"", nil, 0,
		},
		{
			"incorrect min & max",
			&ScaleOptions{Min: ptr.Int64(100), Max: ptr.Int64(10)},
			"", nil, 1,
		},
		{
			"negative min",
			&ScaleOptions{Min: ptr.Int64(-10)},
			"", nil, 1,
		},
		{
			"negative max",
			&ScaleOptions{Max: ptr.Int64(-10)},
			"", nil, 1,
		},
		{
			// 2147483648 == math.MaxInt32 + 1: does not fit the int32 both
			// deployers narrow replica counts to (would wrap to 0).
			"min above int32 max",
			&ScaleOptions{Min: ptr.Int64(2147483648), Max: ptr.Int64(2147483648)},
			"", nil, 2, // one for min, one for max
		},
		{
			"max above int32 max",
			&ScaleOptions{Max: ptr.Int64(2147483648)},
			"", nil, 1,
		},
		{
			"keda and kpa mutually exclusive",
			&ScaleOptions{
				KEDA: &KEDAScaleOptions{Triggers: []KEDATrigger{{Type: "http"}}},
				KPA:  &KPAScaleOptions{Metric: ptr.String("concurrency")},
			},
			"keda", nil, 1,
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
			"knative", nil, 0,
		},
		{
			"invalid kpa metric",
			&ScaleOptions{
				KPA: &KPAScaleOptions{Metric: ptr.String("bad")},
			},
			"knative", nil, 1,
		},
		{
			"kpa target too low",
			&ScaleOptions{
				KPA: &KPAScaleOptions{Target: ptr.Float64(0)},
			},
			"knative", nil, 1,
		},
		{
			"kpa utilization out of range",
			&ScaleOptions{
				KPA: &KPAScaleOptions{Utilization: ptr.Float64(110)},
			},
			"knative", nil, 1,
		},
		{
			"kpa requires knative deployer",
			&ScaleOptions{
				KPA: &KPAScaleOptions{Metric: ptr.String("concurrency")},
			},
			"raw", nil, 1,
		},
		{
			"valid keda http trigger",
			&ScaleOptions{
				KEDA: &KEDAScaleOptions{
					Triggers: []KEDATrigger{
						{Type: "http"},
					},
				},
			},
			"keda", nil, 0,
		},
		{
			"valid keda kafka trigger",
			&ScaleOptions{
				KEDA: &KEDAScaleOptions{
					Triggers: []KEDATrigger{
						{Type: "kafka", LagThreshold: ptr.Int64(10)},
					},
				},
			},
			"keda", &KafkaConfig{Brokers: "b", Topic: "t", ConsumerGroup: "g"}, 0,
		},
		{
			"keda http and kafka triggers combined is not yet supported",
			&ScaleOptions{
				KEDA: &KEDAScaleOptions{
					Triggers: []KEDATrigger{
						{Type: "http"},
						{Type: "kafka", LagThreshold: ptr.Int64(10)},
					},
				},
			},
			"keda", &KafkaConfig{Brokers: "b", Topic: "t", ConsumerGroup: "g"}, 1,
		},
		{
			// An explicitly-written scale.keda with no triggers is an error:
			// the user opted into scale.keda but declared nothing. (A nil
			// scale.keda is valid and defaults to the http scaler -- see the
			// "keda deployer with nil scale" case below.)
			"empty keda triggers",
			&ScaleOptions{
				KEDA: &KEDAScaleOptions{},
			},
			"keda", nil, 1,
		},
		{
			"invalid keda trigger type",
			&ScaleOptions{
				KEDA: &KEDAScaleOptions{
					Triggers: []KEDATrigger{{Type: "invalid"}},
				},
			},
			"keda", nil, 1,
		},
		{
			"keda cron trigger is not yet supported",
			&ScaleOptions{
				KEDA: &KEDAScaleOptions{
					Triggers: []KEDATrigger{{Type: "cron"}},
				},
			},
			"keda", nil, 1,
		},
		{
			"fully specified keda cron trigger is still not yet supported",
			&ScaleOptions{
				KEDA: &KEDAScaleOptions{
					Triggers: []KEDATrigger{
						{Type: "cron", Timezone: "UTC", Start: "0 8 * * *", End: "0 20 * * *", DesiredReplicas: ptr.Int64(3)},
					},
				},
			},
			"keda", nil, 1,
		},
		{
			"keda requires deployer keda",
			&ScaleOptions{
				KEDA: &KEDAScaleOptions{Triggers: []KEDATrigger{{Type: "http"}}},
			},
			"knative", nil, 1,
		},
		{
			// A nil scale.keda is valid for keda: it means "use the default
			// http scaler" (the deployer supplies a plain http trigger).
			"keda deployer with nil scale is valid (defaults to http)",
			nil, "keda", nil, 0,
		},
		{
			"kafka trigger without kafka config",
			&ScaleOptions{
				KEDA: &KEDAScaleOptions{
					Triggers: []KEDATrigger{{Type: "kafka"}},
				},
			},
			"keda", nil, 1,
		},
		{
			"keda pollingInterval too low",
			&ScaleOptions{
				KEDA: &KEDAScaleOptions{
					PollingInterval: ptr.Int32(0),
					Triggers:        []KEDATrigger{{Type: "http"}},
				},
			},
			"keda", nil, 1,
		},
		{
			"keda cooldownPeriod too low",
			&ScaleOptions{
				KEDA: &KEDAScaleOptions{
					CooldownPeriod: ptr.Int32(0),
					Triggers:       []KEDATrigger{{Type: "http"}},
				},
			},
			"keda", nil, 1,
		},
		{
			"http targetValue too low",
			&ScaleOptions{
				KEDA: &KEDAScaleOptions{
					Triggers: []KEDATrigger{{Type: "http", TargetValue: ptr.Int64(0)}},
				},
			},
			"keda", nil, 1,
		},
		{
			"keda max 0 is invalid: not a valid HPA maxReplicas",
			&ScaleOptions{
				Max:  ptr.Int64(0),
				KEDA: &KEDAScaleOptions{Triggers: []KEDATrigger{{Type: "http"}}},
			},
			"keda", nil, 1,
		},
		{
			"knative max 0 means no limit, still valid",
			&ScaleOptions{
				Max: ptr.Int64(0),
				KPA: &KPAScaleOptions{Metric: ptr.String("concurrency")},
			},
			"knative", nil, 0,
		},
		{
			"duplicate http triggers rejected",
			&ScaleOptions{
				KEDA: &KEDAScaleOptions{
					Triggers: []KEDATrigger{
						{Type: "http", TargetValue: ptr.Int64(100)},
						{Type: "http", TargetValue: ptr.Int64(200)},
					},
				},
			},
			"keda", nil, 1,
		},
		{
			"duplicate kafka triggers rejected",
			&ScaleOptions{
				KEDA: &KEDAScaleOptions{
					Triggers: []KEDATrigger{
						{Type: "kafka", LagThreshold: ptr.Int64(5)},
						{Type: "kafka", LagThreshold: ptr.Int64(10)},
					},
				},
			},
			"keda", &KafkaConfig{Brokers: "b", Topic: "t", ConsumerGroup: "g"}, 1,
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
