package keda

import (
	"context"
	"fmt"
	"math"
	"os"
	"strings"
	"time"

	httpv1alpha1 "github.com/kedacore/http-add-on/operator/apis/http/v1alpha1"
	v1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"knative.dev/func/pkg/deployer"
	"knative.dev/func/pkg/deployers"
	fn "knative.dev/func/pkg/functions"
	"knative.dev/func/pkg/k8s"
)

const (
	KedaDeployerName = deployers.Keda
)

type DeployerOpt func(*Deployer)

type Deployer struct {
	k8s.Deployer

	verbose   bool
	decorator deployer.DeployDecorator
	exposer   deployer.Exposer
}

func NewDeployer(opts ...DeployerOpt) *Deployer {
	d := &Deployer{
		Deployer: *k8s.NewDeployer(
			// init with the kedaDeployerDecorator to have the correct deployer labels&annotations
			k8s.WithDeployerDecorator(&kedaDeployerDecorator{}),
		),
	}

	for _, opt := range opts {
		opt(d)
	}
	return d
}

func WithDeployerVerbose(verbose bool) DeployerOpt {
	return func(d *Deployer) {
		d.verbose = verbose
		k8s.WithDeployerVerbose(verbose)(&d.Deployer)
	}
}

func WithExposer(exposer deployer.Exposer) DeployerOpt {
	return func(d *Deployer) {
		d.exposer = exposer
	}
}

func WithDeployerDecorator(decorator deployer.DeployDecorator) DeployerOpt {
	// use the custom keda decorator, which wraps the given decorator,
	// but with the keda specific annotations
	kedaDecorator := &kedaDeployerDecorator{
		wrapper: decorator,
	}

	return func(d *Deployer) {
		d.decorator = kedaDecorator
		k8s.WithDeployerDecorator(kedaDecorator)(&d.Deployer)
	}
}

var _ deployer.DeployDecorator = &kedaDeployerDecorator{}

type kedaDeployerDecorator struct {
	wrapper deployer.DeployDecorator
}

func (k *kedaDeployerDecorator) UpdateAnnotations(function fn.Function, annotations map[string]string) map[string]string {
	if k.wrapper != nil {
		annotations = k.wrapper.UpdateAnnotations(function, annotations)
	}

	// set correct deployer name
	annotations[deployer.DeployerNameAnnotation] = KedaDeployerName

	return annotations
}

func (k *kedaDeployerDecorator) UpdateLabels(function fn.Function, labels map[string]string) map[string]string {
	if k.wrapper != nil {
		labels = k.wrapper.UpdateLabels(function, labels)
	}

	return labels
}

