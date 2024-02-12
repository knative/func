package knative

import (
	"context"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
	"time"

	clienteventingv1 "knative.dev/client-pkg/pkg/eventing/v1"
	eventingv1 "knative.dev/eventing/pkg/apis/eventing/v1"
	duckv1 "knative.dev/pkg/apis/duck/v1"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/rand"
	"k8s.io/apimachinery/pkg/util/sets"
	"knative.dev/client-pkg/pkg/kn/flags"
	servingclientlib "knative.dev/client-pkg/pkg/serving"
	clientservingv1 "knative.dev/client-pkg/pkg/serving/v1"
	"knative.dev/client-pkg/pkg/wait"
	"knative.dev/serving/pkg/apis/autoscaling"
	v1 "knative.dev/serving/pkg/apis/serving/v1"

	fn "knative.dev/func/pkg/functions"
	"knative.dev/func/pkg/k8s"
)

const LIVENESS_ENDPOINT = "/health/liveness"
const READINESS_ENDPOINT = "/health/readiness"

// static default namespace for deployer
const StaticDefaultNamespace = "func"

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

// ActiveNamespace attempts to read the kubernetes active namepsace.
// Missing configs or not having an active kuberentes configuration are
// equivalent to having no default namespace (empty string).
func ActiveNamespace() string {
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
func (d *Deployer) isImageInPrivateRegistry(ctx context.Context, client clientservingv1.KnServingClient, f fn.Function) bool {
	ksvc, err := client.GetService(ctx, f.Name)
	if err != nil {
		return false
	}
	k8sClient, err := k8s.NewKubernetesClientset()
	if err != nil {
		return false
	}
	list, err := k8sClient.CoreV1().Pods(namespace(d.Namespace, f)).List(ctx, metav1.ListOptions{
		LabelSelector: "serving.knative.dev/revision=" + ksvc.Status.LatestCreatedRevisionName + ",serving.knative.dev/service=" + f.Name,
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

// returns correct namespace to deploy to, ordered in a descending order by
// priority: User specified via cli -> client WithDeployer -> already deployed ->
// -> k8s default; if fails, use static default
func namespace(dflt string, f fn.Function) string {
	// namespace ordered by highest priority decending
	namespace := f.Namespace

	// if deployed before: use already deployed namespace
	if namespace == "" {
		namespace = f.Deploy.Namespace
	}

	// deployer WithDeployerNamespace provided
	if namespace == "" {
		namespace = dflt
	}

	if namespace == "" {
		var err error
		// still not set, just use the defaultest default
		namespace, err = k8s.GetDefaultNamespace()
		if err != nil {
			fmt.Fprintf(os.Stderr, "trying to get default namespace returns an error: '%s'\nSetting static default namespace '%s'", err, StaticDefaultNamespace)
			namespace = StaticDefaultNamespace
		}
	}
	return namespace
}

func (d *Deployer) Deploy(ctx context.Context, f fn.Function) (fn.DeploymentResult, error) {

	// returns correct namespace by priority
	namespace := namespace(d.Namespace, f)

	client, err := NewServingClient(namespace)
	if err != nil {
		return fn.DeploymentResult{}, err
	}
	eventingClient, err := NewEventingClient(namespace)
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
		_ = GetKServiceLogs(ctx, namespace, f.Name, f.Deploy.Image, &since, out)
	}()

	previousService, err := client.GetService(ctx, f.Name)
	if err != nil {
		if errors.IsNotFound(err) {

			referencedSecrets := sets.New[string]()
			referencedConfigMaps := sets.New[string]()
			referencedPVCs := sets.New[string]()

			service, err := generateNewService(f, d.decorator)
			if err != nil {
				err = fmt.Errorf("knative deployer failed to generate the Knative Service: %v", err)
				return fn.DeploymentResult{}, err
			}

			err = checkResourcesArePresent(ctx, namespace, &referencedSecrets, &referencedConfigMaps, &referencedPVCs, f.Deploy.ServiceAccountName)
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
					private = d.isImageInPrivateRegistry(ctx, client, f)
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

			err = createTriggers(ctx, f, client, eventingClient)
			if err != nil {
				return fn.DeploymentResult{}, err
			}

			if d.verbose {
				fmt.Printf("Function deployed in namespace %q and exposed at URL:\n%s\n", namespace, route.Status.URL.String())
			}
			return fn.DeploymentResult{
				Status:    fn.Deployed,
				URL:       route.Status.URL.String(),
				Namespace: namespace,
			}, nil

		} else {
			err = fmt.Errorf("knative deployer failed to get the Knative Service: %v", err)
			return fn.DeploymentResult{}, err
		}
	} else {
		// Update the existing Service
		referencedSecrets := sets.New[string]()
		referencedConfigMaps := sets.New[string]()
		referencedPVCs := sets.New[string]()

		newEnv, newEnvFrom, err := processEnvs(f.Run.Envs, &referencedSecrets, &referencedConfigMaps)
		if err != nil {
			return fn.DeploymentResult{}, err
		}

		newVolumes, newVolumeMounts, err := processVolumes(f.Run.Volumes, &referencedSecrets, &referencedConfigMaps, &referencedPVCs)
		if err != nil {
			return fn.DeploymentResult{}, err
		}

		err = checkResourcesArePresent(ctx, namespace, &referencedSecrets, &referencedConfigMaps, &referencedPVCs, f.Deploy.ServiceAccountName)
		if err != nil {
			err = fmt.Errorf("knative deployer failed to update the Knative Service: %v", err)
			return fn.DeploymentResult{}, err
		}

		_, err = client.UpdateServiceWithRetry(ctx, f.Name, updateService(f, previousService, newEnv, newEnvFrom, newVolumes, newVolumeMounts, d.decorator), 3)
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

		err = createTriggers(ctx, f, client, eventingClient)
		if err != nil {
			return fn.DeploymentResult{}, err
		}

		return fn.DeploymentResult{
			Status:    fn.Updated,
			URL:       route.Status.URL.String(),
			Namespace: namespace,
		}, nil
	}
}

func createTriggers(ctx context.Context, f fn.Function, client clientservingv1.KnServingClient, eventingClient clienteventingv1.KnEventingClient) error {
	ksvc, err := client.GetService(ctx, f.Name)
	if err != nil {
		err = fmt.Errorf("knative deployer failed to get the Service for Trigger: %v", err)
		return err
	}

	fmt.Fprintf(os.Stderr, "🎯 Creating Triggers on the cluster\n")

	for i, sub := range f.Deploy.Subscriptions {
		// create the filter:
		attributes := make(map[string]string)
		for key, value := range sub.Filters {
			attributes[key] = value
		}

		err = eventingClient.CreateTrigger(ctx, &eventingv1.Trigger{
			ObjectMeta: metav1.ObjectMeta{
				Name: fmt.Sprintf("%s-function-trigger-%d", ksvc.Name, i),
				OwnerReferences: []metav1.OwnerReference{
					{
						APIVersion: ksvc.APIVersion,
						Kind:       ksvc.Kind,
						Name:       ksvc.GetName(),
						UID:        ksvc.GetUID(),
					},
				},
			},
			Spec: eventingv1.TriggerSpec{
				Broker: sub.Source,

				Subscriber: duckv1.Destination{
					Ref: &duckv1.KReference{
						APIVersion: ksvc.APIVersion,
						Kind:       ksvc.Kind,
						Name:       ksvc.Name,
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
	// set defaults to the values that avoid the following warning "Kubernetes default value is insecure, Knative may default this to secure in a future release"
	runAsNonRoot := true
	allowPrivilegeEscalation := false
	capabilities := corev1.Capabilities{
		Drop: []corev1.Capability{"ALL"},
	}
	seccompProfile := corev1.SeccompProfile{
		Type: corev1.SeccompProfileType("RuntimeDefault"),
	}
	container := corev1.Container{
		Image: f.Deploy.Image,
		SecurityContext: &corev1.SecurityContext{
			RunAsNonRoot:             &runAsNonRoot,
			AllowPrivilegeEscalation: &allowPrivilegeEscalation,
			Capabilities:             &capabilities,
			SeccompProfile:           &seccompProfile,
		},
	}
	setHealthEndpoints(f, &container)

	referencedSecrets := sets.New[string]()
	referencedConfigMaps := sets.New[string]()
	referencedPVC := sets.New[string]()

	newEnv, newEnvFrom, err := processEnvs(f.Run.Envs, &referencedSecrets, &referencedConfigMaps)
	if err != nil {
		return nil, err
	}
	container.Env = newEnv
	container.EnvFrom = newEnvFrom

	newVolumes, newVolumeMounts, err := processVolumes(f.Run.Volumes, &referencedSecrets, &referencedConfigMaps, &referencedPVC)
	if err != nil {
		return nil, err
	}
	container.VolumeMounts = newVolumeMounts

	labels, err := generateServiceLabels(f, decorator)
	if err != nil {
		return nil, err
	}

	annotations := generateServiceAnnotations(f, decorator, nil)

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
							ServiceAccountName: f.Deploy.ServiceAccountName,
							Volumes:            newVolumes,
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

// generateServiceLabels creates a final map of service labels based
// on the function's defined labels plus the
// application of any provided label decorator.
func generateServiceLabels(f fn.Function, d DeployDecorator) (ll map[string]string, err error) {
	ll, err = f.LabelsMap()
	if err != nil {
		return
	}

	if f.Domain != "" {
		ll["func.domain"] = f.Domain
	}

	if d != nil {
		ll = d.UpdateLabels(f, ll)
	}

	return
}

// generateServiceAnnotations creates a final map of service annotations based
// on static defaults plus the function's defined annotations plus the
// application of any provided annotation decorator.
// Also sets `serving.knative.dev/creator` to a value specified in annotations in the service reference in the previousService parameter,
// this is beneficial when we are updating a service to pass validation on Knative side - the annotation is immutable.
func generateServiceAnnotations(f fn.Function, d DeployDecorator, previousService *v1.Service) (aa map[string]string) {
	aa = make(map[string]string)

	// Enables Dapr support.
	// Has no effect unless the target cluster has Dapr control plane installed.
	for k, v := range daprAnnotations(f.Name) {
		aa[k] = v
	}

	// Function-defined annotations
	for k, v := range f.Deploy.Annotations {
		aa[k] = v
	}

	// Decorator
	if d != nil {
		aa = d.UpdateAnnotations(f, aa)
	}

	// Set correct creator if we are updating a function
	if previousService != nil {
		knativeCreatorAnnotation := "serving.knative.dev/creator"
		if val, ok := previousService.Annotations[knativeCreatorAnnotation]; ok {
			aa[knativeCreatorAnnotation] = val
		}
	}

	return
}

// annotations which, if included and Dapr control plane is installed in
// the target cluster will result in a sidecar exposing the dapr HTTP API
// on localhost:3500 and metrics on 9092
func daprAnnotations(appid string) map[string]string {
	aa := make(map[string]string)
	aa["dapr.io/app-id"] = appid
	aa["dapr.io/enabled"] = DaprEnabled
	aa["dapr.io/metrics-port"] = DaprMetricsPort
	aa["dapr.io/app-port"] = "8080"
	aa["dapr.io/enable-api-logging"] = DaprEnableAPILogging
	return aa
}

func updateService(f fn.Function, previousService *v1.Service, newEnv []corev1.EnvVar, newEnvFrom []corev1.EnvFromSource, newVolumes []corev1.Volume, newVolumeMounts []corev1.VolumeMount, decorator DeployDecorator) func(service *v1.Service) (*v1.Service, error) {
	return func(service *v1.Service) (*v1.Service, error) {
		// Removing the name so the k8s server can fill it in with generated name,
		// this prevents conflicts in Revision name when updating the KService from multiple places.
		service.Spec.Template.Name = ""

		annotations := generateServiceAnnotations(f, decorator, previousService)

		// we need to create a separate map for Annotations specified in a Revision,
		// in case we will need to specify autoscaling annotations -> these could be only in a Revision not in a Service
		revisionAnnotations := make(map[string]string)
		for k, v := range annotations {
			revisionAnnotations[k] = v
		}

		service.ObjectMeta.Annotations = annotations
		service.Spec.Template.ObjectMeta.Annotations = revisionAnnotations

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

		labels, err := generateServiceLabels(f, decorator)
		if err != nil {
			return nil, err
		}

		service.ObjectMeta.Labels = labels
		service.Spec.Template.ObjectMeta.Labels = labels

		err = flags.UpdateImage(&service.Spec.Template.Spec.PodSpec, f.Deploy.Image)
		if err != nil {
			return service, err
		}

		cp.Env = newEnv
		cp.EnvFrom = newEnvFrom
		cp.VolumeMounts = newVolumeMounts
		service.Spec.ConfigurationSpec.Template.Spec.Volumes = newVolumes
		service.Spec.ConfigurationSpec.Template.Spec.PodSpec.ServiceAccountName = f.Deploy.ServiceAccountName
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
func processEnvs(envs []fn.Env, referencedSecrets, referencedConfigMaps *sets.Set[string]) ([]corev1.EnvVar, []corev1.EnvFromSource, error) {

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

// withOpenAddresss prepends ADDRESS=0.0.0.0 to the envs if not present.
//
// This is combined with the value of PORT at runtime to determine the full
// Listener address on which a Function will listen tcp requests.
//
// Runtimes should, by default, only listen on the loopback interface by
// default, as they may be `func run` locally, for security purposes.
// This environment vriable instructs the runtimes to listen on all interfaces
// by default when actually being deployed, since they will need to actually
// listen for client requests and for health readiness/liveness probes.
//
// Should a user wish to securely open their function to only receive requests
// on a specific interface, such as a WireGuar-encrypted mesh network which
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

// / processVolumes generates Volumes and VolumeMounts from a function config
// volumes:
//   - secret: example-secret                              # mount Secret as Volume
//     path: /etc/secret-volume
//   - configMap: example-configMap                        # mount ConfigMap as Volume
//     path: /etc/configMap-volume
//   - persistentVolumeClaim: { claimName: example-pvc }   # mount PersistentVolumeClaim as Volume
//     path: /etc/secret-volume
//   - emptyDir: {}                                         # mount EmptyDir as Volume
//     path: /etc/configMap-volume
func processVolumes(volumes []fn.Volume, referencedSecrets, referencedConfigMaps, referencedPVCs *sets.Set[string]) ([]corev1.Volume, []corev1.VolumeMount, error) {

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

// checkResourcesArePresent returns error if Secrets or ConfigMaps
// referenced in input sets are not deployed on the cluster in the specified namespace
func checkResourcesArePresent(ctx context.Context, namespace string, referencedSecrets, referencedConfigMaps, referencedPVCs *sets.Set[string], referencedServiceAccount string) error {

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
