package functions

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

type Volume struct {
	Secret                *string                `yaml:"secret,omitempty" jsonschema:"oneof_required=secret,presistentVolumeClaim,emptyDir"`
	ConfigMap             *string                `yaml:"configMap,omitempty" jsonschema:"oneof_required=configmap,presistentVolumeClaim,emptyDir"`
	PresistentVolumeClaim *PersistentVolumeClaim `yaml:"presistentVolumeClaim,omitempty" jsonschema:"oneof_required=configmap,secret,emptyDir"`
	EmptyDir              *EmptyDir              `yaml:"emptyDir,omitempty" jsonschema:"oneof_required=configmap,secret,presistentVolumeClaim"`
	Path                  *string                `yaml:"path,omitempty"`
}

type PersistentVolumeClaim struct {
	// claimName is the name of a PersistentVolumeClaim in the same namespace as the pod using this volume.
	// More info: https://kubernetes.io/docs/concepts/storage/persistent-volumes#persistentvolumeclaims
	ClaimName string `yaml:"claimName"`
	// readOnly Will force the ReadOnly setting in VolumeMounts.
	// Default false.
	ReadOnly bool `yaml:"readOnly,omitempty"`
}

type EmptyDir struct {
	// medium represents what type of storage medium should back this directory.
	// The default is "" which means to use the node's default medium.
	// Must be an empty string (default) or Memory.
	// More info: https://kubernetes.io/docs/concepts/storage/volumes#emptydir
	Medium corev1.StorageMedium `yaml:"medium,omitempty"`
	// sizeLimit is the total amount of local storage required for this EmptyDir volume.
	// The size limit is also applicable for memory medium.
	// The maximum usage on memory medium EmptyDir would be the minimum value between
	// the SizeLimit specified here and the sum of memory limits of all containers in a pod.
	// The default is nil which means that the limit is undefined.
	// More info: http://kubernetes.io/docs/user-guide/volumes#emptydir
	SizeLimit *resource.Quantity `yaml:"sizeLimit,omitempty"`
}

func (v Volume) String() string {
	var result string
	if v.ConfigMap != nil {
		result = fmt.Sprintf("ConfigMap \"%s\"", *v.ConfigMap)
	} else if v.Secret != nil {
		result = fmt.Sprintf("Secret \"%s\"", *v.Secret)
	} else if v.PresistentVolumeClaim != nil {
		result = fmt.Sprintf("PersistentVolumeClaim \"%s\"", v.PresistentVolumeClaim.ClaimName)
	} else if v.EmptyDir != nil {
		result = "EmptyDir"
	} else {
		result = "No volume type"
	}

	if v.Path != nil {
		result += fmt.Sprintf(" at path: \"%s\"", *v.Path)
	}
	return result
}

// validateVolumes checks that input Volumes are correct and contain all necessary fields.
// Returns array of error messages, empty if no errors are found
//
// Allowed settings:
//   - secret: example-secret                              # mount Secret as Volume
//     path: /etc/secret-volume
//   - configMap: example-configMap                        # mount ConfigMap as Volume
//     path: /etc/configMap-volume
//   - persistentVolumeClaim: { claimName: example-pvc }   # mount PersistentVolumeClaim as Volume
//     path: /etc/secret-volume
//   - emptyDir: {}                                         # mount EmptyDir as Volume
//     path: /etc/configMap-volume
func validateVolumes(volumes []Volume) (errors []string) {

	for i, vol := range volumes {
		numVolumes := 0
		if vol.Secret != nil {
			numVolumes++
		}

		if vol.ConfigMap != nil {
			numVolumes++
		}

		if vol.PresistentVolumeClaim != nil {
			numVolumes++
		}

		if vol.EmptyDir != nil {
			numVolumes++
			if vol.EmptyDir.Medium != corev1.StorageMediumDefault && vol.EmptyDir.Medium != corev1.StorageMediumMemory {
				errors = append(errors, fmt.Sprintf("volume entry #%d (%s) has invalid storage medium (%s)", i, vol, vol.EmptyDir.Medium))
			}
		}

		if numVolumes == 0 {
			errors = append(errors, fmt.Sprintf("volume entry #%d (%s) is missing a volume type", i, vol))
		} else if numVolumes > 1 {
			errors = append(errors, fmt.Sprintf("volume entry #%d (%s) may not specify more than one volume type", i, vol))
		}

		if vol.Path == nil {
			errors = append(errors, fmt.Sprintf("volume entry #%d (%s) is missing path field", i, vol))
		}
	}

	return
}