func (d *Deployer) Deploy(ctx context.Context, f fn.Function) (fn.DeploymentResult, error) {
	triggers := triggers(f)
	if len(triggers) == 0 {
		// triggers(f) only returns empty when scale.keda is present with an
		// explicitly empty triggers list (the nil-Scale/nil-KEDA case falls
		// back to a plain http trigger). ValidateScale already rejects this,
		// but Deploy is reachable without Function.Validate first (library
		// callers, tests): without this check, Deploy would silently skip
		// both the HTTPScaledObject and Kafka ScaledObject paths and deploy
		// with no scaler at all.
		return fn.DeploymentResult{}, fmt.Errorf("function %q: deployer keda requires at least one trigger in scale.keda.triggers", f.Name)
	}
	wantHTTP := hasHTTPTrigger(triggers)
	wantKafka := hasKafkaTrigger(triggers)

	seenTriggerTypes := map[string]bool{}
	for i, t := range triggers {
		if t.Type != "http" && t.Type != "kafka" {
			// ValidateScale already rejects any type other than http/kafka,
			// but Deploy is reachable without it first (library callers,
			// tests): an unrecognized type makes both wantHTTP and wantKafka
			// false, so without this check Deploy would silently skip every
			// scaler path and deploy the raw workload with no scaling at all.
			return fn.DeploymentResult{}, fmt.Errorf(
				"function %q: scale.keda.triggers[%d].type has invalid value %q, allowed: http, kafka", f.Name, i, t.Type)
		}
		if seenTriggerTypes[t.Type] {
			// ValidateScale already rejects a repeated type, but Deploy is
			// reachable without it first: kafkaTrigger()/the http targetValue
			// lookup both only ever use the first match, so a second trigger
			// of the same type would silently have its settings ignored.
			return fn.DeploymentResult{}, fmt.Errorf(
				"function %q: scale.keda.triggers[%d].type %q is repeated: only one trigger of each type is supported", f.Name, i, t.Type)
		}
		seenTriggerTypes[t.Type] = true
	}
	if wantHTTP && wantKafka {
		// ValidateScale already rejects this combination, but Deploy is
		// reachable without it first: the deployer creates a separate
		// HTTPScaledObject for "http" and a separate ScaledObject for "kafka",
		// both targeting the same Deployment, and KEDA only allows one scaler
		// per workload.
		return fn.DeploymentResult{}, fmt.Errorf(
			"function %q: scale.keda.triggers must not combine type http with type kafka: they cannot scale the same Deployment together, not yet supported", f.Name)
	}

	if pollingIntervalIgnored(f, wantHTTP, wantKafka) {
		// Not fatal: the value is inert, not invalid. httpScaledObject scales
		// off the KEDA HTTP add-on's interceptor metrics, which have no polling
		// interval, so it never reads scale.keda.pollingInterval -- only a
		// kafka trigger's ScaledObject honors it. Warn so a caller who set it
		// expecting it to apply isn't left wondering why nothing changed.
		fmt.Fprintf(os.Stderr, "Warning: scale.keda.pollingInterval is ignored for an http trigger (it applies only to kafka triggers); function %q\n", f.Name)
	}

	if wantHTTP {
		if err := validateBridgeName(f.Name); err != nil {
			return fn.DeploymentResult{}, err
		}
	}
	if wantKafka {
		// Counterpart to validateBridgeName above: the Kafka path names its
		// ScaledObject/TriggerAuthentication by appending suffixes to f.Name,
		// which can overflow the 63-character DNS label limit and fail
		// resource creation server-side with no preflight otherwise.
		if err := validateKafkaResourceNames(f.Name, needsTriggerAuth(f.Run.Kafka)); err != nil {
			return fn.DeploymentResult{}, err
		}
	}

	// replicaBounds is a pure function of f -- no cluster state -- so it runs
	// before d.Deployer.Deploy creates anything. It rejects scale.min/max
	// values that would not survive narrowing to int32, a keda max < 1, and an
	// effective min > max, with clear messages, before the raw Deployment
	// exists. ValidateScale covers the same ground, but Deploy is reachable
	// without going through Function.Validate first (library callers, tests).
	minScale, maxScale, err := replicaBounds(f)
	if err != nil {
		return fn.DeploymentResult{}, err
	}
	if wantKafka {
		// The following are all pure functions of f, run before the raw deploy
		// creates anything: failing before the Deployment/Service exist, rather
		// than after, avoids leaving a partial workload behind with no Kafka
		// scaler and no error pointing at why.
		if f.Run.Kafka == nil {
			return fn.DeploymentResult{}, fmt.Errorf("function %q: scale.keda.triggers has a kafka trigger but run.kafka is not configured", f.Name)
		}
		// buildScaledObject sets these directly as KEDA trigger metadata
		// (bootstrapServers/topic/consumerGroup); an empty value produces a
		// ScaledObject that can't connect to any broker.
		var missing []string
		if f.Run.Kafka.Brokers == "" {
			missing = append(missing, "brokers")
		}
		if f.Run.Kafka.Topic == "" {
			missing = append(missing, "topic")
		}
		if f.Run.Kafka.ConsumerGroup == "" {
			missing = append(missing, "consumerGroup")
		}
		if len(missing) > 0 {
			return fn.DeploymentResult{}, fmt.Errorf("function %q: run.kafka is missing required field(s): %s", f.Name, strings.Join(missing, ", "))
		}
		// buildTriggerAuth's TLS-path resolution only depends on
		// f.Run.Kafka/f.Run.Volumes -- no live deployment needed -- so it runs
		// here, before the raw Deployment/Service exist.
		if err := validateKafkaTLSPaths(f.Run.Kafka, f.Run.Volumes); err != nil {
			return fn.DeploymentResult{}, fmt.Errorf("function %q: %w", f.Name, err)
		}
		// Validate the securityProtocol/TLS/SASL consistency with the same
		// rules Function.Validate applies, so a direct Deploy caller that
		// bypasses it can't create a ScaledObject whose SASL/TLS metadata
		// silently disagrees with how the function's own container
		// authenticates.
		if errs := fn.ValidateKafkaSecurity(f.Run.Kafka); len(errs) > 0 {
			return fn.DeploymentResult{}, fmt.Errorf("function %q: %s", f.Name, strings.Join(errs, "; "))
		}
	}

	// Final sweep with the shared scale validator, for direct callers that
	// bypass Function.Validate. The tailored guards above cover the cases worth
	// a deploy-specific message; this catches the remaining ValidateScale
	// checks they don't -- pollingInterval/cooldownPeriod bounds, per-trigger
	// threshold bounds, and the scale.keda<->scale.kpa mutual exclusion.
	// Gated on an explicit scale.keda/scale.kpa so the intentional default-http
	// fallback (neither set -> triggers() supplies a plain http trigger) still
	// deploys instead of tripping "keda requires a trigger".
	if f.Scale != nil && (f.Scale.KEDA != nil || f.Scale.KPA != nil) {
		if errs := fn.ValidateScale(f.Scale, KedaDeployerName, f.Run.Kafka); len(errs) > 0 {
			return fn.DeploymentResult{}, fmt.Errorf("function %q: %s", f.Name, strings.Join(errs, "; "))
		}
	}

	k8sClientset, err := k8s.NewKubernetesClientset()
	if err != nil {
		return fn.DeploymentResult{}, fmt.Errorf("failed to create K8sClientset: %v", err)
	}
	dynClient, err := k8s.NewDynamicClient()
	if err != nil {
		return fn.DeploymentResult{}, fmt.Errorf("failed to create dynamic client: %w", err)
	}

	// Refuse an in-place scaler-type switch from live cluster state, before the
	// raw deploy mutates the Deployment. The client-side gate covers the local
	// deploy path from func.yaml's recorded scaler type; this covers every path
	// (notably the remote pipeline, which never records it) by looking at the
	// scalers that actually exist. Resolve the target namespace the same way the
	// raw deployer will, so the lookup checks the namespace we're about to deploy
	// into.
	scalerNamespace, err := k8s.DeployNamespace(f)
	if err != nil {
		return fn.DeploymentResult{}, fmt.Errorf("failed to resolve deploy namespace: %w", err)
	}
	httpScaledObjectClientset, err := NewHTTPScaledObjectClientset()
	if err != nil {
		return fn.DeploymentResult{}, fmt.Errorf("unable to create HTTPScaledObject client: %w", err)
	}
	if err := refuseConflictingScaler(ctx, httpScaledObjectClientset, dynClient, scalerNamespace, f.Name, wantKafka); err != nil {
		return fn.DeploymentResult{}, fmt.Errorf("function %q: %w", f.Name, err)
	}

	var interceptorNS string
	var exposeRefusal error
	if wantHTTP {
		// Resolved once per deploy and threaded down; DNS label checks before
		// we create anything on cluster. Only the http path exposes.
		interceptorNS, exposeRefusal = interceptorNamespace(ctx, k8sClientset)
		if err := d.validateExposure(f, exposeRefusal); err != nil {
			return fn.DeploymentResult{}, err
		}
	}

	// execute raw deployment deployer
	deployResult, err := d.Deployer.Deploy(ctx, f)
	if err != nil {
		return fn.DeploymentResult{}, fmt.Errorf("failed to deploy function via raw deployer: %w", err)
	}

	// create additional required keda resources
	namespace := deployResult.Namespace

	deployment, err := k8sClientset.AppsV1().Deployments(namespace).Get(ctx, f.Name, metav1.GetOptions{})
	if err != nil {
		return fn.DeploymentResult{}, fmt.Errorf("failed to get deployment %s/%s: %v", namespace, f.Name, err)
	}

	appService, err := k8sClientset.CoreV1().Services(namespace).Get(ctx, f.Name, metav1.GetOptions{})
	if err != nil {
		return fn.DeploymentResult{}, fmt.Errorf("failed to get service %s/%s: %v", namespace, f.Name, err)
	}

	// HTTP trigger path: bridge Service + HTTPScaledObject
	var url string
	appliedExpose := ""
	if wantHTTP {
		ref := deployer.NewExposureRef(f.Name, namespace, interceptorNS)
		if err := ensureInterceptorBridgeService(ctx, k8sClientset, ref, deployment); err != nil {
			return fn.DeploymentResult{}, fmt.Errorf("failed to ensure proxy service exists: %w", err)
		}

		labels, err := deployer.GenerateCommonLabels(f, d.decorator)
		if err != nil {
			return fn.DeploymentResult{}, fmt.Errorf("failed to generate common labels: %w", err)
		}
		annotations := deployer.GenerateCommonAnnotations(f, d.decorator, false, KedaDeployerName)

		target := deployTarget{
			clientset:   k8sClientset,
			dynClient:   dynClient,
			ref:         ref,
			deployment:  deployment,
			appService:  appService,
			labels:      labels,
			annotations: annotations,
			minScale:    minScale,
			maxScale:    maxScale,
			scale:       f.Scale,
		}

		if d.exposer != nil && fn.ActiveExpose(f.Expose) {
			if url, err = d.deployExposed(ctx, target); err != nil {
				return fn.DeploymentResult{}, err
			}
			appliedExpose = f.Expose
		} else {
			if url, err = d.deployClusterLocal(ctx, target); err != nil {
				return fn.DeploymentResult{}, err
			}
		}
	} else {
		// No HTTP trigger -- URL is the app service. The Service listens on
		// port 80 (routing to the container's DefaultHTTPPort via targetPort),
		// so the URL, like elsewhere in the codebase (e.g.
		// pkg/k8s/describer.go), has no explicit port.
		url = fmt.Sprintf("http://%s.%s.svc", f.Name, namespace)

		// A prior deploy may have exposed this function over HTTP. Nothing
		// reconciles that exposure once the HTTP trigger is gone, so clear it
		// the same way deployClusterLocal does -- otherwise the old Route and
		// the Service's exposure annotations stay active, pointing at a
		// function that no longer has anything serving HTTP.
		target := deployTarget{
			clientset: k8sClientset,
			dynClient: dynClient,
			ref:       deployer.NewExposureRef(f.Name, namespace, ""),
		}
		if err := d.clearExposure(ctx, target, appService.Annotations[k8s.RouteNamespaceAnnotation]); err != nil {
			return fn.DeploymentResult{}, err
		}
	}

	// Kafka trigger path: TriggerAuthentication + ScaledObject
	if wantKafka && f.Run.Kafka != nil {
		needsAuth := needsTriggerAuth(f.Run.Kafka)
		if needsAuth {
			ta, err := buildTriggerAuth(f, deployment, namespace)
			if err != nil {
				// A TLS path was explicitly configured but doesn't resolve to
				// any configured volume. Failing here avoids a ScaledObject
				// whose authenticationRef points at a TriggerAuthentication
				// missing the credential it needs.
				return fn.DeploymentResult{}, fmt.Errorf("function %q: %w", f.Name, err)
			}
			if ta == nil {
				// needsTriggerAuth said SASL/TLS credentials need a
				// TriggerAuthentication, but buildTriggerAuth found nothing to
				// resolve at all. Failing here avoids a ScaledObject whose
				// authenticationRef points at a TriggerAuthentication that was
				// never created.
				return fn.DeploymentResult{}, fmt.Errorf(
					"function %q: run.kafka SASL/TLS credentials are configured but could not be resolved to a Secret or environment variable; "+
						"check that run.kafka.sasl/tls paths match a configured volume", f.Name)
			}
			if err := ensureTriggerAuth(ctx, dynClient, ta); err != nil {
				return fn.DeploymentResult{}, fmt.Errorf("failed to ensure TriggerAuthentication: %w", err)
			}
		}

		kt := kafkaTrigger(triggers)
		so := buildScaledObject(f, kt, deployment, namespace, minScale, maxScale)
		if so != nil {
			if err := ensureScaledObject(ctx, dynClient, so); err != nil {
				return fn.DeploymentResult{}, fmt.Errorf("failed to ensure ScaledObject: %w", err)
			}
		}

		if !needsAuth {
			// SASL/TLS credentials were removed from run.kafka while the kafka
			// trigger stayed, so a TriggerAuthentication a prior deploy created
			// is now unreferenced. Delete it only AFTER the ScaledObject above
			// has been reconciled to drop its authenticationRef -- deleting
			// first would, if that update then failed, leave the live
			// ScaledObject pointing at a TriggerAuthentication that no longer
			// exists. deleteTriggerAuthIfExists checks presence first, so a
			// kafka deploy that never had credentials issues no speculative
			// delete (Forbidden on tighter RBAC). Not fatal: owner-ref GC
			// covers a failure.
			deleteTriggerAuthIfExists(ctx, dynClient, namespace, f.Name)
		}
	}

	scalerType := fn.ScalerTypeHTTP
	if wantKafka {
		scalerType = fn.ScalerTypeKafka
	}

	return fn.DeploymentResult{
		Status:     deployResult.Status,
		URL:        url,
		Namespace:  deployResult.Namespace,
		Deployer:   KedaDeployerName,
		Expose:     appliedExpose,
		ScalerType: scalerType,
	}, nil
}

