package k8s

import (
	"context"
	"crypto/sha256"
	"fmt"
	"maps"
	"os"
	"regexp"
	"sort"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/rand"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	clienteventingv1 "knative.dev/client/pkg/eventing/v1"
	eventingv1 "knative.dev/eventing/pkg/apis/eventing/v1"
	eventingv1client "knative.dev/eventing/pkg/client/clientset/versioned/typed/eventing/v1"
	"knative.dev/func/pkg/deployer"
	fn "knative.dev/func/pkg/functions"
	"knative.dev/pkg/apis"
	duckv1 "knative.dev/pkg/apis/duck/v1"
)

const (
	KubernetesDeployerName = "raw"

	DefaultLivenessEndpoint  = "/health/liveness"
	DefaultReadinessEndpoint = "/health/readiness"
	DefaultHTTPPort          = 8080

	// managedByAnnotation identifies triggers managed by this deployer
	managedByAnnotation = "func.knative.dev/managed-by"
	managedByValue      = "func-raw-deployer"
)

type DeployerOpt func(*Deployer)

type Deployer struct {
	verbose   bool
	decorator deployer.DeployDecorator
}

func NewDeployer(opts ...DeployerOpt) *Deployer {
	d := &Deployer{}
	for _, opt := range opts {
		opt(d)
	}
	return d
}

func WithDeployerVerbose(verbose bool) DeployerOpt {
	return func(d *Deployer) {
		d.verbose = verbose
	}
}

func WithDeployerDecorator(decorator deployer.DeployDecorator) DeployerOpt {
	return func(d *Deployer) {
		d.decorator = decorator
	}
}

func onClusterFix(f fn.Function) fn.Function {
	// This only exists because of a bootstapping problem with On-Cluster
	// builds:  It appears that, when sending a function to be built on-cluster
	// the target namespace is not being transmitted in the pipeline
	// configuration.  We should figure out how to transmit this information
	// to the pipeline run for initial builds.  This is a new problem because
	// earlier versions of this logic relied entirely on the current
	// kubernetes context.
	if f.Namespace == "" && f.Deploy.Namespace == "" {
		f.Namespace, _ = GetDefaultNamespace()
	}
	return f
}

// newEventingClient creates a Knative Eventing client from a REST config
func newEventingClient(config *rest.Config, namespace string) (clienteventingv1.KnEventingClient, error) {
	eventingClient, err := eventingv1client.NewForConfig(config)
	if err != nil {
		return nil, err
	}
	return clienteventingv1.NewKnEventingClient(eventingClient, namespace), nil
}

