package functions

import (
	"fmt"

	"k8s.io/apimachinery/pkg/api/resource"
)

type Options struct {
	Scale     *ScaleOptions     `yaml:"scale,omitempty"`
	Resources *ResourcesOptions `yaml:"resources,omitempty"`
}

type ScaleOptions struct {
	Min         *int64            `yaml:"min,omitempty" jsonschema_extras:"minimum=0"`
	Max         *int64            `yaml:"max,omitempty" jsonschema_extras:"minimum=0"`
	Metric      *string           `yaml:"metric,omitempty" jsonschema:"enum=concurrency,enum=rps"`
	Target      *float64          `yaml:"target,omitempty" jsonschema_extras:"minimum=0.01"`
	Utilization *float64          `yaml:"utilization,omitempty" jsonschema:"minimum=1,maximum=100"`
	KEDA        *KEDAScaleOptions `yaml:"keda,omitempty"`
	KPA         *KPAScaleOptions  `yaml:"kpa,omitempty"`
}

type KEDAScaleOptions struct {
	Triggers []KEDATrigger `yaml:"triggers,omitempty"`
}

type KEDATrigger struct {
	Type                   string `yaml:"type" jsonschema:"enum=http,enum=kafka,enum=cron"`
	LagThreshold           *int64 `yaml:"lagThreshold,omitempty" jsonschema_extras:"minimum=1"`
	ActivationLagThreshold *int64 `yaml:"activationLagThreshold,omitempty" jsonschema_extras:"minimum=0"`
	Timezone               string `yaml:"timezone,omitempty"`
	Start                  string `yaml:"start,omitempty"`
	End                    string `yaml:"end,omitempty"`
	DesiredReplicas        *int64 `yaml:"desiredReplicas,omitempty" jsonschema_extras:"minimum=1"`
}

type KPAScaleOptions struct {
	Metric      *string  `yaml:"metric,omitempty" jsonschema:"enum=concurrency,enum=rps"`
	Target      *float64 `yaml:"target,omitempty" jsonschema_extras:"minimum=0.01"`
	Utilization *float64 `yaml:"utilization,omitempty" jsonschema:"minimum=1,maximum=100"`
}

type ResourcesOptions struct {
	Requests *ResourcesRequestsOptions `yaml:"requests,omitempty"`
	Limits   *ResourcesLimitsOptions   `yaml:"limits,omitempty"`
}

type ResourcesLimitsOptions struct {
	CPU         *string `yaml:"cpu,omitempty" jsonschema:"pattern=^([+-]?[0-9.]+)([eEinumkKMGTP]*[-+]?[0-9]*)$"`
	Memory      *string `yaml:"memory,omitempty" jsonschema:"pattern=^([+-]?[0-9.]+)([eEinumkKMGTP]*[-+]?[0-9]*)$"`
	Concurrency *int64  `yaml:"concurrency,omitempty" jsonschema_extras:"minimum=0"`
}

type ResourcesRequestsOptions struct {
	CPU    *string `yaml:"cpu,omitempty" jsonschema:"pattern=^([+-]?[0-9.]+)([eEinumkKMGTP]*[-+]?[0-9]*)$"`
	Memory *string `yaml:"memory,omitempty" jsonschema:"pattern=^([+-]?[0-9.]+)([eEinumkKMGTP]*[-+]?[0-9]*)$"`
}