// validateExposure refuses, before anything is created, an exposure this
// deploy could not honor: a Route name Kubernetes would reject, or an
// interceptor that cannot be confirmed to exist (exposeRefusal, resolved by
// interceptorNamespace). Nothing to check when no exposure is wanted.
func (d *Deployer) validateExposure(f fn.Function, exposeRefusal error) error {
	if d.exposer == nil || !fn.ActiveExpose(f.Expose) {
		return nil
	}
	// The Route's name needs the namespace the function will land in;
	// k8s.DeployNamespace is the same rule the raw deployer uses, so this
	// cannot validate a name the deploy will not use.
	exposeNS, err := k8s.DeployNamespace(f)
	if err != nil {
		return err
	}
	if err := validateExposureName(f, exposeNS); err != nil {
		return err
	}
	// Refuse rather than build a Route to a Service that may not be there:
	// such a Route is admitted and then serves nothing. The two refusals
	// share the NO but not the WHY: "not found" and "could not look" send an
	// operator to different fixes.
	if exposeRefusal != nil {
		return fmt.Errorf("cannot expose function %q: %w", f.Name, exposeRefusal)
	}
	return nil
}

// deployTarget is everything one keda deploy resolved and fetched before
// choosing a path: the clients, the function's placement, replica bounds,
// and the live objects the HSO hangs off
type deployTarget struct {
	clientset   kubernetes.Interface
	dynClient   dynamic.Interface
	ref         deployer.ExposureRef
	deployment  *v1.Deployment
	appService  *corev1.Service
	labels      map[string]string
	annotations map[string]string
	minScale    int32
	maxScale    int32
	scale       *fn.ScaleOptions
}

