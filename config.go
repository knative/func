package function

import (
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/boson-project/func/utils"
	"gopkg.in/yaml.v2"
	"k8s.io/apimachinery/pkg/api/resource"
)

// ConfigFile is the name of the config's serialized form.
const ConfigFile = "func.yaml"

var (
	regWholeSecret      = regexp.MustCompile(`^{{\s*secret:((?:\w|['-]\w)+)\s*}}$`)
	regKeyFromSecret    = regexp.MustCompile(`^{{\s*secret:((?:\w|['-]\w)+):(\w+)\s*}}$`)
	regWholeConfigMap   = regexp.MustCompile(`^{{\s*configMap:((?:\w|['-]\w)+)\s*}}$`)
	regKeyFromConfigMap = regexp.MustCompile(`^{{\s*configMap:((?:\w|['-]\w)+):(\w+)\s*}}$`)
	regLocalEnv         = regexp.MustCompile(`^{{\s*env:(\w+)\s*}}$`)
)

type Volumes []Volume
type Volume struct {
	Secret    *string `yaml:"secret,omitempty"`
	ConfigMap *string `yaml:"configMap,omitempty"`
	Path      *string `yaml:"path"`
}

func (v Volume) String() string {
	if v.ConfigMap != nil {
		return fmt.Sprintf("ConfigMap \"%s\" mounted at path: \"%s\"", *v.ConfigMap, *v.Path)
	} else if v.Secret != nil {
		return fmt.Sprintf("Secret \"%s\" mounted at path: \"%s\"", *v.Secret, *v.Path)
	}

	return ""
}

