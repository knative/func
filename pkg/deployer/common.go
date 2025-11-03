package deployer

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/rand"
	"k8s.io/apimachinery/pkg/util/sets"
	clienteventingv1 "knative.dev/client/pkg/eventing/v1"
	eventingv1 "knative.dev/eventing/pkg/apis/eventing/v1"
	duckv1 "knative.dev/pkg/apis/duck/v1"
	"knative.dev/pkg/kmeta"

	fn "knative.dev/func/pkg/functions"
	"knative.dev/func/pkg/k8s"
)

const (
	DeployTypeAnnotation = "function.knative.dev/deploy-type"

	KnativeDeployerName    = "knative"
	KubernetesDeployerName = "raw"

	DefaultLivenessEndpoint  = "/health/liveness"
	DefaultReadinessEndpoint = "/health/readiness"
	DefaultHTTPPort          = 8080

	// Dapr constants
	DaprEnabled          = "true"
	DaprMetricsPort      = "9092"
	DaprEnableAPILogging = "true"
)

// DeployDecorator is an interface for customizing deployment metadata
type DeployDecorator interface {
	UpdateAnnotations(fn.Function, map[string]string) map[string]string
	UpdateLabels(fn.Function, map[string]string) map[string]string
}

// GenerateCommonLabels creates labels common to both Knative and K8s deployments
func GenerateCommonLabels(f fn.Function, decorator DeployDecorator) (map[string]string, error) {
	ll, err := f.LabelsMap()
	if err != nil {
		return nil, err
	}

	// Standard function labels
	ll["boson.dev/function"] = "true"
	ll["function.knative.dev/name"] = f.Name
	ll["function.knative.dev/runtime"] = f.Runtime

	if f.Domain != "" {
		ll["func.domain"] = f.Domain
	}

	if decorator != nil {
		ll = decorator.UpdateLabels(f, ll)
	}

	return ll, nil
}

// GenerateCommonAnnotations creates annotations common to both Knative and K8s deployments
func GenerateCommonAnnotations(f fn.Function, decorator DeployDecorator, daprInstalled bool, deployType string) map[string]string {
	aa := make(map[string]string)

	// Add Dapr annotations if Dapr is installed
	if daprInstalled {
		for k, v := range GenerateDaprAnnotations(f.Name) {
			aa[k] = v
		}
	}

	if len(deployType) > 0 {
		aa[DeployTypeAnnotation] = deployType
	}

	// Add user-defined annotations
	for k, v := range f.Deploy.Annotations {
		aa[k] = v
	}

	// Apply decorator
	if decorator != nil {
		aa = decorator.UpdateAnnotations(f, aa)
	}

	return aa
}

// SetHealthEndpoints configures health probes for a container
func SetHealthEndpoints(f fn.Function, container *corev1.Container) {
	livenessPath := DefaultLivenessEndpoint
	if f.Deploy.HealthEndpoints.Liveness != "" {
		livenessPath = f.Deploy.HealthEndpoints.Liveness
	}

	readinessPath := DefaultReadinessEndpoint
	if f.Deploy.HealthEndpoints.Readiness != "" {
		readinessPath = f.Deploy.HealthEndpoints.Readiness
	}

	container.LivenessProbe = &corev1.Probe{
		ProbeHandler: corev1.ProbeHandler{
			HTTPGet: &corev1.HTTPGetAction{
				Path: livenessPath,
				Port: intstr.FromInt32(DefaultHTTPPort),
			},
		},
	}

	container.ReadinessProbe = &corev1.Probe{
		ProbeHandler: corev1.ProbeHandler{
			HTTPGet: &corev1.HTTPGetAction{
				Path: readinessPath,
				Port: intstr.FromInt32(DefaultHTTPPort),
			},
		},
	}
}

// SetSecurityContext configures security settings for a container
func SetSecurityContext(container *corev1.Container) {
	runAsNonRoot := true
	allowPrivilegeEscalation := false
	capabilities := corev1.Capabilities{
		Drop: []corev1.Capability{"ALL"},
	}
	seccompProfile := corev1.SeccompProfile{
		Type: "RuntimeDefault",
	}
	container.SecurityContext = &corev1.SecurityContext{
		RunAsNonRoot:             &runAsNonRoot,
		AllowPrivilegeEscalation: &allowPrivilegeEscalation,
		Capabilities:             &capabilities,
		SeccompProfile:           &seccompProfile,
	}
}