// bridgeHosts are the cluster-local names the HSO registers for f: requests
// through the bridge Service reach the interceptor carrying one of these.
func bridgeHosts(ref deployer.ExposureRef) []string {
	return []string{
		fmt.Sprintf("%s.%s.svc", interceptorBridgeServiceName(ref.FunctionName), ref.FunctionNamespace),
		interceptorBridgeServiceName(ref.FunctionName),
	}
}

// deployExposed settles an exposed function in the order Route -> HSO ->
// record. The Route goes first because the router mints the hostname and the
// HSO write is where that hostname gets registered: the interceptor 404s any
// Host header no HSO registers. The record is last: teardown and describe
// read it, never the cluster. A record that cannot be written takes the
// just-created Route back down. A kill between create and record still
// orphans, and delete will not collect that Route; the next exposed deploy
// reclaims it, because Expose finds an existing Route by the function's
// labels. Only reached with an Exposer and an active intent.
func (d *Deployer) deployExposed(ctx context.Context, t deployTarget) (string, error) {
	exposedHost, err := d.exposer.Expose(ctx, t.dynClient, interceptorExposure(t.ref, t.labels, t.annotations))
	if err != nil {
		return "", fmt.Errorf("failed to expose function externally: %w", err)
	}

	hosts := append(bridgeHosts(t.ref), exposedHost)
	if err := ensureHTTPScaledObject(ctx, t, hosts); err != nil {
		return "", fmt.Errorf("failed to ensure http scaled object exists: %w", err)
	}

	// reconcile annotations to function service about exposure
	if err := k8s.RecordExposure(ctx, t.clientset, t.ref, exposedHost); err != nil {
		if rbErr := d.exposer.Unexpose(ctx, t.dynClient, t.ref); rbErr != nil {
			return "", fmt.Errorf("recording the exposure failed: %w; rolling the Route back failed too: %v", err, rbErr)
		}
		return "", fmt.Errorf("recording the exposure failed, the Route was rolled back: %w", err)
	}

	// ocproute terminates TLS at the edge and redirects http.
	return fmt.Sprintf("https://%s", exposedHost), nil
}

