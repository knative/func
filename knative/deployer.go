package knative

import (
	"context"
	"fmt"
	"io"
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
	clientservingv1 "knative.dev/client/pkg/serving/v1"
	"knative.dev/client/pkg/wait"
	"knative.dev/serving/pkg/apis/autoscaling"
	v1 "knative.dev/serving/pkg/apis/serving/v1"

	fn "knative.dev/func"
	"knative.dev/func/k8s"
)

const LIVENESS_ENDPOINT = "/health/liveness"
const READINESS_ENDPOINT = "/health/readiness"

type DeployDecorator interface {
	UpdateAnnotations(fn.Function, map[string]string) map[string]string
	UpdateLabels(fn.Function, map[string]string) map[string]string
}

type DeployerOpt func(*Deployer)

type Deployer struct {
	// Namespace with which to override that set on the default configuration (such as the ~/.kube/config).
	// If left blank, deployment will commence to the configured namespace.
	Namespace string
	// verbose logging enablement flag.
	verbose bool

	decorator DeployDecorator
}

// DefaultNamespace attempts to read the kubernetes active namepsace.
// Missing configs or not having an active kuberentes configuration are
// equivalent to having no default namespace (empty string).
func DefaultNamespace() string {
	// Get client config, if it exists, and from that the namespace
	ns, _, err := k8s.GetClientConfig().Namespace()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: unable to get active namespace: %v\n", err)
	}
	return ns
}

func NewDeployer(opts ...DeployerOpt) *Deployer {
	d := &Deployer{}

	for _, opt := range opts {
		opt(d)
	}

	return d
}

func WithDeployerNamespace(namespace string) DeployerOpt {
	return func(d *Deployer) {
		d.Namespace = namespace
	}
}

func WithDeployerVerbose(verbose bool) DeployerOpt {
	return func(d *Deployer) {
		d.verbose = verbose
	}
}

func WithDeployerDecorator(decorator DeployDecorator) DeployerOpt {
	return func(d *Deployer) {
		d.decorator = decorator
	}
}

// Checks the status of the "user-container" for the ImagePullBackOff reason meaning that
// the container image is not reachable probably because a private registry is being used.
func (d *Deployer) isImageInPrivateRegistry(ctx context.Context, client clientservingv1.KnServingClient, funcName string) bool {
	ksvc, err := client.GetService(ctx, funcName)
	if err != nil {
		return false
	}
	k8sClient, err := k8s.NewKubernetesClientset()
	if err != nil {
		return false
	}
	list, err := k8sClient.CoreV1().Pods(d.Namespace).List(ctx, metav1.ListOptions{
		LabelSelector: "serving.knative.dev/revision=" + ksvc.Status.LatestCreatedRevisionName + ",serving.knative.dev/service=" + funcName,
		FieldSelector: "status.phase=Pending",
	})
	if err != nil {
		return false
	}
	if len(list.Items) != 1 {
		return false
	}

	for _, cont := range list.Items[0].Status.ContainerStatuses {
		if cont.Name == "user-container" {
			return cont.State.Waiting != nil && cont.State.Waiting.Reason == "ImagePullBackOff"
		}
	}
	return false
}

