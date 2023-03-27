package functions

import (
	"fmt"
	"strings"

	"knative.dev/func/pkg/utils"
)

type Env struct {
	Name  *string `yaml:"name,omitempty" jsonschema:"pattern=^[-._a-zA-Z][-._a-zA-Z0-9]*$"`
	Value *string `yaml:"value,omitempty"`
}

func (e Env) String() string {
	if e.Name == nil && e.Value != nil {
		match := regWholeSecret.FindStringSubmatch(*e.Value)
		if len(match) == 2 {
			return fmt.Sprintf("All key=value pairs from Secret \"%s\"", match[1])
		}
		match = regWholeConfigMap.FindStringSubmatch(*e.Value)
		if len(match) == 2 {
			return fmt.Sprintf("All key=value pairs from ConfigMap \"%s\"", match[1])
		}
	} else if e.Name != nil && e.Value != nil {
		match := regKeyFromSecret.FindStringSubmatch(*e.Value)
		if len(match) == 3 {
			return fmt.Sprintf("Env \"%s\" with value set from key \"%s\" from Secret \"%s\"", *e.Name, match[2], match[1])
		}
		match = regKeyFromConfigMap.FindStringSubmatch(*e.Value)
		if len(match) == 3 {
			return fmt.Sprintf("Env \"%s\" with value set from key \"%s\" from ConfigMap \"%s\"", *e.Name, match[2], match[1])
		}
		match = regLocalEnv.FindStringSubmatch(*e.Value)
		if len(match) == 2 {
			return fmt.Sprintf("Env \"%s\" with value set from local env variable \"%s\"", *e.Name, match[1])
		}

		return fmt.Sprintf("Env \"%s\" with value \"%s\"", *e.Name, *e.Value)
	}
	return ""
}

// KeyValuePair returns a string representation of the Env field in form NAME=VALUE
// if NAME is not defined for an Env, empty string is returned
func (e Env) KeyValuePair() string {
	keyValue := ""
	if e.Name != nil {
		value := ""
		if e.Value != nil {
			value = *e.Value
		}

		keyValue = fmt.Sprintf("%s=%s", *e.Name, value)
	}

	return keyValue
}

// ValidateEnvs checks that input Envs are correct and contain all necessary fields.
// Returns array of error messages, empty if no errors are found
//
// Allowed settings:
//   - name: EXAMPLE1                					# ENV directly from a value
//     value: value1
//   - name: EXAMPLE2                 				# ENV from the local ENV var
//     value: {{ env:MY_ENV }}
//   - name: EXAMPLE3
//     value: {{ secret:secretName:key }}   			# ENV from a key in secret
//   - value: {{ secret:secretName }}          		# all key-pair values from secret are set as ENV
//   - name: EXAMPLE4
//     value: {{ configMap:configMapName:key }}   	# ENV from a key in configMap
//   - value: {{ configMap:configMapName }}          	# all key-pair values from configMap are set as ENV
func ValidateEnvs(envs []Env) (errors []string) {
	for i, env := range envs {
		if env.Name == nil && env.Value == nil {
			errors = append(errors, fmt.Sprintf("env entry #%d is not properly set", i))
		} else if env.Value == nil {
			errors = append(errors, fmt.Sprintf("env entry #%d is missing value field, only name '%s' is set", i, *env.Name))
		} else if env.Name == nil {
			// all key-pair values from secret are set as ENV; {{ secret:secretName }} or {{ configMap:configMapName }}
			if !regWholeSecret.MatchString(*env.Value) && !regWholeConfigMap.MatchString(*env.Value) {
				errors = append(errors, fmt.Sprintf("env entry #%d has invalid value field set, it has '%s', but allowed is only '{{ secret:secretName }}' or '{{ configMap:configMapName }}'",
					i, *env.Value))
			}
		} else {

			if err := utils.ValidateEnvVarName(*env.Name); err != nil {
				errors = append(errors, fmt.Sprintf("env entry #%d has invalid name set: %q; %s", i, *env.Name, err.Error()))
			}

			if strings.HasPrefix(*env.Value, "{{") {
				// ENV from the local ENV var; {{ env:MY_ENV }}
				// or
				// ENV from a key in secret/configMap;  {{ secret:secretName:key }} or {{ configMap:configMapName:key }}
				if !regLocalEnv.MatchString(*env.Value) && !regKeyFromSecret.MatchString(*env.Value) && !regKeyFromConfigMap.MatchString(*env.Value) {
					errors = append(errors,
						fmt.Sprintf(
							"env entry #%d with name '%s' has invalid value field set, it has '%s', but allowed is only '{{ env:MY_ENV }}', '{{ secret:secretName:key }}' or '{{ configMap:configMapName:key }}'",
							i, *env.Name, *env.Value))
				}
			}
		}
	}

	return
}

// ValidateBuildEnvs checks that input BuildEnvs are correct and contain all necessary fields.
// Returns array of error messages, empty if no errors are found
//
// Allowed settings:
//   - name: EXAMPLE1                					# ENV directly from a value
//     value: value1
//   - name: EXAMPLE2                 				# ENV from the local ENV var
//     value: {{ env:MY_ENV }}
func ValidateBuildEnvs(envs []Env) (errors []string) {
	for i, env := range envs {
		if env.Name == nil && env.Value == nil {
			errors = append(errors, fmt.Sprintf("env entry #%d is not properly set", i))
		} else if env.Value == nil {
			errors = append(errors, fmt.Sprintf("env entry #%d is missing value field, only name '%s' is set", i, *env.Name))
		} else {

			if err := utils.ValidateEnvVarName(*env.Name); err != nil {
				errors = append(errors, fmt.Sprintf("env entry #%d has invalid name set: %q; %s", i, *env.Name, err.Error()))
			}

			if strings.HasPrefix(*env.Value, "{{") {
				// ENV from the local ENV var; {{ env:MY_ENV }}
				if !regLocalEnv.MatchString(*env.Value) {
					errors = append(errors,
						fmt.Sprintf(
							"env entry #%d with name '%s' has invalid value field set, it has '%s', but allowed is only '{{ env:MY_ENV }}'",
							i, *env.Name, *env.Value))
				}
			}
		}
	}

	return
}