// deployClusterLocal settles a cluster-local function in the order HSO ->
// removal -> record. The HSO shrink kills external traffic first: dropping
// the exposed hostname from the host list makes the interceptor 404 it, so a
// Forbidden in the interceptor's namespace (while removing the now-dead
// Route in clearExposure) fails the deploy with the function scalable and
// effectively unexposed.
func (d *Deployer) deployClusterLocal(ctx context.Context, t deployTarget) (string, error) {
	hosts := bridgeHosts(t.ref)
	if err := ensureHTTPScaledObject(ctx, t, hosts); err != nil {
		return "", fmt.Errorf("failed to ensure http scaled object exists: %w", err)
	}

	if err := d.clearExposure(ctx, t, t.appService.Annotations[k8s.RouteNamespaceAnnotation]); err != nil {
		return "", err
	}

	return fmt.Sprintf("http://%s:8080", hosts[0]), nil // TODO: check on HTTPS too
}

// clearExposure Unexposes the recorded Route, then clears the Service
// record. Unexpose first so a failure leaves the record for retry. Nil
// exposer is a no-op. recordedNS is a parameter so tests can omit it.
func (d *Deployer) clearExposure(ctx context.Context, t deployTarget, recordedNS string) error {
	if d.exposer == nil {
		return nil
	}

	if recordedNS != "" {
		ref := t.ref
		ref.Namespace = recordedNS
		if err := d.exposer.Unexpose(ctx, t.dynClient, ref); err != nil {
			return fmt.Errorf("failed to remove external exposure: %w", err)
		}
	}

	// hostname == "" -> remove the record
	if err := k8s.RecordExposure(ctx, t.clientset, t.ref, ""); err != nil {
		return fmt.Errorf("failed to clear the exposure record: %w", err)
	}
	return nil
}