func (d *Deployer) Deploy(ctx context.Context, f fn.Function) (fn.DeploymentResult, error) {
	f = onClusterFix(f)
	// Choosing f.Namespace vs f.Deploy.Namespace:
	// This is minimal logic currently required of all deployer impls.
	// If f.Namespace is defined, this is the (possibly new) target
	// namespace.  Otherwise use the last deployed namespace.  Error if
	// neither are set.  The logic which arbitrates between curret k8s context,
	// flags, environment variables and global defaults to determine the
	// effective namespace is not logic for the deployer implementation, which
	// should have a minimum of logic.  In this case limited to "new ns or
	// existing namespace?
	namespace := f.Namespace
	if namespace == "" {
		namespace = f.Deploy.Namespace
	}
	if namespace == "" {
		return fn.DeploymentResult{}, fmt.Errorf("deployer requires either a target namespace or that the function be already deployed")
	}

	// Choosing an image to deploy:
	// If the service has not been deployed before, but there exists a
	// build image, this build image should be used for the deploy.
	// TODO: test/consider the case where it HAS been deployed, and the
	// build image has been updated /since/ deployment:  do we need a
	// timestamp? Incrementation?
	if f.Deploy.Image == "" {
		f.Deploy.Image = f.Build.Image
	}

	// Get the Kubernetes REST config
	config, err := GetClientConfig().ClientConfig()
	if err != nil {
		return fn.DeploymentResult{}, err
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return fn.DeploymentResult{}, err
	}

	// Check if Dapr is installed
	daprInstalled := false
	_, err = clientset.CoreV1().Namespaces().Get(ctx, "dapr-system", metav1.GetOptions{})
	if err == nil {
		daprInstalled = true
	}

	deploymentClient := clientset.AppsV1().Deployments(namespace)
	serviceClient := clientset.CoreV1().Services(namespace)

	existingDeployment, err := deploymentClient.Get(ctx, f.Name, metav1.GetOptions{})

	var status fn.Status
	if err == nil {
		// Update the existing function
		deployment, err := d.generateDeployment(f, namespace, daprInstalled)
		if err != nil {
			return fn.DeploymentResult{}, fmt.Errorf("failed to generate deployment resources: %w", err)
		}

		svc, err := d.generateService(f, namespace, daprInstalled, existingDeployment)
		if err != nil {
			return fn.DeploymentResult{}, fmt.Errorf("failed to generate service resources: %w", err)
		}

		// Preserve resource version for update
		deployment.ResourceVersion = existingDeployment.ResourceVersion

		if _, err = deploymentClient.Update(ctx, deployment, metav1.UpdateOptions{}); err != nil {
			return fn.DeploymentResult{}, fmt.Errorf("failed to update deployment: %w", err)
		}

		existingService, err := serviceClient.Get(ctx, f.Name, metav1.GetOptions{})
		if err == nil {
			svc.ResourceVersion = existingService.ResourceVersion
			if _, err = serviceClient.Update(ctx, svc, metav1.UpdateOptions{}); err != nil {
				return fn.DeploymentResult{}, fmt.Errorf("failed to update service: %w", err)
			}
		} else if errors.IsNotFound(err) {
			// Service doesn't exist, create it
			if _, err = serviceClient.Create(ctx, svc, metav1.CreateOptions{}); err != nil {
				return fn.DeploymentResult{}, fmt.Errorf("failed to create service: %w", err)
			}
		} else {
			return fn.DeploymentResult{}, fmt.Errorf("failed to get existing service: %w", err)
		}

		status = fn.Updated
		if d.verbose {
			fmt.Fprintf(os.Stderr, "Updated deployment and service %s in namespace %s\n", f.Name, namespace)
		}
	} else {
		if !errors.IsNotFound(err) {
			return fn.DeploymentResult{}, fmt.Errorf("failed to check for existing deployment: %w", err)
		}

		deployment, err := d.generateDeployment(f, namespace, daprInstalled)
		if err != nil {
			return fn.DeploymentResult{}, fmt.Errorf("failed to generate deployment resources: %w", err)
		}

		deployment, err = deploymentClient.Create(ctx, deployment, metav1.CreateOptions{})
		if err != nil {
			return fn.DeploymentResult{}, fmt.Errorf("failed to create deployment: %w", err)
		}

		svc, err := d.generateService(f, namespace, daprInstalled, deployment)
		if err != nil {
			return fn.DeploymentResult{}, fmt.Errorf("failed to generate service resources: %w", err)
		}

		if _, err = serviceClient.Create(ctx, svc, metav1.CreateOptions{}); err != nil {
			return fn.DeploymentResult{}, fmt.Errorf("failed to create service: %w", err)
		}

		status = fn.Deployed
		if d.verbose {
			fmt.Fprintf(os.Stderr, "Created deployment and service %s in namespace %s\n", f.Name, namespace)
		}
	}

	if err := WaitForDeploymentAvailable(ctx, clientset, namespace, f.Name, DefaultWaitingTimeout); err != nil {
		return fn.DeploymentResult{}, fmt.Errorf("deployment did not become ready: %w", err)
	}

	// Sync triggers
	eventingClient, err := newEventingClient(config, namespace)
	if err != nil {
		return fn.DeploymentResult{}, fmt.Errorf("failed to create eventing client: %w", err)
	}
	if err := syncTriggers(ctx, f, namespace, eventingClient, clientset); err != nil {
		return fn.DeploymentResult{}, fmt.Errorf("failed to sync triggers: %w", err)
	}

	url := fmt.Sprintf("http://%s.%s.svc", f.Name, namespace)

	return fn.DeploymentResult{
		Status:    status,
		URL:       url,
		Namespace: namespace,
	}, nil
}

// generateTriggerName creates a deterministic trigger name based on subscription content
func generateTriggerName(functionName, broker string, filters map[string]string) string {
	filterKeys := make([]string, 0, len(filters))
	for k := range filters {
		filterKeys = append(filterKeys, k)
	}
	sort.Strings(filterKeys)

	parts := make([]string, 0, 1+len(filters))
	parts = append(parts, broker)
	for _, k := range filterKeys {
		parts = append(parts, fmt.Sprintf("%s=%s", k, filters[k]))
	}

	hash := sha256.Sum256([]byte(strings.Join(parts, "|")))
	hashStr := fmt.Sprintf("%x", hash[:4])

	return fmt.Sprintf("%s-trigger-%s", functionName, hashStr)
}

