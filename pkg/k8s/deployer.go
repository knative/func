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
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/util/retry"
	clienteventingv1 "knative.dev/client/pkg/eventing/v1"
	eventingv1 "knative.dev/eventing/pkg/apis/eventing/v1"
	eventingv1client "knative.dev/eventing/pkg/client/clientset/versioned/typed/eventing/v1"
	"knative.dev/func/pkg/deployer"
	"knative.dev/func/pkg/deployers"
	fn "knative.dev/func/pkg/functions"
	"knative.dev/pkg/apis"
	duckv1 "knative.dev/pkg/apis/duck/v1"
)

const (
	KubernetesDeployerName = deployers.Kubernetes

	DefaultLivenessEndpoint  = "/health/liveness"
	DefaultReadinessEndpoint = "/health/readiness"
	DefaultHTTPPort          = 8080

	// RouteHostnameAnnotation records the externally-exposed hostname (if
	// any) on the function's Service, so lister/describer can read it back
	// without re-deriving or re-querying the Route.
	RouteHostnameAnnotation = "function.knative.dev/route-hostname"

	// RouteNamespaceAnnotation records where the exposing Route was created,
	// written by the code that created it; removal reads it. It exists because
	// keda's Route lives with the interceptor, whose namespace depends on how
	// keda was installed.
	//
	// Written and cleared together with RouteHostnameAnnotation.
	RouteNamespaceAnnotation = "function.knative.dev/route-namespace"

	// managedByAnnotation identifies triggers managed by this deployer
	managedByAnnotation = "func.knative.dev/managed-by"
	managedByValue      = "func-raw-deployer"
)

type DeployerOpt func(*Deployer)

type Deployer struct {
	verbose   bool
	decorator deployer.DeployDecorator

	exposer deployer.Exposer
}

func NewDeployer(opts ...DeployerOpt) *Deployer {
	d := &Deployer{}
	for _, opt := range opts {
		opt(d)
	}
	return d
}