func (d *Deployer) Deploy(ctx context.Context, f fn.Function) (fn.DeploymentResult, error) {
	var err error
	if d.Namespace == "" {
		d.Namespace, err = k8s.GetNamespace(d.Namespace)
		if err != nil {
			return fn.DeploymentResult{}, err
		}
	}

	client, err := NewServingClient(d.Namespace)
	if err != nil {
		return fn.DeploymentResult{}, err
	}

	var outBuff SynchronizedBuffer
	var out io.Writer = &outBuff

	if d.verbose {
		out = os.Stderr
	}
	since := time.Now()
	go func() {
		_ = GetKServiceLogs(ctx, d.Namespace, f.Name, f.ImageWithDigest(), &since, out)
	}()

	_, err = client.GetService(ctx, f.Name)
	if err != nil {
		if errors.IsNotFound(err) {

			referencedSecrets := sets.NewString()
			referencedConfigMaps := sets.NewString()

			service, err := generateNewService(f, d.decorator)
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

			if d.verbose {
				fmt.Println("Waiting for Knative Service to become ready")
			}
			chprivate := make(chan bool)
			cherr := make(chan error)
			go func() {
				private := false
				for !private {
					time.Sleep(5 * time.Second)
					private = d.isImageInPrivateRegistry(ctx, client, f.Name)
					chprivate <- private
				}
				close(chprivate)
			}()
			go func() {
				err, _ := client.WaitForService(ctx, f.Name,
					clientservingv1.WaitConfig{Timeout: DefaultWaitingTimeout, ErrorWindow: DefaultErrorWindowTimeout},
					wait.NoopMessageCallback())
				cherr <- err
				close(cherr)
			}()

			presumePrivate := false
		main:
			// Wait for either a timeout or a container condition signaling the image is unreachable
			for {
				select {
				case private := <-chprivate:
					if private {
						presumePrivate = true
						break main
					}
				case err = <-cherr:
					break main
				}
			}
			if presumePrivate {
				err := fmt.Errorf("your function image is unreachable. It is possible that your docker registry is private. If so, make sure you have set up pull secrets https://knative.dev/docs/developer/serving/deploying-from-private-registry")
				return fn.DeploymentResult{}, err
			}
			if err != nil {
				err = fmt.Errorf("knative deployer failed to wait for the Knative Service to become ready: %v", err)
				if !d.verbose {
					fmt.Fprintln(os.Stderr, "\nService output:")
					_, _ = io.Copy(os.Stderr, &outBuff)
					fmt.Fprintln(os.Stderr)
				}
				return fn.DeploymentResult{}, err
			}

			route, err := client.GetRoute(ctx, f.Name)
			if err != nil {
				err = fmt.Errorf("knative deployer failed to get the Route: %v", err)
				return fn.DeploymentResult{}, err
			}

			if d.verbose {
				fmt.Printf("Function deployed in namespace %q and exposed at URL:\n%s\n", d.Namespace, route.Status.URL.String())
			}
			return fn.DeploymentResult{
				Status:    fn.Deployed,
				URL:       route.Status.URL.String(),
				Namespace: d.Namespace,
			}, nil

		} else {
			err = fmt.Errorf("knative deployer failed to get the Knative Service: %v", err)
			return fn.DeploymentResult{}, err
		}
	} else {
		// Update the existing Service
		referencedSecrets := sets.NewString()
		referencedConfigMaps := sets.NewString()

		newEnv, newEnvFrom, err := processEnvs(f.Run.Envs, &referencedSecrets, &referencedConfigMaps)
		if err != nil {
			return fn.DeploymentResult{}, err
		}

		newVolumes, newVolumeMounts, err := processVolumes(f.Run.Volumes, &referencedSecrets, &referencedConfigMaps)
		if err != nil {
			return fn.DeploymentResult{}, err
		}

		err = checkSecretsConfigMapsArePresent(ctx, d.Namespace, &referencedSecrets, &referencedConfigMaps)
		if err != nil {
			err = fmt.Errorf("knative deployer failed to update the Knative Service: %v", err)
			return fn.DeploymentResult{}, err
		}

		_, err = client.UpdateServiceWithRetry(ctx, f.Name, updateService(f, newEnv, newEnvFrom, newVolumes, newVolumeMounts, d.decorator), 3)
		if err != nil {
			err = fmt.Errorf("knative deployer failed to update the Knative Service: %v", err)
			return fn.DeploymentResult{}, err
		}

		err, _ = client.WaitForService(ctx, f.Name,
			clientservingv1.WaitConfig{Timeout: DefaultWaitingTimeout, ErrorWindow: DefaultErrorWindowTimeout},
			wait.NoopMessageCallback())
		if err != nil {
			if !d.verbose {
				fmt.Fprintln(os.Stderr, "\nService output:")
				_, _ = io.Copy(os.Stderr, &outBuff)
				fmt.Fprintln(os.Stderr)
			}
			return fn.DeploymentResult{}, err
		}

		route, err := client.GetRoute(ctx, f.Name)
		if err != nil {
			err = fmt.Errorf("knative deployer failed to get the Route: %v", err)
			return fn.DeploymentResult{}, err
		}

		return fn.DeploymentResult{
			Status:    fn.Updated,
			URL:       route.Status.URL.String(),
			Namespace: d.Namespace,
		}, nil
	}
}

func probeFor(url string) *corev1.Probe {
	return &corev1.Probe{
		ProbeHandler: corev1.ProbeHandler{
			HTTPGet: &corev1.HTTPGetAction{
				Path: url,
			},
		},
	}
}

func setHealthEndpoints(f fn.Function, c *corev1.Container) *corev1.Container {
	// Set the defaults
	c.LivenessProbe = probeFor(LIVENESS_ENDPOINT)
	c.ReadinessProbe = probeFor(READINESS_ENDPOINT)

	// If specified in func.yaml, the provided values override the defaults
	if f.Deploy.HealthEndpoints.Liveness != "" {
		c.LivenessProbe = probeFor(f.Deploy.HealthEndpoints.Liveness)
	}
	if f.Deploy.HealthEndpoints.Readiness != "" {
		c.ReadinessProbe = probeFor(f.Deploy.HealthEndpoints.Readiness)
	}
	return c
}