func syncTriggers(ctx context.Context, f fn.Function, namespace string, eventingClient clienteventingv1.KnEventingClient, clientset kubernetes.Interface) error {
	// Build set of desired trigger names from current subscriptions
	desiredTriggers := sets.New[string]()
	for _, sub := range f.Deploy.Subscriptions {
		triggerName := generateTriggerName(f.Name, sub.Source, sub.Filters)
		desiredTriggers.Insert(triggerName)
	}

	// Create or update triggers from current subscriptions
	if len(f.Deploy.Subscriptions) > 0 {
		svc, err := clientset.CoreV1().Services(namespace).Get(ctx, f.Name, metav1.GetOptions{})
		if err != nil {
			return fmt.Errorf("failed to get service: %w", err)
		}

		deployment, err := clientset.AppsV1().Deployments(namespace).Get(ctx, f.Name, metav1.GetOptions{})
		if err != nil {
			return fmt.Errorf("failed to get deployment: %w", err)
		}

		fmt.Fprintf(os.Stderr, "🎯 Syncing Triggers on the cluster\n")

		for _, sub := range f.Deploy.Subscriptions {
			attributes := make(map[string]string)
			maps.Copy(attributes, sub.Filters)

			triggerName := generateTriggerName(f.Name, sub.Source, sub.Filters)

			trigger := &eventingv1.Trigger{
				ObjectMeta: metav1.ObjectMeta{
					Name: triggerName,
					Annotations: map[string]string{
						managedByAnnotation: managedByValue,
					},
					OwnerReferences: []metav1.OwnerReference{
						{
							APIVersion: "apps/v1",
							Kind:       "Deployment",
							Name:       deployment.Name,
							UID:        deployment.UID,
						},
					},
				},
				Spec: eventingv1.TriggerSpec{
					Broker: sub.Source,
					Subscriber: duckv1.Destination{
						URI: &apis.URL{
							Scheme: "http",
							Host:   fmt.Sprintf("%s.%s.svc.cluster.local", svc.Name, namespace),
						},
					},
					Filter: &eventingv1.TriggerFilter{
						Attributes: attributes,
					},
				},
			}

			err := eventingClient.CreateTrigger(ctx, trigger)
			if err != nil && !errors.IsAlreadyExists(err) {
				return fmt.Errorf("failed to create trigger: %w", err)
			}
		}
	}

	// Clean up stale triggers
	return deleteStaleTriggers(ctx, eventingClient, f.Name, desiredTriggers)
}

// deleteStaleTriggers removes triggers managed by this deployer that are no longer in the desired set
func deleteStaleTriggers(ctx context.Context, eventingClient clienteventingv1.KnEventingClient, functionName string, desiredTriggers sets.Set[string]) error {
	// List existing triggers in the namespace
	existingTriggers, err := eventingClient.ListTriggers(ctx)
	if err != nil {
		if strings.HasPrefix(err.Error(), "no or newer Knative Eventing API found on the backend") {
			// knative eventing not installed -> nothing to do and return early
			return nil
		}

		// If triggers can't be listed ,skip cleanup
		if errors.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("failed to list triggers: %w", err)
	}

	// Delete stale triggers (only those belonging to this function)
	triggerPrefix := functionName + "-trigger-"
	for _, trigger := range existingTriggers.Items {
		if !strings.HasPrefix(trigger.Name, triggerPrefix) {
			continue
		}

		// Only delete triggers we manage
		if trigger.Annotations[managedByAnnotation] == managedByValue {
			// Check if this trigger is still desired
			if !desiredTriggers.Has(trigger.Name) {
				fmt.Fprintf(os.Stderr, "🗑️  Deleting stale trigger: %s\n", trigger.Name)
				err := eventingClient.DeleteTrigger(ctx, trigger.Name)
				if err != nil && !errors.IsNotFound(err) {
					return fmt.Errorf("failed to delete stale trigger %s: %w", trigger.Name, err)
				}
			}
		}
	}

	return nil
}