func WithExposer(exposer deployer.Exposer) DeployerOpt {
	return func(d *Deployer) {
		d.exposer = exposer
	}
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
	// This only exists because of a bootstrapping problem with On-Cluster
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
	// neither are set.  The logic which arbitrates between current k8s context,
	// flags, environment variables and global defaults to determine the
	// effective namespace is not logic for the deployer implementation, which
	// should have a minimum of logic.  In this case limited to "new ns or
	// existing namespace?
	namespace, err := DeployNamespace(f)
	if err != nil {
		return fn.DeploymentResult{}, err
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

	dynClient, err := dynamic.NewForConfig(config)
	if err != nil {
		return fn.DeploymentResult{}, fmt.Errorf("failed to create dynamic client: %w", err)
	}

	// Check if Dapr is installed
	daprInstalled := false
	_, err = clientset.CoreV1().Namespaces().Get(ctx, "dapr-system", metav1.GetOptions{})
	if err == nil {
		daprInstalled = true
	}

	// shared identity stamps
	labels, err := deployer.GenerateCommonLabels(f, d.decorator)
	if err != nil {
		return fn.DeploymentResult{}, fmt.Errorf("failed to generate common labels: %w", err)
	}
	annotations := deployer.GenerateCommonAnnotations(f, d.decorator, daprInstalled, KubernetesDeployerName)

	deploymentClient := clientset.AppsV1().Deployments(namespace)
	serviceClient := clientset.CoreV1().Services(namespace)

	existingDeployment, err := deploymentClient.Get(ctx, f.Name, metav1.GetOptions{})

	var status fn.Status
	var svc *corev1.Service
	if err == nil {
		// Update the existing function
		referencedSecrets := sets.New[string]()
		referencedConfigMaps := sets.New[string]()
		referencedPVCs := sets.New[string]()

		deployment, err := d.generateDeployment(f, namespace, labels, annotations, &referencedSecrets, &referencedConfigMaps, &referencedPVCs)
		if err != nil {
			return fn.DeploymentResult{}, fmt.Errorf("failed to generate deployment resources: %w", err)
		}

		if err = CheckResourcesArePresent(ctx, namespace, &referencedSecrets, &referencedConfigMaps, &referencedPVCs, f.Deploy.ServiceAccountName, f.Deploy.ImagePullSecret); err != nil {
			return fn.DeploymentResult{}, fmt.Errorf("failed to validate referenced resources: %w", err)
		}

		existingService, svcGetErr := serviceClient.Get(ctx, f.Name, metav1.GetOptions{})
		if svcGetErr != nil {
			if !errors.IsNotFound(svcGetErr) {
				return fn.DeploymentResult{}, fmt.Errorf("failed to get existing service: %w", svcGetErr)
			}
			existingService = nil
		}

		svc, err = d.generateService(f, namespace, labels, annotations, existingDeployment, existingService)
		if err != nil {
			return fn.DeploymentResult{}, fmt.Errorf("failed to generate service resources: %w", err)
		}

		// Preserve resource version for update
		deployment.ResourceVersion = existingDeployment.ResourceVersion

		if err := preserveDeploymentSelector(existingDeployment, deployment, f.Name); err != nil {
			return fn.DeploymentResult{}, err
		}

		if _, err = deploymentClient.Update(ctx, deployment, metav1.UpdateOptions{}); err != nil {
			return fn.DeploymentResult{}, fmt.Errorf("failed to update deployment: %w", err)
		}

		// update/create service; keep the returned object, its UID owns the
		// exposure and trigger satellites below
		if svcGetErr == nil {
			svc.ResourceVersion = existingService.ResourceVersion
			if svc, err = serviceClient.Update(ctx, svc, metav1.UpdateOptions{}); err != nil {
				return fn.DeploymentResult{}, fmt.Errorf("failed to update service: %w", err)
			}
		} else {
			// Confirmed IsNotFound above the generateService()
			if svc, err = serviceClient.Create(ctx, svc, metav1.CreateOptions{}); err != nil {
				return fn.DeploymentResult{}, fmt.Errorf("failed to create service: %w", err)
			}
		}

		status = fn.Updated
		if d.verbose {
			fmt.Fprintf(os.Stderr, "Updated deployment and service %s in namespace %s\n", f.Name, namespace)
		}
	} else {
		if !errors.IsNotFound(err) {
			return fn.DeploymentResult{}, fmt.Errorf("failed to check for existing deployment: %w", err)
		}

		referencedSecrets := sets.New[string]()
		referencedConfigMaps := sets.New[string]()
		referencedPVCs := sets.New[string]()

		deployment, err := d.generateDeployment(f, namespace, labels, annotations, &referencedSecrets, &referencedConfigMaps, &referencedPVCs)
		if err != nil {
			return fn.DeploymentResult{}, fmt.Errorf("failed to generate deployment resources: %w", err)
		}

		if err = CheckResourcesArePresent(ctx, namespace, &referencedSecrets, &referencedConfigMaps, &referencedPVCs, f.Deploy.ServiceAccountName, f.Deploy.ImagePullSecret); err != nil {
			return fn.DeploymentResult{}, fmt.Errorf("failed to validate referenced resources: %w", err)
		}

		deployment, err = deploymentClient.Create(ctx, deployment, metav1.CreateOptions{})
		if err != nil {
			return fn.DeploymentResult{}, fmt.Errorf("failed to create deployment: %w", err)
		}

		svc, err = d.generateService(f, namespace, labels, annotations, deployment, nil)
		if err != nil {
			return fn.DeploymentResult{}, fmt.Errorf("failed to generate service resources: %w", err)
		}

		if svc, err = serviceClient.Create(ctx, svc, metav1.CreateOptions{}); err != nil {
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

	// Reconcile external exposure after Service/Deployment exists on cluster
	// (backend + owner reference for the exposing object).
	url, appliedExpose, err := d.resolveExposure(ctx, f, namespace, svc, clientset, dynClient, labels, annotations)
	if err != nil {
		return fn.DeploymentResult{}, err
	}

	// Sync triggers
	eventingClient, err := newEventingClient(config, namespace)
	if err != nil {
		return fn.DeploymentResult{}, fmt.Errorf("failed to create eventing client: %w", err)
	}
	if err := warnOrFailOnTriggerSync(syncTriggers(ctx, f, namespace, eventingClient, clientset), namespace, len(f.Deploy.Subscriptions) > 0); err != nil {
		return fn.DeploymentResult{}, err
	}

	return fn.DeploymentResult{
		Status:    status,
		URL:       url,
		Namespace: namespace,
		Deployer:  KubernetesDeployerName,
		Expose:    appliedExpose,
	}, nil
}

// preserveDeploymentSelector copies the existing Deployment's selector onto
// desired before Update. Selector field is immutable; older funcs pinned whole
// lable map, including domain. If desired's podtemplate no longer matches a
// pinned label, it refuses.
func preserveDeploymentSelector(existing, desired *appsv1.Deployment, fnName string) error {
	if existing == nil || existing.Spec.Selector == nil {
		return nil
	}
	desired.Spec.Selector = existing.Spec.Selector.DeepCopy()
	for k, v := range existing.Spec.Selector.MatchLabels {
		if desired.Spec.Template.Labels[k] != v {
			return fmt.Errorf(
				"function %q cannot be updated in place: its Deployment was created by an older func whose selector pins %s=%q, and this deploy no longer carries that label. A pinned label can only change by recreation: run 'func delete' and deploy again",
				fnName, k, v)
		}
	}
	return nil
}

// DeployNamespace is where a function will be deployed: the requested
// namespace, or the one it is already deployed in. The wider arbitration
// between kube context, flags, environment and global defaults is settled
// earlier, before a Function reaches a deployer.
//
// Exported so pkg/keda can apply this exact rule ahead of its embedded raw
// deploy; a copy could drift.
func DeployNamespace(f fn.Function) (string, error) {
	if f.Namespace != "" {
		return f.Namespace, nil
	}
	if f.Deploy.Namespace != "" {
		return f.Deploy.Namespace, nil
	}
	return "", fmt.Errorf("deployer requires either a target namespace or that the function be already deployed")
}

// resolveExposure reconciles external exposure: it creates or updates the
// Route when f.Expose asks for one, removes it when only the Service's
// record says one exists, and records the Route's namespace and hostname as
// annotations on the function's Service.
//
// A nil exposer means cluster-local: create nothing, remove nothing, leave
// the record alone, return the Service's own URL. This is how keda uses the
// embedded raw deployer - Deployment and Service only; its exposure is its
// own.
func (d *Deployer) resolveExposure(ctx context.Context, f fn.Function,
	namespace string, svc *corev1.Service, clientset kubernetes.Interface,
	dynClient dynamic.Interface, labels, annotations map[string]string) (url string, appliedExpose string, err error) {

	defaultURL := fmt.Sprintf("http://%s.%s.svc", f.Name, namespace)

	// do nothing with nil exposer - cluster-local exposure
	if d.exposer == nil {
		return defaultURL, "", nil
	}

	// Route ns from function svc annotation
	recordedNS := svc.Annotations[RouteNamespaceAnnotation]

	switch {
	case fn.ActiveExpose(f.Expose): // want a Route: create or update it
		url, err = d.ensureExposure(ctx, f, namespace, svc, clientset, dynClient, labels, annotations)
		if err != nil {
			return "", "", fmt.Errorf("external exposure failed: %w", err)
		}
		return url, f.Expose, nil

	// since we have the svc fetched, use its annotation - cluster-side info
	case recordedNS != "": // dont want a Route but we got one - unexpose
		ref := deployer.NewExposureRef(f.Name, namespace, recordedNS)
		if err := d.exposer.Unexpose(ctx, dynClient, ref); err != nil {
			return "", "", fmt.Errorf("failed to remove external exposure: %w", err)
		}
		if err := RecordExposure(ctx, clientset, ref, ""); err != nil {
			return "", "", err
		}
		return defaultURL, "", nil

	default: // nothing wanted, nothing recorded: Route API untouched
		return defaultURL, "", nil
	}
}

// ensureExposure builds an Exposure for the function's own Service, calls
// the configured Exposer, and records the admitted hostname on svc.
func (d *Deployer) ensureExposure(ctx context.Context, f fn.Function,
	namespace string, svc *corev1.Service, clientset kubernetes.Interface,
	dynClient dynamic.Interface, labels, annotations map[string]string) (string, error) {
	controller := true
	e := deployer.Exposure{
		FunctionName: f.Name,
		// The raw deployer's Route sits beside its function, so these two
		// are the same namespace. They diverge for keda.
		FunctionNamespace: namespace,
		Name:              f.Name,
		Namespace:         namespace,
		TargetService:     f.Name,
		TargetPort:        "http",
		Owner: &metav1.OwnerReference{
			APIVersion: "v1",
			Kind:       "Service",
			Name:       svc.Name,
			UID:        svc.UID,
			Controller: &controller,
		},
		Labels:      maps.Clone(labels),
		Annotations: withoutWorkloadAnnotations(annotations),
	}

	if d.verbose {
		fmt.Fprintf(os.Stderr, "🌐 Exposing function externally -> %s\n", f.Name)
	}

	host, err := d.exposer.Expose(ctx, dynClient, e)
	if err != nil {
		return "", err
	}

	// set hostname&route namespace annotations on the function's service
	if err := RecordExposure(ctx, clientset, e.Ref(), host); err != nil {
		// Record failed; teardown only looks at the record, so Unexpose now.
		if rbErr := d.exposer.Unexpose(ctx, dynClient, e.Ref()); rbErr != nil {
			return "", fmt.Errorf("recording the exposure failed: %w; rolling the Route back failed too: %v", err, rbErr)
		}
		return "", fmt.Errorf("recording the exposure failed, the Route was rolled back: %w", err)
	}

	// ocproute uses edge TLS with redirect; other exposers may differ later.
	return fmt.Sprintf("https://%s", host), nil
}

// RecordExposure writes or clears the Route record on the function's Service.
// ref identifies the Service (FunctionName + FunctionNamespace) and where
// the Route lives (Namespace). A non-empty hostname writes both annotations;
// an empty hostname removes them. A missing Service is fine when clearing.
func RecordExposure(ctx context.Context, clientset kubernetes.Interface, ref deployer.ExposureRef, hostname string) error {
	wantNS := ""
	if hostname != "" {
		wantNS = ref.Namespace
	}
	// Get->Update: a 409 means the Service was patched between those two
	// calls (kubectl, another deploy, a cluster operator). Retry with a new Get.
	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		svc, err := clientset.CoreV1().Services(ref.FunctionNamespace).Get(ctx, ref.FunctionName, metav1.GetOptions{})
		if err != nil {
			return err
		}
		if svc.Annotations[RouteHostnameAnnotation] == hostname &&
			svc.Annotations[RouteNamespaceAnnotation] == wantNS {
			return nil
		}
		// Both annotations together, always
		if hostname == "" {
			delete(svc.Annotations, RouteHostnameAnnotation)
			delete(svc.Annotations, RouteNamespaceAnnotation)
		} else {
			if svc.Annotations == nil {
				svc.Annotations = map[string]string{}
			}
			svc.Annotations[RouteHostnameAnnotation] = hostname
			svc.Annotations[RouteNamespaceAnnotation] = wantNS
		}
		_, err = clientset.CoreV1().Services(ref.FunctionNamespace).Update(ctx, svc, metav1.UpdateOptions{})
		return err
	})
	if err != nil {
		if hostname == "" && errors.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("failed to update exposure state on service %q: %w", ref.FunctionName, err)
	}
	return nil
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

// TODO: gauron: this is a quick fix, not every deployment should be dependent
// on knative eventing. For a proper fix we should make sure we only touch
// resources that are required for a specific deployer. It might be the case
// that we will gate the incompatible deployer switch with a "delete current
// function first to remove its resources before switching..."
//
// warnOrFailOnTriggerSync is the ONLY place trigger-sync errors are checked
// for Forbidden - syncTriggers/deleteStaleTriggers just propagate whatever
// they get. Trigger sync is unconditional (runs even with no subscriptions
// or Eventing installed) and RBAC precedes existence, so a 403 can hit a
// deploy that already succeeded and never touched eventing. Internal calls
// already fail fast, so the first 403 aborts the whole sync immediately -
// checking once here gives exactly one warning and no further doomed
// calls. Any other error still fails the deploy. Forbidden is only
// tolerated when hasSubscriptions is false - a function that declares
// subscriptions is genuinely dependent on eventing, so a forbidden trigger
// sync must fail its deploy rather than silently no-op.
func warnOrFailOnTriggerSync(syncErr error, namespace string, hasSubscriptions bool) error {
	if syncErr == nil {
		return nil
	}
	if errors.IsForbidden(syncErr) {
		if hasSubscriptions {
			return fmt.Errorf("function declares eventing subscriptions but access to triggers.eventing.knative.dev is denied in namespace %q: %w", namespace, syncErr)
		}
		fmt.Fprintf(os.Stderr, "Warning: cannot sync eventing triggers (permission denied) - skipping; "+
			"grant access to triggers.eventing.knative.dev in namespace %q to manage function subscriptions; "+
			"if you are not using func subscriptions, you can safely ignore this message\n", namespace)
		return nil
	}
	return fmt.Errorf("failed to sync triggers: %w", syncErr)
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

		// gauron99: take the name and UID from the service - no need to fetch depl here?
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

func (d *Deployer) generateDeployment(f fn.Function, namespace string, labels, annotations map[string]string, referencedSecrets, referencedConfigMaps, referencedPVCs *sets.Set[string]) (*appsv1.Deployment, error) {

	envVars, envFrom, err := ProcessEnvs(f.Run.Envs, referencedSecrets, referencedConfigMaps)
	if err != nil {
		return nil, fmt.Errorf("failed to process environment variables: %w", err)
	}
	envVars = AppendKafkaEnvs(envVars, f.Run.Kafka)

	volumes, volumeMounts, err := ProcessVolumes(f.Run.Volumes, referencedSecrets, referencedConfigMaps, referencedPVCs)
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
				MatchLabels: deployer.SelectorLabels(labels),
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels:      labels,
					Annotations: annotations,
				},
				Spec: corev1.PodSpec{
					Containers:         []corev1.Container{container},
					ServiceAccountName: f.Deploy.ServiceAccountName,
					ImagePullSecrets:   ImagePullSecrets(f.Deploy.ImagePullSecret),
					Volumes:            volumes,
				},
			},
		},
	}

	return deployment, nil
}