const (
	// defaultMinReplicas / defaultMaxReplicas are the HTTPScaledObject
	// replica bounds when the function does not set scale.min / scale.max.
	defaultMinReplicas int32 = 1
	defaultMaxReplicas int32 = 10
)

// replicaBounds is scale.min and scale.max from the function, or the
// defaults above when either is unset. The HTTPScaledObject spec requires
// both; these fallbacks are keda's, not shared with the raw or knative
// deployers.
func replicaBounds(f fn.Function) (min, max int32, err error) {
	min, max = defaultMinReplicas, defaultMaxReplicas
	if scale := f.Scale; scale != nil {
		// scale.min/max are int64 in func.yaml but the HTTPScaledObject
		// replica counts are int32. ValidateScale rejects out-of-range values,
		// but Deploy is reachable without it (library callers), so guard here
		// too: a value outside [0, MaxInt32] would otherwise wrap on narrowing
		// -- e.g. int32(1<<32) == 0. Mirrors the preflight check in the raw
		// deployer.
		if scale.Min != nil {
			if *scale.Min < 0 || *scale.Min > math.MaxInt32 {
				return 0, 0, fmt.Errorf("function %q: scale.min %d is out of range [0, %d]", f.Name, *scale.Min, math.MaxInt32)
			}
			min = int32(*scale.Min)
		}
		if scale.Max != nil {
			if *scale.Max < 0 || *scale.Max > math.MaxInt32 {
				return 0, 0, fmt.Errorf("function %q: scale.max %d is out of range [0, %d]", f.Name, *scale.Max, math.MaxInt32)
			}
			max = int32(*scale.Max)
		}
	}
	// keda maps scale.max straight to the HPA's maxReplicas, which must be >= 1.
	// ValidateScale rejects an explicit scale.max: 0 for keda, but Deploy is
	// reachable without it (library callers), so guard the effective value here
	// too -- otherwise min: 0, max: 0 also slips past the min > max check below.
	if max < 1 {
		return 0, 0, fmt.Errorf("function %q: scale.max %d is invalid; keda requires a maximum of at least 1 (leave scale.max unset to use keda's default)", f.Name, max)
	}
	// ValidateScale compares min against max only when both are set explicitly,
	// so scale.min above keda's default max (with max unset) slips past it and
	// would otherwise yield an HTTPScaledObject with min > max, which KEDA/HPA
	// rejects. Compare the effective values -- including any default filled in
	// above -- and fail with a clear message instead.
	if min > max {
		return 0, 0, fmt.Errorf("function %q: scale.min (%d) exceeds the effective scale.max (%d); set scale.max explicitly", f.Name, min, max)
	}
	return
}