func (d *Deployer) generateDeployment(f fn.Function, namespace string, daprInstalled bool) (*appsv1.Deployment, error) {
	labels, err := deployer.GenerateCommonLabels(f, d.decorator)
	if err != nil {
		return nil, err
	}

	annotations := deployer.GenerateCommonAnnotations(f, d.decorator, daprInstalled, KubernetesDeployerName)

	// Use annotations for pod template
	podAnnotations := make(map[string]string)
	maps.Copy(podAnnotations, annotations)

	// Process environment variables and volumes
	referencedSecrets := sets.New[string]()
	referencedConfigMaps := sets.New[string]()
	referencedPVCs := sets.New[string]()

	envVars, envFrom, err := ProcessEnvs(f.Run.Envs, &referencedSecrets, &referencedConfigMaps)
	if err != nil {
		return nil, fmt.Errorf("failed to process environment variables: %w", err)
	}

	volumes, volumeMounts, err := ProcessVolumes(f.Run.Volumes, &referencedSecrets, &referencedConfigMaps, &referencedPVCs)
	if err != nil {
		return nil, fmt.Errorf("failed to process volumes: %w", err)
	}

	container := corev1.Container{
		Name:  "user-container",
		Image: f.Deploy.Image,
		Ports: []corev1.ContainerPort{
			{
				ContainerPort: DefaultHTTPPort,
				Protocol:      corev1.ProtocolTCP,
			},
		},
		Env:          envVars,
		EnvFrom:      envFrom,
		VolumeMounts: volumeMounts,
	}

	SetHealthEndpoints(f, &container)
	SetSecurityContext(&container)

	replicas := int32(1)
	if f.Deploy.Options.Scale != nil && f.Deploy.Options.Scale.Min != nil && *f.Deploy.Options.Scale.Min > 0 {
		replicas = int32(*f.Deploy.Options.Scale.Min)
	}

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:        f.Name,
			Namespace:   namespace,
			Labels:      labels,
			Annotations: annotations,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels:      labels,
					Annotations: podAnnotations,
				},
				Spec: corev1.PodSpec{
					Containers:         []corev1.Container{container},
					ServiceAccountName: f.Deploy.ServiceAccountName,
					Volumes:            volumes,
				},
			},
		},
	}

	return deployment, nil
}

func (d *Deployer) generateService(f fn.Function, namespace string, daprInstalled bool, deployment *appsv1.Deployment) (*corev1.Service, error) {
	labels, err := deployer.GenerateCommonLabels(f, d.decorator)
	if err != nil {
		return nil, err
	}

	annotations := deployer.GenerateCommonAnnotations(f, d.decorator, daprInstalled, KubernetesDeployerName)

	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:        f.Name,
			Namespace:   namespace,
			Labels:      labels,
			Annotations: annotations,
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(deployment, appsv1.SchemeGroupVersion.WithKind("Deployment")),
			},
		},
		Spec: corev1.ServiceSpec{
			Type:     corev1.ServiceTypeClusterIP,
			Selector: labels,
			Ports: []corev1.ServicePort{
				{
					Name:       "http",
					Port:       80,
					TargetPort: intstr.FromInt32(DefaultHTTPPort),
					Protocol:   corev1.ProtocolTCP,
				},
			},
		},
	}

	return service, nil
}

// CheckResourcesArePresent returns error if Secrets or ConfigMaps
// referenced in input sets are not deployed on the cluster in the specified namespace
func CheckResourcesArePresent(ctx context.Context, namespace string, referencedSecrets, referencedConfigMaps, referencedPVCs *sets.Set[string], referencedServiceAccount string) error {
	errMsg := ""
	for s := range *referencedSecrets {
		_, err := GetSecret(ctx, s, namespace)
		if err != nil {
			if errors.IsForbidden(err) {
				errMsg += " Ensure that the service account has the necessary permissions to access the secret.\n"
			} else {
				errMsg += fmt.Sprintf("  referenced Secret \"%s\" is not present in namespace \"%s\"\n", s, namespace)
			}
		}
	}

	for cm := range *referencedConfigMaps {
		_, err := GetConfigMap(ctx, cm, namespace)
		if err != nil {
			errMsg += fmt.Sprintf("  referenced ConfigMap \"%s\" is not present in namespace \"%s\"\n", cm, namespace)
		}
	}

	for pvc := range *referencedPVCs {
		_, err := GetPersistentVolumeClaim(ctx, pvc, namespace)
		if err != nil {
			errMsg += fmt.Sprintf("  referenced PersistentVolumeClaim \"%s\" is not present in namespace \"%s\"\n", pvc, namespace)
		}
	}

	// check if referenced ServiceAccount is present in the namespace if it is not default
	if referencedServiceAccount != "" && referencedServiceAccount != "default" {
		err := GetServiceAccount(ctx, referencedServiceAccount, namespace)
		if err != nil {
			errMsg += fmt.Sprintf("  referenced ServiceAccount \"%s\" is not present in namespace \"%s\"\n", referencedServiceAccount, namespace)
		}
	}

	if errMsg != "" {
		return fmt.Errorf("error(s) while validating resources:\n%s", errMsg)
	}

	return nil
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

func UsesRawDeployer(annotations map[string]string) bool {
	deployer, ok := annotations[deployer.DeployerNameAnnotation]

	return ok && deployer == KubernetesDeployerName
}
