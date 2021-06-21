package knative

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
	"k8s.io/apimachinery/pkg/util/sets"
	"knative.dev/client/pkg/kn/flags"
	servingclientlib "knative.dev/client/pkg/serving"
	"knative.dev/client/pkg/wait"
	"knative.dev/serving/pkg/apis/autoscaling"
	servingv1 "knative.dev/serving/pkg/apis/serving/v1"
	v1 "knative.dev/serving/pkg/apis/serving/v1"

	fn "github.com/boson-project/func"
	"github.com/boson-project/func/k8s"
)

type Deployer struct {
	// Namespace with which to override that set on the default configuration (such as the ~/.kube/config).
	// If left blank, deployment will commence to the configured namespace.
	Namespace string
	// Verbose logging enablement flag.
	Verbose bool
}

func NewDeployer(namespaceOverride string) (deployer *Deployer, err error) {
	deployer = &Deployer{}
	namespace, err := k8s.GetNamespace(namespaceOverride)
	if err != nil {
		return
	}
	deployer.Namespace = namespace
	return
}

func (d *Deployer) Deploy(ctx context.Context, f fn.Function) (result fn.DeploymentResult, err error) {

	client, err := NewServingClient(d.Namespace)
	if err != nil {
		return fn.DeploymentResult{}, err
	}

	_, err = client.GetService(ctx, f.Name)
	if err != nil {
		if errors.IsNotFound(err) {

			referencedSecrets := sets.NewString()
			referencedConfigMaps := sets.NewString()

			service, err := generateNewService(f.Name, f.ImageWithDigest(), f.Runtime, f.Envs, f.Volumes, f.Annotations, f.Options)
			if err != nil {
				err = fmt.Errorf("knative deployer failed to generate the Knative Service: %v", err)
				return fn.DeploymentResult{}, err
			}

			err = checkSecretsConfigMapsArePresent(ctx, d.Namespace, &referencedSecrets, &referencedConfigMaps)
			if err != nil {
				err = fmt.Errorf("knative deployer failed to generate the Knative Service: %v", err)
				return fn.DeploymentResult{}, err
			}

			err = client.CreateService(ctx, service)
			if err != nil {
				err = fmt.Errorf("knative deployer failed to deploy the Knative Service: %v", err)
				return fn.DeploymentResult{}, err
			}

			if d.Verbose {
				fmt.Println("Waiting for Knative Service to become ready")
			}
			err, _ = client.WaitForService(ctx, f.Name, DefaultWaitingTimeout, wait.NoopMessageCallback())
			if err != nil {
				err = fmt.Errorf("knative deployer failed to wait for the Knative Service to become ready: %v", err)
				return fn.DeploymentResult{}, err
			}

			route, err := client.GetRoute(ctx, f.Name)
			if err != nil {
				err = fmt.Errorf("knative deployer failed to get the Route: %v", err)
				return fn.DeploymentResult{}, err
			}

			fmt.Println("Function deployed at URL: " + route.Status.URL.String())
			return fn.DeploymentResult{
				Status: fn.Deployed,
				URL:    route.Status.URL.String(),
			}, nil

		} else {
			err = fmt.Errorf("knative deployer failed to get the Knative Service: %v", err)
			return fn.DeploymentResult{}, err
		}
	} else {
		// Update the existing Service
		referencedSecrets := sets.NewString()
		referencedConfigMaps := sets.NewString()

		newEnv, newEnvFrom, err := processEnvs(f.Envs, &referencedSecrets, &referencedConfigMaps)
		if err != nil {
			return fn.DeploymentResult{}, err
		}

		newVolumes, newVolumeMounts, err := processVolumes(f.Volumes, &referencedSecrets, &referencedConfigMaps)
		if err != nil {
			return fn.DeploymentResult{}, err
		}

		err = checkSecretsConfigMapsArePresent(ctx, d.Namespace, &referencedSecrets, &referencedConfigMaps)
		if err != nil {
			err = fmt.Errorf("knative deployer failed to update the Knative Service: %v", err)
			return fn.DeploymentResult{}, err
		}

		_, err = client.UpdateServiceWithRetry(ctx, f.Name, updateService(f.ImageWithDigest(), newEnv, newEnvFrom, newVolumes, newVolumeMounts, f.Annotations, f.Options), 3)
		if err != nil {
			err = fmt.Errorf("knative deployer failed to update the Knative Service: %v", err)
			return fn.DeploymentResult{}, err
		}

		route, err := client.GetRoute(ctx, f.Name)
		if err != nil {
			err = fmt.Errorf("knative deployer failed to get the Route: %v", err)
			return fn.DeploymentResult{}, err
		}

		return fn.DeploymentResult{
			Status: fn.Updated,
			URL:    route.Status.URL.String(),
		}, nil
	}
}