// GenerateDaprAnnotations generates annotations for Dapr support
// These annotations, if included and Dapr control plane is installed in
// the target cluster, will result in a sidecar exposing the Dapr HTTP API
// on localhost:3500 and metrics on 9092
func GenerateDaprAnnotations(appID string) map[string]string {
	aa := make(map[string]string)
	aa["dapr.io/app-id"] = appID
	aa["dapr.io/enabled"] = DaprEnabled
	aa["dapr.io/metrics-port"] = DaprMetricsPort
	aa["dapr.io/app-port"] = "8080"
	aa["dapr.io/enable-api-logging"] = DaprEnableAPILogging
	return aa
}

// ProcessEnvs generates array of EnvVars and EnvFromSources from a function config
// envs:
//   - name: EXAMPLE1                            # ENV directly from a value
//     value: value1
//   - name: EXAMPLE2                            # ENV from the local ENV var
//     value: {{ env:MY_ENV }}
//   - name: EXAMPLE3
//     value: {{ secret:example-secret:key }}    # ENV from a key in Secret
//   - value: {{ secret:example-secret }}        # all ENVs from Secret
//   - name: EXAMPLE4
//     value: {{ configMap:configMapName:key }}  # ENV from a key in ConfigMap
//   - value: {{ configMap:configMapName }}      # all key-pair values from ConfigMap are set as ENV
func ProcessEnvs(envs []fn.Env, referencedSecrets, referencedConfigMaps *sets.Set[string]) ([]corev1.EnvVar, []corev1.EnvFromSource, error) {

	envs = withOpenAddress(envs) // prepends ADDRESS=0.0.0.0 if not extant

	envVars := []corev1.EnvVar{{Name: "BUILT", Value: time.Now().Format("20060102T150405")}}
	envFrom := []corev1.EnvFromSource{}

	for _, env := range envs {
		if env.Name == nil && env.Value != nil {
			// all key-pair values from secret/configMap are set as ENV, eg. {{ secret:secretName }} or {{ configMap:configMapName }}
			if strings.HasPrefix(*env.Value, "{{") {
				envFromSource, err := createEnvFromSource(*env.Value, referencedSecrets, referencedConfigMaps)
				if err != nil {
					return nil, nil, err
				}
				envFrom = append(envFrom, *envFromSource)
				continue
			}
		} else if env.Name != nil && env.Value != nil {
			if strings.HasPrefix(*env.Value, "{{") {
				slices := strings.Split(strings.Trim(*env.Value, "{} "), ":")
				if len(slices) == 3 {
					// ENV from a key in secret/configMap, eg. FOO={{ secret:secretName:key }} FOO={{ configMap:configMapName.key }}
					valueFrom, err := createEnvVarSource(slices, referencedSecrets, referencedConfigMaps)
					envVars = append(envVars, corev1.EnvVar{Name: *env.Name, ValueFrom: valueFrom})
					if err != nil {
						return nil, nil, err
					}
					continue
				} else if len(slices) == 2 {
					// ENV from the local ENV var, eg. FOO={{ env:LOCAL_ENV }}
					localValue, err := processLocalEnvValue(*env.Value)
					if err != nil {
						return nil, nil, err
					}
					envVars = append(envVars, corev1.EnvVar{Name: *env.Name, Value: localValue})
					continue
				}
			} else {
				// a standard ENV with key and value, eg. FOO=bar
				envVars = append(envVars, corev1.EnvVar{Name: *env.Name, Value: *env.Value})
				continue
			}
		}
		return nil, nil, fmt.Errorf("unsupported env source entry \"%v\"", env)
	}

	return envVars, envFrom, nil
}

// withOpenAddress prepends ADDRESS=0.0.0.0 to the envs if not present.
//
// This is combined with the value of PORT at runtime to determine the full
// Listener address on which a Function will listen tcp requests.
//
// Runtimes should, by default, only listen on the loopback interface by
// default, as they may be `func run` locally, for security purposes.
// This environment variable instructs the runtimes to listen on all interfaces
// by default when actually being deployed, since they will need to actually
// listen for client requests and for health readiness/liveness probes.
//
// Should a user wish to securely open their function to only receive requests
// on a specific interface, such as a WireGuard-encrypted mesh network which
// presents as a specific interface, that can be achieved by setting the
// ADDRESS value as an environment variable on their function to the interface
// on which to listen.
//
// NOTE this env is currently only respected by scaffolded Go functions, because
// they are the only ones which support being `func run` locally.  Other
// runtimes will respect the value as they are updated to support scaffolding.
func withOpenAddress(ee []fn.Env) []fn.Env {
	// TODO: this is unnecessarily complex due to both key and value of the
	// envs slice being being pointers.  There is an outstanding tech-debt item
	// to remove pointers from Function Envs, Volumes, Labels, and Options.
	var found bool
	for _, e := range ee {
		if e.Name != nil && *e.Name == "ADDRESS" {
			found = true
			break
		}
	}
	if !found {
		k := "ADDRESS"
		v := "0.0.0.0"
		ee = append(ee, fn.Env{Name: &k, Value: &v})
	}
	return ee
}