type Envs []Env
type Env struct {
	Name  *string `yaml:"name,omitempty"`
	Value *string `yaml:"value"`
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

type Options struct {
	Scale     *ScaleOptions     `yaml:"scale,omitempty"`
	Resources *ResourcesOptions `yaml:"resources,omitempty"`
}

type ScaleOptions struct {
	Min         *int64   `yaml:"min,omitempty"`
	Max         *int64   `yaml:"max,omitempty"`
	Metric      *string  `yaml:"metric,omitempty"`
	Target      *float64 `yaml:"target,omitempty"`
	Utilization *float64 `yaml:"utilization,omitempty"`
}

type ResourcesOptions struct {
	Requests *ResourcesRequestsOptions `yaml:"requests,omitempty"`
	Limits   *ResourcesLimitsOptions   `yaml:"limits,omitempty"`
}

type ResourcesLimitsOptions struct {
	CPU         *string `yaml:"cpu,omitempty"`
	Memory      *string `yaml:"memory,omitempty"`
	Concurrency *int64  `yaml:"concurrency,omitempty"`
}

type ResourcesRequestsOptions struct {
	CPU    *string `yaml:"cpu,omitempty"`
	Memory *string `yaml:"memory,omitempty"`
}

// Config represents the serialized state of a Function's metadata.
// See the Function struct for attribute documentation.
type config struct {
	Name        string            `yaml:"name"`
	Namespace   string            `yaml:"namespace"`
	Runtime     string            `yaml:"runtime"`
	Image       string            `yaml:"image"`
	ImageDigest string            `yaml:"imageDigest"`
	Builder     string            `yaml:"builder"`
	BuilderMap  map[string]string `yaml:"builderMap"`
	Volumes     Volumes           `yaml:"volumes"`
	Envs        Envs              `yaml:"envs"`
	Annotations map[string]string `yaml:"annotations"`
	Options     Options           `yaml:"options"`
	// Add new values to the toConfig/fromConfig functions.
}

// newConfig returns a Config populated from data serialized to disk if it is
// available.  Errors are returned if the path is not valid, if there are
// errors accessing an extant config file, or the contents of the file do not
// unmarshall.  A missing file at a valid path does not error but returns the
// empty value of Config.
func newConfig(root string) (c config, err error) {
	filename := filepath.Join(root, ConfigFile)
	if _, err = os.Stat(filename); err != nil {
		// do not consider a missing config file an error.  Just return.
		if os.IsNotExist(err) {
			err = nil
		}
		return
	}
	bb, err := ioutil.ReadFile(filename)
	if err != nil {
		return
	}

	errMsg := ""
	errMsgHeader := "'func.yaml' config file is not valid:\n"
	errMsgReg := regexp.MustCompile("not found in type .*")

	// Let's try to unmarshal the config file, any fields that are found
	// in the data that do not have corresponding struct members, or mapping
	// keys that are duplicates, will result in an error.
	err = yaml.UnmarshalStrict(bb, &c)
	if err != nil {
		errMsg = err.Error()

		if strings.HasPrefix(errMsg, "yaml: unmarshal errors:") {
			errMsg = errMsgReg.ReplaceAllString(errMsg, "is not valid")
			errMsg = strings.Replace(errMsg, "yaml: unmarshal errors:\n", errMsgHeader, 1)
		} else if strings.HasPrefix(errMsg, "yaml:") {
			errMsg = errMsgReg.ReplaceAllString(errMsg, "is not valid")
			errMsg = strings.Replace(errMsg, "yaml: ", errMsgHeader+"  ", 1)
		}
	}

	// Let's check that all entries in `volumes`, `envs` and `options` contain all required fields
	volumesErrors := validateVolumes(c.Volumes)
	envsErrors := ValidateEnvs(c.Envs)
	optionsErrors := validateOptions(c.Options)
	if len(volumesErrors) > 0 || len(envsErrors) > 0 || len(optionsErrors) > 0 {
		// if there aren't any previously reported errors, we need to set the error message header first
		if errMsg == "" {
			errMsg = errMsgHeader
		} else {
			// if there are some previously reporeted errors, we need to indent them
			errMsg = errMsg + "\n"
		}

		// lets make the error message a little bit nice -> indent each error message
		for i := range volumesErrors {
			volumesErrors[i] = "  " + volumesErrors[i]
		}
		for i := range envsErrors {
			envsErrors[i] = "  " + envsErrors[i]
		}
		for i := range optionsErrors {
			optionsErrors[i] = "  " + optionsErrors[i]
		}

		errMsg = errMsg + strings.Join(volumesErrors, "\n")
		// we have errors from both volumes and envs sections -> let's make sure they are both indented
		if len(volumesErrors) > 0 && len(envsErrors) > 0 {
			errMsg = errMsg + "\n"
		}
		errMsg = errMsg + strings.Join(envsErrors, "\n")
		// lets indent options related errors if there are already some set
		if len(optionsErrors) > 0 && (len(volumesErrors) > 0 || len(envsErrors) > 0) {
			errMsg = errMsg + "\n"
		}
		errMsg = errMsg + strings.Join(optionsErrors, "\n")
	}

	if errMsg != "" {
		err = errors.New(errMsg)
	}

	return
}

// fromConfig returns a Function populated from config.
// Note that config does not include ancillary fields not serialized, such as Root.
func fromConfig(c config) (f Function) {
	return Function{
		Name:        c.Name,
		Namespace:   c.Namespace,
		Runtime:     c.Runtime,
		Image:       c.Image,
		ImageDigest: c.ImageDigest,
		Builder:     c.Builder,
		BuilderMap:  c.BuilderMap,
		Volumes:     c.Volumes,
		Envs:        c.Envs,
		Annotations: c.Annotations,
		Options:     c.Options,
	}
}

// toConfig serializes a Function to a config object.
func toConfig(f Function) config {
	return config{
		Name:        f.Name,
		Namespace:   f.Namespace,
		Runtime:     f.Runtime,
		Image:       f.Image,
		ImageDigest: f.ImageDigest,
		Builder:     f.Builder,
		BuilderMap:  f.BuilderMap,
		Volumes:     f.Volumes,
		Envs:        f.Envs,
		Annotations: f.Annotations,
		Options:     f.Options,
	}
}

// writeConfig for the given Function out to disk at root.
func writeConfig(f Function) (err error) {
	path := filepath.Join(f.Root, ConfigFile)
	c := toConfig(f)
	var bb []byte
	if bb, err = yaml.Marshal(&c); err != nil {
		return
	}
	return ioutil.WriteFile(path, bb, 0644)
}

// validateVolumes checks that input Volumes are correct and contain all necessary fields.
// Returns array of error messages, empty if no errors are found
//
// Allowed settings:
// - secret: example-secret              		# mount Secret as Volume
// 	 path: /etc/secret-volume
// - configMap: example-configMap              	# mount ConfigMap as Volume
// 	 path: /etc/configMap-volume
func validateVolumes(volumes Volumes) (errors []string) {

	for i, vol := range volumes {
		if vol.Secret != nil && vol.ConfigMap != nil {
			errors = append(errors, fmt.Sprintf("volume entry #%d is not properly set, both secret '%s' and configMap '%s' can not be set at the same time",
				i, *vol.Secret, *vol.ConfigMap))
		} else if vol.Path == nil && vol.Secret == nil && vol.ConfigMap == nil {
			errors = append(errors, fmt.Sprintf("volume entry #%d is not properly set", i))
		} else if vol.Path == nil {
			if vol.Secret != nil {
				errors = append(errors, fmt.Sprintf("volume entry #%d is missing path field, only secret '%s' is set", i, *vol.Secret))
			} else if vol.ConfigMap != nil {
				errors = append(errors, fmt.Sprintf("volume entry #%d is missing path field, only configMap '%s' is set", i, *vol.ConfigMap))
			}
		} else if vol.Path != nil && vol.Secret == nil && vol.ConfigMap == nil {
			errors = append(errors, fmt.Sprintf("volume entry #%d is missing secret or configMap field, only path '%s' is set", i, *vol.Path))
		}
	}

	return
}

// ValidateEnvs checks that input Envs are correct and contain all necessary fields.
// Returns array of error messages, empty if no errors are found
//
// Allowed settings:
// - name: EXAMPLE1                					# ENV directly from a value
//   value: value1
// - name: EXAMPLE2                 				# ENV from the local ENV var
//   value: {{ env:MY_ENV }}
// - name: EXAMPLE3
//   value: {{ secret:secretName:key }}   			# ENV from a key in secret
// - value: {{ secret:secretName }}          		# all key-pair values from secret are set as ENV
// - name: EXAMPLE4
//   value: {{ configMap:configMapName:key }}   	# ENV from a key in configMap
// - value: {{ configMap:configMapName }}          	# all key-pair values from configMap are set as ENV
func ValidateEnvs(envs Envs) (errors []string) {

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