func httpScaledObject(t deployTarget, hosts []string) (*httpv1alpha1.HTTPScaledObject, error) {
	deployment := t.deployment
	service := t.appService
	if len(service.Spec.Ports) == 0 {
		return nil, fmt.Errorf("service %s has no ports defined", service.Name)
	}

	cooldown := int32(300)
	targetValue := int64(100)
	if t.scale != nil && t.scale.KEDA != nil {
		if t.scale.KEDA.CooldownPeriod != nil {
			cooldown = *t.scale.KEDA.CooldownPeriod
		}
		for _, trig := range t.scale.KEDA.Triggers {
			if trig.Type == "http" && trig.TargetValue != nil {
				targetValue = *trig.TargetValue
				break
			}
		}
	}

	return &httpv1alpha1.HTTPScaledObject{
		ObjectMeta: metav1.ObjectMeta{
			Name:        t.ref.FunctionName,
			Namespace:   t.ref.FunctionNamespace,
			Labels:      t.labels,
			Annotations: t.annotations,
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: "apps/v1",
					Kind:       "Deployment",
					Name:       deployment.Name,
					UID:        deployment.UID,
					Controller: new(true),
				},
			},
		},
		Spec: httpv1alpha1.HTTPScaledObjectSpec{
			Hosts: hosts,
			ScaleTargetRef: httpv1alpha1.ScaleTargetRef{
				APIVersion: "apps/v1",
				Kind:       "Deployment",
				Name:       deployment.Name,
				Service:    service.Name,
				Port:       service.Spec.Ports[0].Port,
			},
			Replicas: &httpv1alpha1.ReplicaStruct{
				Min: new(t.minScale),
				Max: new(t.maxScale),
			},
			CooldownPeriod: new(cooldown),
			ScalingMetric: &httpv1alpha1.ScalingMetricSpec{
				Rate: &httpv1alpha1.RateMetricSpec{
					TargetValue: int(targetValue),
					Window: metav1.Duration{
						Duration: time.Minute,
					},
					Granularity: metav1.Duration{
						Duration: time.Second,
					},
				},
			},
		},
	}, nil
}

// pollingIntervalIgnored reports whether scale.keda.pollingInterval is set but
// will have no effect. Only a kafka trigger's ScaledObject honors it (see
// buildScaledObjectSpec); the HTTPScaledObject scales off the KEDA HTTP add-on's
// interceptor metrics, which have no polling interval, so httpScaledObject never
// reads it. Deploy warns (rather than rejects) when this holds. Pure so it can
// be unit-tested without capturing stderr.
func pollingIntervalIgnored(f fn.Function, wantHTTP, wantKafka bool) bool {
	return wantHTTP && !wantKafka &&
		f.Scale != nil && f.Scale.KEDA != nil && f.Scale.KEDA.PollingInterval != nil
}

// deleteTriggerAuthIfExists removes the function's Kafka TriggerAuthentication
// when it is present, warning rather than failing. The Get first means a
// function that never had one issues no delete (avoiding Forbidden on tighter
// RBAC), and owner-ref GC collects anything a warned-past failure leaves behind.
func deleteTriggerAuthIfExists(ctx context.Context, dynClient dynamic.Interface, ns, funcName string) {
	taName := triggerAuthName(funcName)
	if _, err := dynClient.Resource(triggerAuthGVR).Namespace(ns).Get(ctx, taName, metav1.GetOptions{}); err != nil {
		// Not found, or we cannot look: nothing we should try to delete.
		return
	}
	if err := deleteTriggerAuth(ctx, dynClient, ns, taName); err != nil {
		fmt.Fprintf(os.Stderr, "warning: %v\n", err)
	}
}

func interceptorBridgeServiceName(name string) string {
	return name + interceptorBridgeSuffix
}