func createEnvFromSource(value string, referencedSecrets, referencedConfigMaps *sets.Set[string]) (*corev1.EnvFromSource, error) {
	slices := strings.Split(strings.Trim(value, "{} "), ":")
	if len(slices) != 2 {
		return nil, fmt.Errorf("env requires a value in form \"resourceType:name\" where \"resourceType\" can be one of \"configMap\" or \"secret\"; got %q", slices)
	}

	envVarSource := corev1.EnvFromSource{}

	typeString := strings.TrimSpace(slices[0])
	sourceName := strings.TrimSpace(slices[1])

	var sourceType string

	switch typeString {
	case "configMap":
		sourceType = "ConfigMap"
		envVarSource.ConfigMapRef = &corev1.ConfigMapEnvSource{
			LocalObjectReference: corev1.LocalObjectReference{
				Name: sourceName,
			}}

		if !referencedConfigMaps.Has(sourceName) {
			referencedConfigMaps.Insert(sourceName)
		}
	case "secret":
		sourceType = "Secret"
		envVarSource.SecretRef = &corev1.SecretEnvSource{
			LocalObjectReference: corev1.LocalObjectReference{
				Name: sourceName,
			}}
		if !referencedSecrets.Has(sourceName) {
			referencedSecrets.Insert(sourceName)
		}
	default:
		return nil, fmt.Errorf("unsupported env source type %q; supported source types are \"configMap\" or \"secret\"", slices[0])
	}

	if len(sourceName) == 0 {
		return nil, fmt.Errorf("the name of %s cannot be an empty string", sourceType)
	}

	return &envVarSource, nil
}

func createEnvVarSource(slices []string, referencedSecrets, referencedConfigMaps *sets.Set[string]) (*corev1.EnvVarSource, error) {
	if len(slices) != 3 {
		return nil, fmt.Errorf("env requires a value in form \"resourceType:name:key\" where \"resourceType\" can be one of \"configMap\" or \"secret\"; got %q", slices)
	}

	envVarSource := corev1.EnvVarSource{}

	typeString := strings.TrimSpace(slices[0])
	sourceName := strings.TrimSpace(slices[1])
	sourceKey := strings.TrimSpace(slices[2])

	var sourceType string

	switch typeString {
	case "configMap":
		sourceType = "ConfigMap"
		envVarSource.ConfigMapKeyRef = &corev1.ConfigMapKeySelector{
			LocalObjectReference: corev1.LocalObjectReference{
				Name: sourceName,
			},
			Key: sourceKey}

		if !referencedConfigMaps.Has(sourceName) {
			referencedConfigMaps.Insert(sourceName)
		}
	case "secret":
		sourceType = "Secret"
		envVarSource.SecretKeyRef = &corev1.SecretKeySelector{
			LocalObjectReference: corev1.LocalObjectReference{
				Name: sourceName,
			},
			Key: sourceKey}

		if !referencedSecrets.Has(sourceName) {
			referencedSecrets.Insert(sourceName)
		}
	default:
		return nil, fmt.Errorf("unsupported env source type %q; supported source types are \"configMap\" or \"secret\"", slices[0])
	}

	if len(sourceName) == 0 {
		return nil, fmt.Errorf("the name of %s cannot be an empty string", sourceType)
	}

	if len(sourceKey) == 0 {
		return nil, fmt.Errorf("the key referenced by resource %s %q cannot be an empty string", sourceType, sourceName)
	}

	return &envVarSource, nil
}

var evRegex = regexp.MustCompile(`^{{\s*(\w+)\s*:(\w+)\s*}}$`)

const (
	ctxIdx = 1
	valIdx = 2
)