func probeFor(url string) *corev1.Probe {
	return &corev1.Probe{
		Handler: corev1.Handler{
			HTTPGet: &corev1.HTTPGetAction{
				Path: url,
			},
		},
	}
}

func generateNewService(name, image, runtime string, envs fn.Envs, volumes fn.Volumes, annotations map[string]string, options fn.Options) (*servingv1.Service, error) {
	containers := []corev1.Container{
		{
			Image: image,
		},
	}

	if runtime != "quarkus" {
		containers[0].LivenessProbe = probeFor("/health/liveness")
		containers[0].ReadinessProbe = probeFor("/health/readiness")
	}

	referencedSecrets := sets.NewString()
	referencedConfigMaps := sets.NewString()

	newEnv, newEnvFrom, err := processEnvs(envs, &referencedSecrets, &referencedConfigMaps)
	if err != nil {
		return nil, err
	}
	containers[0].Env = newEnv
	containers[0].EnvFrom = newEnvFrom

	newVolumes, newVolumeMounts, err := processVolumes(volumes, &referencedSecrets, &referencedConfigMaps)
	if err != nil {
		return nil, err
	}
	containers[0].VolumeMounts = newVolumeMounts

	service := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
			Labels: map[string]string{
				"boson.dev/function": "true",
				"boson.dev/runtime":  runtime,
			},
			Annotations: annotations,
		},
		Spec: v1.ServiceSpec{
			ConfigurationSpec: v1.ConfigurationSpec{
				Template: v1.RevisionTemplateSpec{
					Spec: v1.RevisionSpec{
						PodSpec: corev1.PodSpec{
							Containers: containers,
							Volumes:    newVolumes,
						},
					},
				},
			},
		},
	}

	err = setServiceOptions(&service.Spec.Template, options)
	if err != nil {
		return service, err
	}

	return service, nil
}

func updateService(image string, newEnv []corev1.EnvVar, newEnvFrom []corev1.EnvFromSource, newVolumes []corev1.Volume, newVolumeMounts []corev1.VolumeMount,
	annotations map[string]string, options fn.Options) func(service *servingv1.Service) (*servingv1.Service, error) {
	return func(service *servingv1.Service) (*servingv1.Service, error) {
		// Removing the name so the k8s server can fill it in with generated name,
		// this prevents conflicts in Revision name when updating the KService from multiple places.
		service.Spec.Template.Name = ""

		// Don't bother being as clever as we are with env variables
		// Just set the annotations to be whatever we find in func.yaml
		for k, v := range annotations {
			service.ObjectMeta.Annotations[k] = v
		}

		err := setServiceOptions(&service.Spec.Template, options)
		if err != nil {
			return service, err
		}

		err = flags.UpdateImage(&service.Spec.Template.Spec.PodSpec, image)
		if err != nil {
			return service, err
		}

		service.Spec.ConfigurationSpec.Template.Spec.Containers[0].Env = newEnv
		service.Spec.ConfigurationSpec.Template.Spec.Containers[0].EnvFrom = newEnvFrom

		service.Spec.ConfigurationSpec.Template.Spec.Containers[0].VolumeMounts = newVolumeMounts
		service.Spec.ConfigurationSpec.Template.Spec.Volumes = newVolumes

		return service, nil
	}
}