func generateNewService(f fn.Function, decorator DeployDecorator) (*v1.Service, error) {
	container := corev1.Container{
		Image: f.ImageWithDigest(),
	}
	setHealthEndpoints(f, &container)

	referencedSecrets := sets.NewString()
	referencedConfigMaps := sets.NewString()

	newEnv, newEnvFrom, err := processEnvs(f.Run.Envs, &referencedSecrets, &referencedConfigMaps)
	if err != nil {
		return nil, err
	}
	container.Env = newEnv
	container.EnvFrom = newEnvFrom

	newVolumes, newVolumeMounts, err := processVolumes(f.Run.Volumes, &referencedSecrets, &referencedConfigMaps)
	if err != nil {
		return nil, err
	}
	container.VolumeMounts = newVolumeMounts

	labels, err := f.LabelsMap()
	if err != nil {
		return nil, err
	}
	if decorator != nil {
		labels = decorator.UpdateLabels(f, labels)
	}

	annotations := f.Deploy.Annotations
	if decorator != nil {
		annotations = decorator.UpdateAnnotations(f, annotations)
	}

	// we need to create a separate map for Annotations specified in a Revision,
	// in case we will need to specify autoscaling annotations -> these could be only in a Revision not in a Service
	revisionAnnotations := make(map[string]string)
	for k, v := range annotations {
		revisionAnnotations[k] = v
	}

	service := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:        f.Name,
			Labels:      labels,
			Annotations: annotations,
		},
		Spec: v1.ServiceSpec{
			ConfigurationSpec: v1.ConfigurationSpec{
				Template: v1.RevisionTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels:      labels,
						Annotations: revisionAnnotations,
					},
					Spec: v1.RevisionSpec{
						PodSpec: corev1.PodSpec{
							Containers: []corev1.Container{
								container,
							},
							Volumes: newVolumes,
						},
					},
				},
			},
		},
	}

	err = setServiceOptions(&service.Spec.Template, f.Deploy.Options)
	if err != nil {
		return service, err
	}

	return service, nil
}

func updateService(f fn.Function, newEnv []corev1.EnvVar, newEnvFrom []corev1.EnvFromSource, newVolumes []corev1.Volume, newVolumeMounts []corev1.VolumeMount, decorator DeployDecorator) func(service *v1.Service) (*v1.Service, error) {
	return func(service *v1.Service) (*v1.Service, error) {
		// Removing the name so the k8s server can fill it in with generated name,
		// this prevents conflicts in Revision name when updating the KService from multiple places.
		service.Spec.Template.Name = ""

		// Don't bother being as clever as we are with env variables
		// Just set the annotations and labels to be whatever we find in func.yaml
		if decorator != nil {
			service.ObjectMeta.Annotations = decorator.UpdateAnnotations(f, service.ObjectMeta.Annotations)
		}

		for k, v := range f.Deploy.Annotations {
			service.ObjectMeta.Annotations[k] = v
			service.Spec.Template.ObjectMeta.Annotations[k] = v
		}
		// I hate that we have to do this. Users should not see these values.
		// It is an implementation detail. These health endpoints should not be
		// a part of func.yaml since the user can only mess things up by changing
		// them. Ultimately, this information is determined by the language pack.
		// Which is another reason to consider having a global config to store
		// some metadata which is fairly static. For example, a .config/func/global.yaml
		// file could contain information about all known language packs. As new
		// language packs are discovered through use of the --repository flag when
		// creating a function, this information could be extracted from
		// language-pack.yaml for each template and written to the local global
		// config. At runtime this configuration file could be consulted. I don't
		// know what this would mean for developers using the func library directly.
		cp := &service.Spec.Template.Spec.Containers[0]
		setHealthEndpoints(f, cp)

		err := setServiceOptions(&service.Spec.Template, f.Deploy.Options)
		if err != nil {
			return service, err
		}

		labels, err := f.LabelsMap()
		if err != nil {
			return nil, err
		}
		if decorator != nil {
			labels = decorator.UpdateLabels(f, labels)
		}

		service.ObjectMeta.Labels = labels
		service.Spec.Template.ObjectMeta.Labels = labels

		err = flags.UpdateImage(&service.Spec.Template.Spec.PodSpec, f.ImageWithDigest())
		if err != nil {
			return service, err
		}

		cp.Env = newEnv
		cp.EnvFrom = newEnvFrom
		cp.VolumeMounts = newVolumeMounts
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
func processEnvs(envs []fn.Env, referencedSecrets, referencedConfigMaps *sets.String) ([]corev1.EnvVar, []corev1.EnvFromSource, error) {

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

// / processVolumes generates Volumes and VolumeMounts from a function config
// volumes:
//   - secret: example-secret               # mount Secret as Volume
//     path: /etc/secret-volume
//   - configMap: example-cm                # mount ConfigMap as Volume
//     path: /etc/cm-volume
func processVolumes(volumes []fn.Volume, referencedSecrets, referencedConfigMaps *sets.String) ([]corev1.Volume, []corev1.VolumeMount, error) {

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
func setServiceOptions(template *v1.RevisionTemplateSpec, options fn.Options) error {

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
