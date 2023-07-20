package functions

import (
	"fmt"
	"os"
	"strings"

	"knative.dev/func/pkg/utils"
)

type Label struct {
	// Key consist of optional prefix part (ended by '/') and name part
	// Prefix part validation pattern: [a-z0-9]([-a-z0-9]*[a-z0-9])?(\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*
	// Name part validation pattern: ([A-Za-z0-9][-A-Za-z0-9_.]*)?[A-Za-z0-9]
	Key   *string `yaml:"key" jsonschema:"pattern=^([a-z0-9]([-a-z0-9]*[a-z0-9])?(\\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*\\/)?([A-Za-z0-9][-A-Za-z0-9_.]*)?[A-Za-z0-9]$"`
	Value *string `yaml:"value,omitempty" jsonschema:"pattern=^(([A-Za-z0-9][-A-Za-z0-9_.]*)?[A-Za-z0-9])?$"`
}

func (l Label) String() string {
	if l.Key != nil && l.Value == nil {
		return fmt.Sprintf("Label with key \"%s\"", *l.Key)
	} else if l.Key != nil && l.Value != nil {
		match := regLocalEnv.FindStringSubmatch(*l.Value)
		if len(match) == 2 {
			return fmt.Sprintf("Label with key \"%s\" and value set from local env variable \"%s\"", *l.Key, match[1])
		}
		return fmt.Sprintf("Label with key \"%s\" and value \"%s\"", *l.Key, *l.Value)
	}
	return ""
}

// ValidateLabels checks that input labels are correct and contain all necessary fields.
// Returns array of error messages, empty if no errors are found
//
// Allowed settings:
//   - key: EXAMPLE1                				# label directly from a value
//     value: value1
//   - key: EXAMPLE2                 				# label from the local ENV var
//     value: {{ env:MY_ENV }}
func ValidateLabels(labels []Label) (errors []string) {
	for i, label := range labels {
		if label.Key == nil && label.Value == nil {
			errors = append(errors, fmt.Sprintf("label entry #%d is not properly set", i))
		} else if label.Key == nil && label.Value != nil {
			errors = append(errors, fmt.Sprintf("label entry #%d is missing key field, only value '%s' is set", i, *label.Value))
		} else {
			if err := utils.ValidateLabelKey(*label.Key); err != nil {
				errors = append(errors, fmt.Sprintf("label entry #%d has invalid key set: %q; %s", i, *label.Key, err.Error()))
			}
			if label.Value != nil {
				if err := utils.ValidateLabelValue(*label.Value); err != nil {
					errors = append(errors, fmt.Sprintf("label entry #%d has invalid value set: %q; %s", i, *label.Value, err.Error()))
				}

				if strings.HasPrefix(*label.Value, "{{") {
					// ENV from the local ENV var; {{ env:MY_ENV }}
					if !regLocalEnv.MatchString(*label.Value) {
						errors = append(errors,
							fmt.Sprintf(
								"label entry #%d with key '%s' has invalid value field set, it has '%s', but allowed is only '{{ env:MY_ENV }}'",
								i, *label.Key, *label.Value))
					} else {
						match := regLocalEnv.FindStringSubmatch(*label.Value)
						value := os.Getenv(match[1])
						if err := utils.ValidateLabelValue(value); err != nil {
							errors = append(errors, fmt.Sprintf("label entry #%d with key '%s' has invalid value when the environment is evaluated: '%s': %s", i, *label.Key, value, err.Error()))
						}
					}
				}
			}
		}
	}

	return
}