func processLocalEnvValue(val string) (string, error) {
	match := evRegex.FindStringSubmatch(val)
	if len(match) > valIdx {
		if match[ctxIdx] != "env" {
			return "", fmt.Errorf("allowed env value entry is \"{{ env:LOCAL_VALUE }}\"; got: %q", match[ctxIdx])
		}
		if v, ok := os.LookupEnv(match[valIdx]); ok {
			return v, nil
		} else {
			return "", fmt.Errorf("required local environment variable %q is not set", match[valIdx])
		}
	} else {
		return val, nil
	}
}

// ProcessVolumes generates Volumes and VolumeMounts from a function config
// volumes:
//   - secret: example-secret                              # mount Secret as Volume
//     path: /etc/secret-volume
//   - configMap: example-configMap                        # mount ConfigMap as Volume
//     path: /etc/configMap-volume
//   - persistentVolumeClaim: { claimName: example-pvc }   # mount PersistentVolumeClaim as Volume
//     path: /etc/secret-volume
//   - emptyDir: {}                                         # mount EmptyDir as Volume
//     path: /etc/configMap-volume
func ProcessVolumes(volumes []fn.Volume, referencedSecrets, referencedConfigMaps, referencedPVCs *sets.Set[string]) ([]corev1.Volume, []corev1.VolumeMount, error) {
	createdVolumes := sets.NewString()
	usedPaths := sets.NewString()

	newVolumes := []corev1.Volume{}
	newVolumeMounts := []corev1.VolumeMount{}

	for _, vol := range volumes {

		volumeName := ""

		if vol.Secret != nil {
			volumeName = "secret-" + *vol.Secret

			if !createdVolumes.Has(volumeName) {
				newVolumes = append(newVolumes, corev1.Volume{
					Name: volumeName,
					VolumeSource: corev1.VolumeSource{
						Secret: &corev1.SecretVolumeSource{
							SecretName: *vol.Secret,
						},
					},
				})
				createdVolumes.Insert(volumeName)

				if !referencedSecrets.Has(*vol.Secret) {
					referencedSecrets.Insert(*vol.Secret)
				}
			}
		} else if vol.ConfigMap != nil {
			volumeName = "config-map-" + *vol.ConfigMap

			if !createdVolumes.Has(volumeName) {
				newVolumes = append(newVolumes, corev1.Volume{
					Name: volumeName,
					VolumeSource: corev1.VolumeSource{
						ConfigMap: &corev1.ConfigMapVolumeSource{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: *vol.ConfigMap,
							},
						},
					},
				})
				createdVolumes.Insert(volumeName)

				if !referencedConfigMaps.Has(*vol.ConfigMap) {
					referencedConfigMaps.Insert(*vol.ConfigMap)
				}
			}
		} else if vol.PersistentVolumeClaim != nil {
			volumeName = "pvc-" + *vol.PersistentVolumeClaim.ClaimName

			if !createdVolumes.Has(volumeName) {
				newVolumes = append(newVolumes, corev1.Volume{
					Name: volumeName,
					VolumeSource: corev1.VolumeSource{
						PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
							ClaimName: *vol.PersistentVolumeClaim.ClaimName,
							ReadOnly:  vol.PersistentVolumeClaim.ReadOnly,
						},
					},
				})
				createdVolumes.Insert(volumeName)

				if !referencedPVCs.Has(*vol.PersistentVolumeClaim.ClaimName) {
					referencedPVCs.Insert(*vol.PersistentVolumeClaim.ClaimName)
				}
			}
		} else if vol.EmptyDir != nil {
			volumeName = "empty-dir-" + rand.String(7)

			if !createdVolumes.Has(volumeName) {

				var sizeLimit *resource.Quantity
				if vol.EmptyDir.SizeLimit != nil {
					sl, err := resource.ParseQuantity(*vol.EmptyDir.SizeLimit)
					if err != nil {
						return nil, nil, fmt.Errorf("invalid quantity for sizeLimit: %s. Error: %s", *vol.EmptyDir.SizeLimit, err)
					}
					sizeLimit = &sl
				}

				newVolumes = append(newVolumes, corev1.Volume{
					Name: volumeName,
					VolumeSource: corev1.VolumeSource{
						EmptyDir: &corev1.EmptyDirVolumeSource{
							Medium:    corev1.StorageMedium(vol.EmptyDir.Medium),
							SizeLimit: sizeLimit,
						},
					},
				})
				createdVolumes.Insert(volumeName)
			}
		}

		if volumeName != "" {
			if !usedPaths.Has(*vol.Path) {
				newVolumeMounts = append(newVolumeMounts, corev1.VolumeMount{
					Name:      volumeName,
					MountPath: *vol.Path,
				})
				usedPaths.Insert(*vol.Path)
			} else {
				return nil, nil, fmt.Errorf("mount path %s is defined multiple times", *vol.Path)
			}
		}
	}

	return newVolumes, newVolumeMounts, nil
}