// generateService builds the function's Service; existingService is the
// currently-deployed Service on update, nil on create.
func (d *Deployer) generateService(f fn.Function, namespace string, labels, annotations map[string]string, deployment *appsv1.Deployment, existingService *corev1.Service) (*corev1.Service, error) {
	// clone to add specific information unto service and keep original intact
	annotations = maps.Clone(annotations)
	if annotations == nil {
		annotations = map[string]string{}
	}
	// Unlike the annotations above, which always regenerate, the exposure
	// record is re-applied: it is cluster-derived, written only once the Route
	// is admitted, and this Update replaces the whole annotation map.
	if existingService != nil && existingService.Annotations[RouteHostnameAnnotation] != "" {
		annotations[RouteHostnameAnnotation] = existingService.Annotations[RouteHostnameAnnotation]
		// No key means no record; never write it empty.
		if recordedNS := existingService.Annotations[RouteNamespaceAnnotation]; recordedNS != "" {
			annotations[RouteNamespaceAnnotation] = recordedNS
		}
	}

	// Built by hand rather than with metav1.NewControllerRef, which also sets
	// BlockOwnerDeletion. That flag takes effect only during foreground
	// cascading deletion, which nothing here requests; deletion defaults to
	// background, where it does nothing. Setting it does, however, make the
	// OwnerReferencesPermissionEnforcement admission plugin require update on
	// the owner's finalizers subresource - a grant neither a plain func user
	// nor the Tekton pipeline ServiceAccount holds by default, so the Service
	// create is rejected outright. OpenShift enables that plugin; KinD does
	// not, so the failure never appears in upstream CI but in OpenShift.
	// PS: using metav1.NewControllerRef would fail every remote deploy via raw on ocp.
	controller := true
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:        f.Name,
			Namespace:   namespace,
			Labels:      labels,
			Annotations: annotations,
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: appsv1.SchemeGroupVersion.String(),
					Kind:       "Deployment",
					Name:       deployment.Name,
					UID:        deployment.UID,
					Controller: &controller,
				},
			},
		},
		Spec: corev1.ServiceSpec{
			Type: corev1.ServiceTypeClusterIP,
			// Same selector as the Deployment: a Service selecting on the
			// domain would stop matching the running pods the moment the
			// domain changed, blacking the function out until the rollout
			// caught up.
			Selector: deployer.SelectorLabels(labels),
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

// withoutWorkloadAnnotations drops Dapr sidecar keys. Those belong on the
// depl/pods not exposing object.
func withoutWorkloadAnnotations(annotations map[string]string) map[string]string {
	out := maps.Clone(annotations)
	if out == nil {
		return map[string]string{}
	}
	for k := range deployer.GenerateDaprAnnotations("") {
		delete(out, k)
	}
	return out
}

// CheckResourcesArePresent returns error if Secrets or ConfigMaps
// referenced in input sets are not deployed on the cluster in the specified namespace
func CheckResourcesArePresent(ctx context.Context, namespace string, referencedSecrets, referencedConfigMaps, referencedPVCs *sets.Set[string], referencedServiceAccount, imagePullSecret string) error {
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

	if imagePullSecret != "" {
		_, err := GetSecret(ctx, imagePullSecret, namespace)
		if err != nil {
			errMsg += fmt.Sprintf("  referenced image pull Secret \"%s\" is not present in namespace \"%s\"\n", imagePullSecret, namespace)
		}
	}

	if errMsg != "" {
		return fmt.Errorf("error(s) while validating resources:\n%s", errMsg)
	}

	return nil
}

// ImagePullSecrets converts a secret name to a slice of LocalObjectReference
// suitable for use in a PodSpec. Returns nil if the name is empty.
func ImagePullSecrets(secret string) []corev1.LocalObjectReference {
	if secret == "" {
		return nil
	}
	return []corev1.LocalObjectReference{{Name: secret}}
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

func AppendKafkaEnvs(envVars []corev1.EnvVar, kafka *fn.KafkaConfig) []corev1.EnvVar {
	if kafka == nil || kafka.Brokers == "" || kafka.Topic == "" || kafka.ConsumerGroup == "" {
		return envVars
	}
	envVars = append(envVars,
		corev1.EnvVar{Name: "FUNC_TRANSPORT", Value: "kafka"},
		corev1.EnvVar{Name: "KAFKA_BROKERS", Value: kafka.Brokers},
		corev1.EnvVar{Name: "KAFKA_TOPIC", Value: kafka.Topic},
		corev1.EnvVar{Name: "KAFKA_CONSUMER_GROUP", Value: kafka.ConsumerGroup},
	)
	return envVars
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
			if vol.Path == nil {
				return nil, nil, fmt.Errorf("volume %q is missing required path field", volumeName)
			}
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