func interceptorBridgeService(ref deployer.ExposureRef, deployment *v1.Deployment) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      interceptorBridgeServiceName(ref.FunctionName),
			Namespace: ref.FunctionNamespace,
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: "apps/v1",
					Kind:       "Deployment",
					Name:       deployment.Name,
					UID:        deployment.UID,
					Controller: new(true),
				},
			},
		},
		Spec: corev1.ServiceSpec{
			Type:         corev1.ServiceTypeExternalName,
			ExternalName: fmt.Sprintf("%s.%s.svc.cluster.local", interceptorServiceName, ref.Namespace),
		},
	}
}

// ensureInterceptorBridgeService makes sure to create the service which serves
// as the entrypoint to the function this service will server as an external-name
// service and forward the request to the keda interceptor-proxy by preserving
// the host name. This service name is also used in the HTTPScaledObject as
// host name to allow the interceptor to match the request with the correct
// target/scaledObject.
func ensureInterceptorBridgeService(ctx context.Context,
	clientset *kubernetes.Clientset, ref deployer.ExposureRef, deployment *v1.Deployment) error {

	expected := interceptorBridgeService(ref, deployment)
	existing, err := clientset.CoreV1().Services(expected.Namespace).Get(ctx, expected.Name, metav1.GetOptions{})
	if err != nil {
		if k8serrors.IsNotFound(err) {
			if _, err := clientset.CoreV1().Services(expected.Namespace).Create(ctx, expected, metav1.CreateOptions{}); err != nil {
				return fmt.Errorf("failed to create service to interceptor proxy: %w", err)
			}

			return nil
		}

		return fmt.Errorf("failed to get service to interceptor proxy: %w", err)
	}

	// check if we need to update
	if !equality.Semantic.DeepEqual(existing.Spec, expected.Spec) {
		// Preserve resource version for update
		expected.ResourceVersion = existing.ResourceVersion

		if _, err = clientset.CoreV1().Services(ref.FunctionNamespace).Update(ctx, expected, metav1.UpdateOptions{}); err != nil {
			return fmt.Errorf("failed to update service to interceptor proxy: %w", err)
		}

		return nil
	}

	return nil
}

func ensureHTTPScaledObject(ctx context.Context, t deployTarget, hosts []string) error {
	expected, err := httpScaledObject(t, hosts)
	if err != nil {
		return fmt.Errorf("failed to generate http scaled object: %w", err)
	}

	httpScaledObjectClientset, err := NewHTTPScaledObjectClientset()
	if err != nil {
		return fmt.Errorf("failed to create HTTPScaledObject clientset: %v", err)
	}

	existing, err := httpScaledObjectClientset.HttpV1alpha1().HTTPScaledObjects(expected.Namespace).Get(ctx, expected.Name, metav1.GetOptions{})
	if err != nil {
		if k8serrors.IsNotFound(err) {
			if _, err := httpScaledObjectClientset.HttpV1alpha1().HTTPScaledObjects(expected.Namespace).Create(ctx, expected, metav1.CreateOptions{}); err != nil {
				return fmt.Errorf("failed to create HTTPScaledObject: %w", err)
			}

			if err := WaitForHTTPScaledObjectAvailable(ctx, httpScaledObjectClientset, t.ref.FunctionNamespace, expected.Name, k8s.DefaultWaitingTimeout); err != nil {
				return fmt.Errorf("HTTPScaledObject did not become ready: %w", err)
			}

			return nil
		}

		return fmt.Errorf("failed to get HTTPScaledObject: %w", err)
	}

	// check if we need to update
	if !equality.Semantic.DeepEqual(existing.Spec, expected.Spec) {
		// Preserve resource version for update
		expected.ResourceVersion = existing.ResourceVersion

		if _, err = httpScaledObjectClientset.HttpV1alpha1().HTTPScaledObjects(expected.Namespace).Update(ctx, expected, metav1.UpdateOptions{}); err != nil {
			return fmt.Errorf("failed to update HTTPScaledObject: %w", err)
		}

		if err := WaitForHTTPScaledObjectAvailable(ctx, httpScaledObjectClientset, t.ref.FunctionNamespace, expected.Name, k8s.DefaultWaitingTimeout); err != nil {
			return fmt.Errorf("HTTPScaledObject did not become ready: %w", err)
		}

		return nil
	}

	return nil
}

func UsesKedaDeployer(annotations map[string]string) bool {
	deployer, ok := annotations[deployer.DeployerNameAnnotation]

	return ok && deployer == KedaDeployerName
}