// CheckResourcesArePresent returns error if Secrets or ConfigMaps
// referenced in input sets are not deployed on the cluster in the specified namespace
func CheckResourcesArePresent(ctx context.Context, namespace string, referencedSecrets, referencedConfigMaps, referencedPVCs *sets.Set[string], referencedServiceAccount string) error {
	errMsg := ""
	for s := range *referencedSecrets {
		_, err := k8s.GetSecret(ctx, s, namespace)
		if err != nil {
			if errors.IsForbidden(err) {
				errMsg += " Ensure that the service account has the necessary permissions to access the secret.\n"
			} else {
				errMsg += fmt.Sprintf("  referenced Secret \"%s\" is not present in namespace \"%s\"\n", s, namespace)
			}
		}
	}

	for cm := range *referencedConfigMaps {
		_, err := k8s.GetConfigMap(ctx, cm, namespace)
		if err != nil {
			errMsg += fmt.Sprintf("  referenced ConfigMap \"%s\" is not present in namespace \"%s\"\n", cm, namespace)
		}
	}

	for pvc := range *referencedPVCs {
		_, err := k8s.GetPersistentVolumeClaim(ctx, pvc, namespace)
		if err != nil {
			errMsg += fmt.Sprintf("  referenced PersistentVolumeClaim \"%s\" is not present in namespace \"%s\"\n", pvc, namespace)
		}
	}

	// check if referenced ServiceAccount is present in the namespace if it is not default
	if referencedServiceAccount != "" && referencedServiceAccount != "default" {
		err := k8s.GetServiceAccount(ctx, referencedServiceAccount, namespace)
		if err != nil {
			errMsg += fmt.Sprintf("  referenced ServiceAccount \"%s\" is not present in namespace \"%s\"\n", referencedServiceAccount, namespace)
		}
	}

	if errMsg != "" {
		return fmt.Errorf("error(s) while validating resources:\n%s", errMsg)
	}

	return nil
}

func CreateTriggers(ctx context.Context, f fn.Function, obj kmeta.Accessor, eventingClient clienteventingv1.KnEventingClient) error {
	fmt.Fprintf(os.Stderr, "🎯 Creating Triggers on the cluster\n")

	for i, sub := range f.Deploy.Subscriptions {
		// create the filter:
		attributes := make(map[string]string)
		for key, value := range sub.Filters {
			attributes[key] = value
		}

		err := eventingClient.CreateTrigger(ctx, &eventingv1.Trigger{
			ObjectMeta: metav1.ObjectMeta{
				Name: fmt.Sprintf("%s-function-trigger-%d", obj.GetName(), i),
				OwnerReferences: []metav1.OwnerReference{
					{
						APIVersion: obj.GroupVersionKind().Version,
						Kind:       obj.GroupVersionKind().Kind,
						Name:       obj.GetName(),
						UID:        obj.GetUID(),
					},
				},
			},
			Spec: eventingv1.TriggerSpec{
				Broker: sub.Source,

				Subscriber: duckv1.Destination{
					Ref: &duckv1.KReference{
						APIVersion: obj.GroupVersionKind().Version,
						Kind:       obj.GroupVersionKind().Kind,
						Name:       obj.GetName(),
					}},

				Filter: &eventingv1.TriggerFilter{
					Attributes: attributes,
				},
			},
		})
		if err != nil && !errors.IsAlreadyExists(err) {
			err = fmt.Errorf("knative deployer failed to create the Trigger: %v", err)
			return err
		}
	}
	return nil
}

func UsesKnativeDeployer(annoations map[string]string) bool {
	deployType, ok := annoations[DeployTypeAnnotation]

	// if annotation is not set (which defines for backwards compatibility the knative deployType) or the deployType
	// is set explicitly to the knative deployer, we need to handle this service
	return !ok || deployType == KnativeDeployerName
}

func UsesRawDeployer(annotations map[string]string) bool {
	deployType, ok := annotations[DeployTypeAnnotation]

	return ok && deployType == KubernetesDeployerName
}
