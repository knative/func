package functions

import "fmt"

type Volume struct {
	Secret    *string `yaml:"secret,omitempty" jsonschema:"oneof_required=secret"`
	ConfigMap *string `yaml:"configMap,omitempty" jsonschema:"oneof_required=configmap"`
	Path      *string `yaml:"path,omitempty"`
}

func (v Volume) String() string {
	if v.ConfigMap != nil {
		return fmt.Sprintf("ConfigMap \"%s\" mounted at path: \"%s\"", *v.ConfigMap, *v.Path)
	} else if v.Secret != nil {
		return fmt.Sprintf("Secret \"%s\" mounted at path: \"%s\"", *v.Secret, *v.Path)
	}

	return ""
}

// validateVolumes checks that input Volumes are correct and contain all necessary fields.
// Returns array of error messages, empty if no errors are found
//
// Allowed settings:
//   - secret: example-secret              		# mount Secret as Volume
//     path: /etc/secret-volume
//   - configMap: example-configMap              	# mount ConfigMap as Volume
//     path: /etc/configMap-volume
func validateVolumes(volumes []Volume) (errors []string) {

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
