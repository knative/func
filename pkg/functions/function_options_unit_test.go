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
			"correct 'scale.metric' - concurrency",
			Options{
				Scale: &ScaleOptions{
					Metric: ptr.String("concurrency"),
				},
			},
			0,
		},
		{
			"correct 'scale.metric' - rps",
			Options{
				Scale: &ScaleOptions{
					Metric: ptr.String("rps"),
				},
			},
			0,
		},
		{
			"incorrect 'scale.metric'",
			Options{
				Scale: &ScaleOptions{
					Metric: ptr.String("foo"),
				},
			},
			1,
		},
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
			"correct  'scale.min' & 'scale.max'",
			Options{
				Scale: &ScaleOptions{
					Min: ptr.Int64(0),
					Max: ptr.Int64(10),
				},
			},
			0,
		},
		{
			"incorrect  'scale.min' & 'scale.max'",
			Options{
				Scale: &ScaleOptions{
					Min: ptr.Int64(100),
					Max: ptr.Int64(10),
				},
			},
			1,
		},
		{
			"incorrect 'scale.min' - negative value",
			Options{
				Scale: &ScaleOptions{
					Min: ptr.Int64(-10),
				},
			},
			1,
		},
		{
			"incorrect 'scale.max' - negative value",
			Options{
				Scale: &ScaleOptions{
					Max: ptr.Int64(-10),
				},
			},
			1,
		},
		{
			"correct 'scale.target'",
			Options{
				Scale: &ScaleOptions{
					Target: ptr.Float64(50),
				},
			},
			0,
		},
		{
			"incorrect 'scale.target'",
			Options{
				Scale: &ScaleOptions{
					Target: ptr.Float64(0),
				},
			},
			1,
		},
		{
			"correct 'scale.utilization'",
			Options{
				Scale: &ScaleOptions{
					Utilization: ptr.Float64(50),
				},
			},
			0,
		},
		{
			"incorrect 'scale.utilization' - < 1",
			Options{
				Scale: &ScaleOptions{
					Utilization: ptr.Float64(0),
				},
			},
			1,
		},
		{
			"incorrect 'scale.utilization' - > 100",
			Options{
				Scale: &ScaleOptions{
					Utilization: ptr.Float64(110),
				},
			},
			1,
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
			"correct all options",
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
				Scale: &ScaleOptions{
					Min:         ptr.Int64(0),
					Max:         ptr.Int64(10),
					Metric:      ptr.String("concurrency"),
					Target:      ptr.Float64(40.5),
					Utilization: ptr.Float64(35.5),
				},
			},
			0,
		},
		{
			"incorrect all options",
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
				Scale: &ScaleOptions{
					Min:         ptr.Int64(-1),
					Max:         ptr.Int64(-1),
					Metric:      ptr.String("foo"),
					Target:      ptr.Float64(-1),
					Utilization: ptr.Float64(110),
				},
			},
			10,
		},
		{
			"valid keda triggers",
			Options{
				Scale: &ScaleOptions{
					KEDA: &KEDAScaleOptions{
						Triggers: []KEDATrigger{
							{Type: "http"},
							{Type: "kafka", LagThreshold: ptr.Int64(10)},
						},
					},
				},
			},
			0,
		},
		{
			"empty keda triggers",
			Options{
				Scale: &ScaleOptions{
					KEDA: &KEDAScaleOptions{},
				},
			},
			1,
		},
		{
			"invalid keda trigger type",
			Options{
				Scale: &ScaleOptions{
					KEDA: &KEDAScaleOptions{
						Triggers: []KEDATrigger{
							{Type: "invalid"},
						},
					},
				},
			},
			1,
		},
		{
			"keda cron trigger missing fields",
			Options{
				Scale: &ScaleOptions{
					KEDA: &KEDAScaleOptions{
						Triggers: []KEDATrigger{
							{Type: "cron"},
						},
					},
				},
			},
			4,
		},
		{
			"valid keda cron trigger",
			Options{
				Scale: &ScaleOptions{
					KEDA: &KEDAScaleOptions{
						Triggers: []KEDATrigger{
							{Type: "cron", Timezone: "UTC", Start: "0 8 * * *", End: "0 20 * * *", DesiredReplicas: ptr.Int64(3)},
						},
					},
				},
			},
			0,
		},
		{
			"keda and kpa mutually exclusive",
			Options{
				Scale: &ScaleOptions{
					KEDA: &KEDAScaleOptions{Triggers: []KEDATrigger{{Type: "http"}}},
					KPA:  &KPAScaleOptions{Metric: ptr.String("concurrency")},
				},
			},
			1,
		},
		{
			"valid kpa options",
			Options{
				Scale: &ScaleOptions{
					KPA: &KPAScaleOptions{
						Metric:      ptr.String("rps"),
						Target:      ptr.Float64(50),
						Utilization: ptr.Float64(80),
					},
				},
			},
			0,
		},
		{
			"invalid kpa metric",
			Options{
				Scale: &ScaleOptions{
					KPA: &KPAScaleOptions{
						Metric: ptr.String("bad"),
					},
				},
			},
			1,
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