// validateOptions checks that input Options are correctly set.
// Returns array of error messages, empty if no errors are found
func validateOptions(options Options) (errors []string) {

	// options.scale
	if options.Scale != nil {
		if options.Scale.Min != nil {
			if *options.Scale.Min < 0 {
				errors = append(errors, fmt.Sprintf("options field \"scale.min\" has invalid value set: %d, the value must be greater than \"0\"",
					*options.Scale.Min))
			}
		}

		if options.Scale.Max != nil {
			if *options.Scale.Max < 0 {
				errors = append(errors, fmt.Sprintf("options field \"scale.max\" has invalid value set: %d, the value must be greater than \"0\"",
					*options.Scale.Max))
			}
		}

		if options.Scale.Min != nil && options.Scale.Max != nil {
			if *options.Scale.Max < *options.Scale.Min {
				errors = append(errors, "options field \"scale.max\" value must be greater or equal to \"scale.min\"")
			}
		}

		if options.Scale.Metric != nil {
			if *options.Scale.Metric != "concurrency" && *options.Scale.Metric != "rps" {
				errors = append(errors, fmt.Sprintf("options field \"scale.metric\" has invalid value set: %s, allowed is only \"concurrency\" or \"rps\"",
					*options.Scale.Metric))
			}
		}

		if options.Scale.Target != nil {
			if *options.Scale.Target < 0.01 {
				errors = append(errors, fmt.Sprintf("options field \"scale.target\" has value set to \"%f\", but it must not be less than 0.01",
					*options.Scale.Target))
			}
		}

		if options.Scale.Utilization != nil {
			if *options.Scale.Utilization < 1 || *options.Scale.Utilization > 100 {
				errors = append(errors,
					fmt.Sprintf("options field \"scale.utilization\" has value set to \"%f\", but it must not be less than 1 or greater than 100",
						*options.Scale.Utilization))
			}
		}

		if options.Scale.KEDA != nil && options.Scale.KPA != nil {
			errors = append(errors, "options fields \"scale.keda\" and \"scale.kpa\" are mutually exclusive")
		}

		if options.Scale.KEDA != nil {
			errors = append(errors, validateKEDAScale(options.Scale.KEDA)...)
		}

		if options.Scale.KPA != nil {
			errors = append(errors, validateKPAScale(options.Scale.KPA)...)
		}
	}

	// options.resource
	if options.Resources != nil {

		// options.resource.requests
		if options.Resources.Requests != nil {

			if options.Resources.Requests.CPU != nil {
				_, err := resource.ParseQuantity(*options.Resources.Requests.CPU)
				if err != nil {
					errors = append(errors, fmt.Sprintf("options field \"resources.requests.cpu\" has invalid value set: \"%s\"; \"%s\"",
						*options.Resources.Requests.CPU, err.Error()))
				}
			}

			if options.Resources.Requests.Memory != nil {
				_, err := resource.ParseQuantity(*options.Resources.Requests.Memory)
				if err != nil {
					errors = append(errors, fmt.Sprintf("options field \"resources.requests.memory\" has invalid value set: \"%s\"; \"%s\"",
						*options.Resources.Requests.Memory, err.Error()))
				}
			}
		}

		// options.resource.limits
		if options.Resources.Limits != nil {

			if options.Resources.Limits.CPU != nil {
				_, err := resource.ParseQuantity(*options.Resources.Limits.CPU)
				if err != nil {
					errors = append(errors, fmt.Sprintf("options field \"resources.limits.cpu\" has invalid value set: \"%s\"; \"%s\"",
						*options.Resources.Limits.CPU, err.Error()))
				}
			}

			if options.Resources.Limits.Memory != nil {
				_, err := resource.ParseQuantity(*options.Resources.Limits.Memory)
				if err != nil {
					errors = append(errors, fmt.Sprintf("options field \"resources.limits.memory\" has invalid value set: \"%s\"; \"%s\"",
						*options.Resources.Limits.Memory, err.Error()))
				}
			}

			if options.Resources.Limits.Concurrency != nil {
				if *options.Resources.Limits.Concurrency < 0 {
					errors = append(errors, fmt.Sprintf("options field \"resources.limits.concurrency\" has value set to \"%d\", but it must not be less than 0",
						*options.Resources.Limits.Concurrency))
				}
			}
		}
	}

	return
}

func validateKEDAScale(keda *KEDAScaleOptions) (errors []string) {
	if len(keda.Triggers) == 0 {
		errors = append(errors, "options field \"scale.keda.triggers\" must not be empty when scale.keda is set")
		return
	}
	for i, t := range keda.Triggers {
		switch t.Type {
		case "http":
			// no extra fields required
		case "kafka":
			if t.LagThreshold != nil && *t.LagThreshold < 1 {
				errors = append(errors, fmt.Sprintf("options field \"scale.keda.triggers[%d].lagThreshold\" must be at least 1", i))
			}
			if t.ActivationLagThreshold != nil && *t.ActivationLagThreshold < 0 {
				errors = append(errors, fmt.Sprintf("options field \"scale.keda.triggers[%d].activationLagThreshold\" must not be negative", i))
			}
		case "cron":
			if t.Timezone == "" {
				errors = append(errors, fmt.Sprintf("options field \"scale.keda.triggers[%d].timezone\" is required for cron triggers", i))
			}
			if t.Start == "" {
				errors = append(errors, fmt.Sprintf("options field \"scale.keda.triggers[%d].start\" is required for cron triggers", i))
			}
			if t.End == "" {
				errors = append(errors, fmt.Sprintf("options field \"scale.keda.triggers[%d].end\" is required for cron triggers", i))
			}
			if t.DesiredReplicas == nil {
				errors = append(errors, fmt.Sprintf("options field \"scale.keda.triggers[%d].desiredReplicas\" is required for cron triggers", i))
			} else if *t.DesiredReplicas < 1 {
				errors = append(errors, fmt.Sprintf("options field \"scale.keda.triggers[%d].desiredReplicas\" must be at least 1", i))
			}
		default:
			errors = append(errors, fmt.Sprintf("options field \"scale.keda.triggers[%d].type\" has invalid value %q, allowed: http, kafka, cron", i, t.Type))
		}
	}
	return
}

func validateKPAScale(kpa *KPAScaleOptions) (errors []string) {
	if kpa.Metric != nil {
		if *kpa.Metric != "concurrency" && *kpa.Metric != "rps" {
			errors = append(errors, fmt.Sprintf("options field \"scale.kpa.metric\" has invalid value set: %s, allowed is only \"concurrency\" or \"rps\"",
				*kpa.Metric))
		}
	}
	if kpa.Target != nil {
		if *kpa.Target < 0.01 {
			errors = append(errors, fmt.Sprintf("options field \"scale.kpa.target\" has value set to \"%f\", but it must not be less than 0.01",
				*kpa.Target))
		}
	}
	if kpa.Utilization != nil {
		if *kpa.Utilization < 1 || *kpa.Utilization > 100 {
			errors = append(errors, fmt.Sprintf("options field \"scale.kpa.utilization\" has value set to \"%f\", but it must not be less than 1 or greater than 100",
				*kpa.Utilization))
		}
	}
	return
}