// processEnvs generates array of EnvVars and EnvFromSources from a function config
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
func processEnvs(envs fn.Envs, referencedSecrets, referencedConfigMaps *sets.String) ([]corev1.EnvVar, []corev1.EnvFromSource, error) {

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

func createEnvFromSource(value string, referencedSecrets, referencedConfigMaps *sets.String) (*corev1.EnvFromSource, error) {
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

func createEnvVarSource(slices []string, referencedSecrets, referencedConfigMaps *sets.String) (*corev1.EnvVarSource, error) {

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

/// processVolumes generates Volumes and VolumeMounts from a function config
// volumes:
// - secret: example-secret               # mount Secret as Volume
//   path: /etc/secret-volume
// - configMap: example-cm                # mount ConfigMap as Volume
//   path: /etc/cm-volume
func processVolumes(volumes fn.Volumes, referencedSecrets, referencedConfigMaps *sets.String) ([]corev1.Volume, []corev1.VolumeMount, error) {

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

// checkSecretsConfigMapsArePresent returns error if Secrets or ConfigMaps
// referenced in input sets are not deployed on the cluster in the specified namespace
func checkSecretsConfigMapsArePresent(ctx context.Context, namespace string, referencedSecrets, referencedConfigMaps *sets.String) error {

	errMsg := ""
	for s := range *referencedSecrets {
		_, err := k8s.GetSecret(ctx, s, namespace)
		if err != nil {
			errMsg += fmt.Sprintf("  referenced Secret \"%s\" is not present in namespace \"%s\"\n", s, namespace)
		}
	}

	for cm := range *referencedConfigMaps {
		_, err := k8s.GetConfigMap(ctx, cm, namespace)
		if err != nil {
			errMsg += fmt.Sprintf("  referenced ConfigMap \"%s\" is not present in namespace \"%s\"\n", cm, namespace)
		}
	}

	if errMsg != "" {
		return fmt.Errorf("\n" + errMsg)
	}

	return nil
}

// setServiceOptions sets annotations on Service Revision Template or in the Service Spec
// from values specifed in function configuration options
func setServiceOptions(template *servingv1.RevisionTemplateSpec, options fn.Options) error {

	toRemove := []string{}
	toUpdate := map[string]string{}

	if options.Scale != nil {
		if options.Scale.Min != nil {
			toUpdate[autoscaling.MinScaleAnnotationKey] = fmt.Sprintf("%d", *options.Scale.Min)
		} else {
			toRemove = append(toRemove, autoscaling.MinScaleAnnotationKey)
		}

		if options.Scale.Max != nil {
			toUpdate[autoscaling.MaxScaleAnnotationKey] = fmt.Sprintf("%d", *options.Scale.Max)
		} else {
			toRemove = append(toRemove, autoscaling.MaxScaleAnnotationKey)
		}

		if options.Scale.Metric != nil {
			toUpdate[autoscaling.MetricAnnotationKey] = *options.Scale.Metric
		} else {
			toRemove = append(toRemove, autoscaling.MetricAnnotationKey)
		}

		if options.Scale.Target != nil {
			toUpdate[autoscaling.TargetAnnotationKey] = fmt.Sprintf("%f", *options.Scale.Target)
		} else {
			toRemove = append(toRemove, autoscaling.TargetAnnotationKey)
		}

		if options.Scale.Utilization != nil {
			toUpdate[autoscaling.TargetUtilizationPercentageKey] = fmt.Sprintf("%f", *options.Scale.Utilization)
		} else {
			toRemove = append(toRemove, autoscaling.TargetUtilizationPercentageKey)
		}

	}

	// in the container always set Requests/Limits & Concurrency values based on the contents of config
	template.Spec.PodSpec.Containers[0].Resources.Requests = nil
	template.Spec.PodSpec.Containers[0].Resources.Limits = nil
	template.Spec.ContainerConcurrency = nil

	if options.Resources != nil {
		if options.Resources.Requests != nil {
			template.Spec.PodSpec.Containers[0].Resources.Requests = corev1.ResourceList{}

			if options.Resources.Requests.CPU != nil {
				value, err := resource.ParseQuantity(*options.Resources.Requests.CPU)
				if err != nil {
					return err
				}
				template.Spec.PodSpec.Containers[0].Resources.Requests[corev1.ResourceCPU] = value
			}

			if options.Resources.Requests.Memory != nil {
				value, err := resource.ParseQuantity(*options.Resources.Requests.Memory)
				if err != nil {
					return err
				}
				template.Spec.PodSpec.Containers[0].Resources.Requests[corev1.ResourceMemory] = value
			}
		}

		if options.Resources.Limits != nil {
			template.Spec.PodSpec.Containers[0].Resources.Limits = corev1.ResourceList{}

			if options.Resources.Limits.CPU != nil {
				value, err := resource.ParseQuantity(*options.Resources.Limits.CPU)
				if err != nil {
					return err
				}
				template.Spec.PodSpec.Containers[0].Resources.Limits[corev1.ResourceCPU] = value
			}

			if options.Resources.Limits.Memory != nil {
				value, err := resource.ParseQuantity(*options.Resources.Limits.Memory)
				if err != nil {
					return err
				}
				template.Spec.PodSpec.Containers[0].Resources.Limits[corev1.ResourceMemory] = value
			}

			if options.Resources.Limits.Concurrency != nil {
				template.Spec.ContainerConcurrency = options.Resources.Limits.Concurrency
			}
		}
	}

	return servingclientlib.UpdateRevisionTemplateAnnotations(template, toUpdate, toRemove)
}
