package k8s

import (
	"context"
	"fmt"
	"net"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/util/retry"
	"knative.dev/func/pkg/deployer"
	fn "knative.dev/func/pkg/functions"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwclientset "sigs.k8s.io/gateway-api/pkg/client/clientset/versioned"
)

const (
	// gatewayAPIGroupVersion is checked via discovery to detect whether the
	// Gateway API CRDs (any implementation) are installed on the cluster.
	gatewayAPIGroupVersion = "gateway.networking.k8s.io/v1"

	// sslipDomain is the magic-DNS suffix used to mint a hostname directly
	// from a Gateway's IP address, with no DNS configuration required.
	sslipDomain = "sslip.io"

	// remediateNoGatewayController is the remediation text for the "no
	// Gateway API CRDs installed" hard error.
	remediateNoGatewayController = "Install a Gateway API implementation (e.g. Contour, Envoy Gateway, Istio), " +
		"pass --expose=gateway:<ns>/<name> to use an existing Gateway, or deploy only cluster-local with --expose=none"
)

// ParseGatewayRef splits a "namespace/name" or "namespace/" ref (the part of
// --expose=gateway:<ref>) into parts. name is empty for the trailing-slash
// form ("namespace/"), meaning auto-discovery should be restricted to that
// namespace rather than an exact Gateway being pinned; a Gateway name can
// never legitimately be empty, so the two forms cannot be confused.
func ParseGatewayRef(ref string) (namespace, name string, err error) {
	namespace, name, found := strings.Cut(ref, "/")
	if !found {
		return "", "", fmt.Errorf(
			"--expose=gateway:%s is not valid; use \"gateway:namespace/name\" to pin a Gateway or \"gateway:namespace/\" to restrict discovery to a namespace", ref)
	}
	if namespace == "" {
		return "", "", fmt.Errorf(
			"--expose=gateway:%s is not valid; namespace is required (\"gateway:namespace/name\" or \"gateway:namespace/\")", ref)
	}
	return namespace, name, nil
}

// GatewayAPIAvailable checks if Gateway API CRDs are installed on the
// cluster, via a targeted discovery call for the exact group/version this
// package uses. A cluster that genuinely lacks the CRDs reports NotFound,
// which is reported as (false, nil); any other discovery error (RBAC,
// network, a broken aggregated APIService elsewhere) is a real error and
// must not be silently treated as "not installed".
func GatewayAPIAvailable(clientset kubernetes.Interface) (bool, error) {
	resources, err := clientset.Discovery().ServerResourcesForGroupVersion(gatewayAPIGroupVersion)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return false, nil
		}
		return false, fmt.Errorf("failed to check for Gateway API availability: %w", err)
	}
	for _, r := range resources.APIResources {
		if r.Kind == "HTTPRoute" {
			return true, nil
		}
	}
	return false, nil
}

// ResolveGateway resolves the Gateway to attach a function's HTTPRoute to.
//
//   - ref is "namespace/name": that exact Gateway is preflighted (exists,
//     Programmed=True, a listener admits fnNamespace); any failed check is a
//     hard error naming the check.
//   - ref is "namespace/" (trailing slash): auto-discovery is restricted to
//     that namespace, applying the same filters; exactly one match is used,
//     zero or multiple is a hard error.
//   - ref is "": Gateways are discovered cluster-wide (all namespaces) with
//     the same filters; exactly one match is used (and announced), multiple
//     is a hard error listing candidates, and zero is a hard error with
//     remediation options (see noEligibleGatewayError()).
//
// getLabels supplies the function namespace's labels for Selector-based
// admission; see nsLabelGetter for the memoization contract.
func ResolveGateway(ctx context.Context, gwClient gwclientset.Interface, fnNamespace, ref string, getLabels nsLabelGetter) (*gwv1.Gateway, error) {
	if ref != "" {
		refNs, refName, perr := ParseGatewayRef(ref)
		if perr != nil {
			return nil, perr
		}

		// exact Gateway match
		if refName != "" {
			gw, gerr := gwClient.GatewayV1().Gateways(refNs).Get(ctx, refName, metav1.GetOptions{})
			if gerr != nil {
				if apierrors.IsNotFound(gerr) {
					return nil, fmt.Errorf("--expose=gateway:%s/%s: Gateway does not exist", refNs, refName)
				}
				return nil, fmt.Errorf("--expose=gateway:%s/%s: failed to get Gateway: %w", refNs, refName, gerr)
			}
			if !isGatewayProgrammed(gw) {
				return nil, fmt.Errorf("--expose=gateway:%s/%s: Gateway is not Programmed=True", refNs, refName)
			}
			listener, aerr := selectListener(gw, fnNamespace, getLabels)
			if aerr != nil {
				return nil, fmt.Errorf("--expose=gateway:%s/%s: %w", refNs, refName, aerr)
			}
			if listener == nil {
				return nil, fmt.Errorf("--expose=gateway:%s/%s: no HTTP/HTTPS listener admits namespace %q", refNs, refName, fnNamespace)
			}
			return gw, nil
		}

		// "namespace/" - restrict discovery to refNs.
		candidates, cerr := listEligibleGateways(ctx, gwClient, refNs, fnNamespace, getLabels)
		if cerr != nil {
			return nil, cerr
		}
		switch len(candidates) {
		case 0:
			return nil, noEligibleGatewayError(refNs)
		case 1:
			return candidates[0], nil
		default:
			return nil, fmt.Errorf("multiple eligible Gateways in namespace %q: %s - pin one with --expose=gateway:<ns>/<name>",
				refNs, gatewayNames(candidates))
		}
	}

	// Unset: cluster-wide discovery across all namespaces.
	candidates, cerr := listEligibleGateways(ctx, gwClient, metav1.NamespaceAll, fnNamespace, getLabels)
	if cerr != nil {
		return nil, cerr
	}
	switch len(candidates) {
	case 0:
		return nil, noEligibleGatewayError("")
	case 1:
		gw := candidates[0]
		fmt.Fprintf(os.Stderr, "using gateway %s/%s\n", gw.Namespace, gw.Name)
		return gw, nil
	default:
		return nil, fmt.Errorf("multiple eligible Gateways found cluster-wide: %s - pin one with --expose=gateway:<ns>/<name>",
			gatewayNames(candidates))
	}
}

func gatewayNames(candidates []*gwv1.Gateway) string {
	names := make([]string, len(candidates))
	for i, gw := range candidates {
		names[i] = fmt.Sprintf("%s/%s", gw.Namespace, gw.Name)
	}
	return strings.Join(names, ", ")
}

// listEligibleGateways lists Gateways in 'ns' (when calling this function
// send metav1.NamespaceAll as 'ns' for cluster-wide) filtered to Programmed=True
// with an HTTP/HTTPS listener that admits fnNamespace. Results are sorted by
// namespace/name for deterministic error messages and single-match selection.
func listEligibleGateways(ctx context.Context, gwClient gwclientset.Interface, ns, fnNamespace string, getLabels nsLabelGetter) ([]*gwv1.Gateway, error) {
	list, err := gwClient.GatewayV1().Gateways(ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list Gateways: %w", err)
	}

	var candidates []*gwv1.Gateway
	for i := range list.Items {
		gw := &list.Items[i]
		if !isGatewayProgrammed(gw) {
			continue
		}
		listener, lerr := selectListener(gw, fnNamespace, getLabels)
		if lerr != nil {
			return nil, fmt.Errorf("gateway %s/%s: %w", gw.Namespace, gw.Name, lerr)
		}
		if listener != nil {
			candidates = append(candidates, gw)
		}
	}

	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].Namespace != candidates[j].Namespace {
			return candidates[i].Namespace < candidates[j].Namespace
		}
		return candidates[i].Name < candidates[j].Name
	})

	return candidates, nil
}

func isGatewayProgrammed(gw *gwv1.Gateway) bool {
	return meta.IsStatusConditionTrue(gw.Status.Conditions, string(gwv1.GatewayConditionProgrammed))
}

// selectListener is the single source of truth for both listener eligibility
// and hostname minting. It considers every listener on gw that is HTTP/HTTPS
// and allows the HTTPRoute kind (allowedRoutes.kinds, if set), and collects
// those that admit fnNamespace (allowedRoutes.namespaces). An admission check
// that itself fails (e.g. an RBAC error) is treated as NON-ADMITTING rather
// than aborting the scan, so a later listener still gets
// a chance - "no namespace RBAC unless a Selector decides the outcome".
//
// Eligibility is "at least one admitting listener"; nil, nil means none
// admitted and every admission check succeeded, while nil, err means none
// admitted and at least one admission check failed (surfaced as context for
// the caller's failure). Among admitting listeners, one bearing a usable
// (wildcard) hostname is preferred for minting - regardless of position -
// falling back to the first admitting listener (the IP/sslip path)
// otherwise.
func selectListener(gw *gwv1.Gateway, fnNamespace string, getLabels nsLabelGetter) (*gwv1.Listener, error) {
	var admitting []*gwv1.Listener
	var admitErr error

	for i := range gw.Spec.Listeners {
		listener := &gw.Spec.Listeners[i]
		if listener.Protocol != gwv1.HTTPProtocolType && listener.Protocol != gwv1.HTTPSProtocolType {
			continue
		}
		if !listenerAllowsHTTPRouteKind(listener) {
			continue
		}
		admits, err := admitsNamespace(listener, fnNamespace, gw.Namespace, getLabels)
		// if err, don't fail to allow following listeners be potentially accepted
		if err != nil {
			if admitErr == nil {
				admitErr = err
			}
			continue
		}
		if admits {
			admitting = append(admitting, listener)
		}
	}

	if len(admitting) == 0 {
		return nil, admitErr
	}
	for _, listener := range admitting {
		// prefer listener that has wildcard hostname eg. "*.apps.com"
		if listener.Hostname != nil && strings.HasPrefix(string(*listener.Hostname), "*.") {
			return listener, nil
		}
	}
	return admitting[0], nil
}

// listenerAllowsHTTPRouteKind reports whether allowedRoutes.kinds (if set)
// includes HTTPRoute. An unset/empty kinds list means all kinds supported by
// the listener's protocol are allowed, which includes HTTPRoute for HTTP/HTTPS.
func listenerAllowsHTTPRouteKind(listener *gwv1.Listener) bool {
	if listener.AllowedRoutes == nil || len(listener.AllowedRoutes.Kinds) == 0 {
		return true
	}
	for _, k := range listener.AllowedRoutes.Kinds {
		if k.Kind == "HTTPRoute" {
			return true
		}
	}
	return false
}

// admitsNamespace evaluates allowedRoutes.namespaces.from for a listener:
// All admits any namespace; Same requires fnNamespace == gwNamespace;
// Selector matches fnNamespace's labels against the given LabelSelector.
func admitsNamespace(listener *gwv1.Listener, fnNamespace, gwNamespace string, getLabels nsLabelGetter) (bool, error) {
	from := gwv1.NamespacesFromSame // Gateway API default
	if listener.AllowedRoutes != nil && listener.AllowedRoutes.Namespaces != nil && listener.AllowedRoutes.Namespaces.From != nil {
		from = *listener.AllowedRoutes.Namespaces.From
	}
	switch from {
	case gwv1.NamespacesFromAll:
		return true, nil
	case gwv1.NamespacesFromSame:
		return fnNamespace == gwNamespace, nil
	case gwv1.NamespacesFromSelector:
		// case says: listener admits routes from namespaces which match its
		// selector eg. allowedRoutes.namespaces.selector.matchLabels {team: a}
		if listener.AllowedRoutes.Namespaces.Selector == nil {
			return false, nil
		}
		sel, err := metav1.LabelSelectorAsSelector(listener.AllowedRoutes.Namespaces.Selector)
		if err != nil {
			return false, fmt.Errorf("invalid allowedRoutes.namespaces.selector: %w", err)
		}
		nsLabels, err := getLabels()
		if err != nil {
			return false, err
		}
		return sel.Matches(nsLabels), nil
	default:
		return fnNamespace == gwNamespace, nil // safe default for unknown/future values
	}
}

// nsLabelGetter returns the labels of the function's namespace, consulted by
// Selector-based allowedRoutes admission checks. Implementations MUST be
// memoized - construct via newNSLabelGetter() - because admission may call
// the getter once per listener across every gateway in a resolution, and the
// exposure flow shares ONE getter across gateway resolution and hostname
// minting so both decide on a single label snapshot. A non-memoized getter
// would re-issue the namespace GET per listener and re-open the read-skew
// window between resolve and mint.
type nsLabelGetter func() (labels.Set, error)

// newNSLabelGetter builds the canonical nsLabelGetter: a lazy, memoized
// fetch of ns's labels. No API call happens until the first invocation (so
// clusters that never hit a Selector admission never need "get namespaces"
// permission), and both the labels and any fetch error are memoized so the
// cluster is asked at most once per getter.
func newNSLabelGetter(ctx context.Context, clientset kubernetes.Interface, ns string) nsLabelGetter {
	return sync.OnceValues(func() (labels.Set, error) {
		nsObj, err := clientset.CoreV1().Namespaces().Get(ctx, ns, metav1.GetOptions{})
		if err != nil {
			return nil, fmt.Errorf("failed to get namespace %q for allowedRoutes selector match: %w", ns, err)
		}
		return labels.Set(nsObj.Labels), nil
	})
}

// ResolveHostname determines the HTTPRoute hostname for a function, given the
// resolved Gateway and its admitting listener (as selected by
// ResolveGateway/selectListener() - the same listener admission is used for
// both eligibility and hostname minting). Chain (first match wins):
//  1. f.Domain set        -> "<name>.<ns>.<f.Domain>"; if the listener sets
//     its own hostname, the minted hostname must intersect it, else a hard
//     error (so two functions sharing --domain never collide).
//  2. Listener has a wildcard hostname "*.dom" -> "<name>-<ns>.dom" (a single
//     DNS label, which the wildcard matches).
//  3. Gateway has an IP address (net.ParseIP) -> "<name>-<ns>.<ip>.sslip.io",
//     preferring IPv4; an IPv6-only address is dash-encoded for sslip.io
//     (":" -> "-") since a raw colon is not a valid hostname character.
//     A DNS-name address (e.g. a cloud LoadBalancer) cannot mint a subdomain.
//
// Anything else is a hard error asking for --domain.
func ResolveHostname(f fn.Function, namespace string, gw *gwv1.Gateway, listener *gwv1.Listener) (string, error) {
	var listenerHostname string
	if listener.Hostname != nil {
		listenerHostname = string(*listener.Hostname)
	}

	if f.Domain != "" {
		hostname := fmt.Sprintf("%s.%s.%s", f.Name, namespace, f.Domain)
		if listenerHostname != "" && !hostnameIntersects(listenerHostname, hostname) {
			return "", fmt.Errorf(
				"--domain %q mints hostname %q which does not match the Gateway %s/%s listener hostname %q",
				f.Domain, hostname, gw.Namespace, gw.Name, listenerHostname)
		}
		return hostname, nil
	}

	if strings.HasPrefix(listenerHostname, "*.") {
		baseDomain := listenerHostname[2:]
		return fmt.Sprintf("%s-%s.%s", f.Name, namespace, baseDomain), nil
	}

	if ip := gatewayExternalIP(gw); ip != "" {
		return fmt.Sprintf("%s-%s.%s", f.Name, namespace, ip+"."+sslipDomain), nil
	}

	return "", fmt.Errorf(
		"cannot determine a hostname for function %q: Gateway %s/%s has no wildcard listener hostname "+
			"and no IP address (its external address may be a DNS name, e.g. a cloud load balancer).\n\n"+
			"Set --domain explicitly to derive a hostname", f.Name, gw.Namespace, gw.Name)
}

// hostnameIntersects reports whether candidate matches listenerHostname,
// either exactly or against a wildcard ("*.dom") pattern.
func hostnameIntersects(listenerHostname, candidate string) bool {
	if listenerHostname == candidate {
		return true
	}
	if strings.HasPrefix(listenerHostname, "*.") {
		base := listenerHostname[2:]
		return candidate == base || strings.HasSuffix(candidate, "."+base)
	}
	return false
}

// gatewayExternalIP returns a status.addresses entry suitable for minting an
// sslip.io hostname, preferring the first IPv4 address; if only IPv6
// addresses exist, the first is dash-encoded per sslip.io's IPv6 convention
// (every ":" becomes "-", e.g. "2a01:4f8::1" -> "2a01-4f8--1") since a raw
// colon is not a valid hostname character. Returns "" if no address parses
// as an IP (e.g. all addresses are DNS hostnames).
func gatewayExternalIP(gw *gwv1.Gateway) string {
	var ipv6 string
	for _, addr := range gw.Status.Addresses {
		if addr.Value == "" {
			continue
		}
		ip := net.ParseIP(addr.Value)
		if ip == nil {
			continue
		}
		if v4 := ip.To4(); v4 != nil {
			// Canonicalize: an IPv4-mapped IPv6 address (e.g.
			// "::ffff:192.0.2.1") parses as IPv4 here but addr.Value still
			// contains colons - the dotted-quad form is what must be minted.
			return v4.String()
		}
		if ipv6 == "" {
			ipv6 = addr.Value
		}
	}
	if ipv6 != "" {
		return strings.ReplaceAll(ipv6, ":", "-")
	}
	return ""
}

// GenerateHTTPRoute creates an HTTPRoute for a function. Labels/annotations
// mirror Deployment/Service generation: common labels plus decorator hooks,
// and common annotations (deployer name, user f.Deploy.Annotations, decorator
// hooks) via the same deployer.GenerateCommon* helpers.
func GenerateHTTPRoute(f fn.Function, svcName string, svcPort int32, hostname string, deployment *appsv1.Deployment, gw *gwv1.Gateway, decorator deployer.DeployDecorator, deployerName string) (*gwv1.HTTPRoute, error) {
	labels, err := deployer.GenerateCommonLabels(f, decorator)
	if err != nil {
		return nil, err
	}
	annotations := deployer.GenerateCommonAnnotations(f, decorator, false /* dapr n/a for routing */, deployerName)

	parentRef := gwv1.ParentReference{Name: gwv1.ObjectName(gw.Name)}
	if gw.Namespace != "" && gw.Namespace != deployment.Namespace {
		ns := gwv1.Namespace(gw.Namespace)
		parentRef.Namespace = &ns
	}

	pathType := gwv1.PathMatchPathPrefix
	pathValue := "/"
	port := svcPort

	route := &gwv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:        f.Name,
			Namespace:   deployment.Namespace,
			Labels:      labels,
			Annotations: annotations,
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(deployment, appsv1.SchemeGroupVersion.WithKind("Deployment")),
			},
		},
		Spec: gwv1.HTTPRouteSpec{
			CommonRouteSpec: gwv1.CommonRouteSpec{
				ParentRefs: []gwv1.ParentReference{parentRef},
			},
			Hostnames: []gwv1.Hostname{gwv1.Hostname(hostname)},
			Rules: []gwv1.HTTPRouteRule{
				{
					Matches: []gwv1.HTTPRouteMatch{
						{Path: &gwv1.HTTPPathMatch{Type: &pathType, Value: &pathValue}},
					},
					BackendRefs: []gwv1.HTTPBackendRef{
						{
							BackendRef: gwv1.BackendRef{
								BackendObjectReference: gwv1.BackendObjectReference{
									Name: gwv1.ObjectName(svcName),
									Port: &port,
								},
							},
						},
					},
				},
			},
		},
	}

	return route, nil
}

// EnsureHTTPRoute creates or updates an HTTPRoute, retrying the whole
// get-mutate-update cycle on a 409 conflict (a controller status write can
// race an update from here).
func EnsureHTTPRoute(ctx context.Context, gwClient gwclientset.Interface, ns string, route *gwv1.HTTPRoute) error {
	client := gwClient.GatewayV1().HTTPRoutes(ns)
	name := route.Name

	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		existing, getErr := client.Get(ctx, name, metav1.GetOptions{})
		if getErr != nil {
			if apierrors.IsNotFound(getErr) {
				// A prior iteration's Update attempt (below) may have set
				// route.ResourceVersion; the apiserver rejects a Create that
				// carries one, so it must be cleared before every Create.
				route.ResourceVersion = ""
				_, createErr := client.Create(ctx, route, metav1.CreateOptions{})
				return createErr
			}
			return getErr
		}
		route.ResourceVersion = existing.ResourceVersion
		_, updateErr := client.Update(ctx, route, metav1.UpdateOptions{})
		return updateErr
	})
	if err != nil {
		return fmt.Errorf("failed to ensure HTTPRoute %q: %w", name, err)
	}
	return nil
}

// DeleteHTTPRoute removes an HTTPRoute if it exists.
func DeleteHTTPRoute(ctx context.Context, gwClient gwclientset.Interface, ns, name string) error {
	err := gwClient.GatewayV1().HTTPRoutes(ns).Delete(ctx, name, metav1.DeleteOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("failed to delete HTTPRoute %q: %w", name, err)
	}
	return nil
}

// isManagedHTTPRoute reports whether route was created by GenerateHTTPRoute()
// - as opposed to a user-authored or third-party HTTPRoute that happens to
// share the function's name, which must never be deleted out from under the
// user. Both signals are required: a bare boson.dev/function label, or a
// deployer annotation written by some other component, alone does not prove
// func's raw deployer owns the route.
func isManagedHTTPRoute(route *gwv1.HTTPRoute) bool {
	return route.Labels["boson.dev/function"] == "true" &&
		route.Annotations[deployer.DeployerNameAnnotation] == KubernetesDeployerName
}

// RemoveManagedHTTPRoute deletes the HTTPRoute named 'name' in 'ns' only if
// func owns it (isManagedHTTPRoute()). Returns (removed, error):
//   - not found (route absent, or the CRDs aren't installed) -> (false, nil)
//   - found but not managed -> (false, nil), warning printed, route kept
//   - found and managed, deleted -> (true, nil)
func RemoveManagedHTTPRoute(ctx context.Context, gwClient gwclientset.Interface, ns, name string) (bool, error) {
	route, err := gwClient.GatewayV1().HTTPRoutes(ns).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return false, nil
		}
		return false, fmt.Errorf("failed to check for existing HTTPRoute %q: %w", name, err)
	}

	if !isManagedHTTPRoute(route) {
		fmt.Fprintf(os.Stderr,
			"⚠️  an HTTPRoute named %q exists in namespace %q but is not managed by func - leaving it in place\n",
			name, ns)
		return false, nil
	}

	if err := DeleteHTTPRoute(ctx, gwClient, ns, name); err != nil {
		return false, err
	}
	return true, nil
}

// WaitForRouteAccepted polls the HTTPRoute status until the parent matching
// gwName/gwNamespace reports Accepted=True at the route's current
// generation. It fails immediately (not waiting out the full timeout) only
// when that parent reports Accepted=False, surfacing the condition's reason
// and message; Unknown (the route may still be loading into the cluster) is
// polled through to the timeout like an unreported condition. Stale statuses
// - from a different parent, or an old generation (e.g. a mid-flight Gateway
// switch) - are ignored rather than trusted.
func WaitForRouteAccepted(ctx context.Context, gwClient gwclientset.Interface, ns, name, gwName, gwNamespace string, timeout time.Duration) error {
	var lastErr error
	pollErr := wait.PollUntilContextTimeout(ctx, time.Second, timeout, true, func(ctx context.Context) (bool, error) {
		route, err := gwClient.GatewayV1().HTTPRoutes(ns).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			lastErr = fmt.Errorf("failed to get HTTPRoute %q: %w", name, err)
			return false, nil
		}

		for _, parent := range route.Status.Parents {
			if !parentRefMatches(parent.ParentRef, gwName, gwNamespace, ns) {
				continue
			}
			cond := meta.FindStatusCondition(parent.Conditions, string(gwv1.RouteConditionAccepted))
			// ObservedGeneration == 0 means the controller doesn't report
			// it at all - a hard equality requirement would lock such
			// implementations out entirely, turning a real Accepted=False
			// into a blind timeout instead of a fail-fast.
			if cond == nil || (cond.ObservedGeneration != route.Generation && cond.ObservedGeneration != 0) {
				continue // not yet observed at the current spec generation
			}
			if cond.Status == metav1.ConditionTrue {
				return true, nil
			}
			if cond.Status == metav1.ConditionFalse {
				lastErr = fmt.Errorf("HTTPRoute %q was rejected by Gateway %s/%s: %s: %s",
					name, gwNamespace, gwName, cond.Reason, cond.Message)
				return false, lastErr
			}
			// ConditionUnknown: the controller hasn't reached a verdict yet
			// (route still loading) - keep polling rather than failing fast.
			continue
		}

		return false, nil
	})
	if pollErr != nil {
		if lastErr != nil {
			return lastErr
		}
		return fmt.Errorf("HTTPRoute %q was not accepted by Gateway %s/%s within %s: %w", name, gwNamespace, gwName, timeout, pollErr)
	}
	return nil
}

// parentRefMatches reports whether ref (a status.parents[].parentRef) refers
// to the Gateway gwNamespace/gwName. A parentRef with no namespace set
// defaults to the route's own namespace per the Gateway API spec. Kind and
// Group are also checked (defaulting to "Gateway" and
// "gateway.networking.k8s.io" when unset, per spec): mesh implementations
// report status for Service parents too, and a bare name match against
// those would misattribute their verdict to our Gateway.
// simply: check that HTTPRoute holds correct 'parent' (GW) info
func parentRefMatches(ref gwv1.ParentReference, gwName, gwNamespace, routeNamespace string) bool {
	if ref.Kind != nil && string(*ref.Kind) != "Gateway" {
		return false
	}
	if ref.Group != nil && string(*ref.Group) != gwv1.GroupName {
		return false
	}
	if string(ref.Name) != gwName {
		return false
	}
	refNamespace := routeNamespace
	if ref.Namespace != nil && *ref.Namespace != "" {
		refNamespace = string(*ref.Namespace)
	}
	return refNamespace == gwNamespace
}

// noEligibleGatewayError builds the actionable hard error returned when
// Gateway discovery (cluster-wide or namespace-restricted) finds zero
// eligible Gateways. The message hands the operator every remaining option:
// pin an existing Gateway, ask an administrator to create one (with a
// minimal ready-to-apply manifest), or opt out entirely. scope, if
// non-empty, names the namespace a restricted ("gateway:<ns>/") search was
// limited to; the cluster-wide head says "on this cluster" instead, so the
// message always names exactly the scope that was searched.
func noEligibleGatewayError(scope string) error {
	head := "no eligible gateway found on this cluster"
	if scope != "" {
		head = fmt.Sprintf("no eligible gateway found in namespace %q", scope)
	}
	return fmt.Errorf(
		"%s.\nOptions:\n"+
			"  - pin an existing Gateway:  func deploy --expose=gateway:<namespace>/<name>\n"+
			"  - ask your cluster administrator to create one; minimal example:\n\n"+
			"      apiVersion: gateway.networking.k8s.io/v1\n"+
			"      kind: Gateway\n"+
			"      metadata:\n"+
			"        name: func-gateway\n"+
			"        namespace: <infra-namespace>\n"+
			"      spec:\n"+
			"        gatewayClassName: <run: kubectl get gatewayclass>\n"+
			"        listeners:\n"+
			"        - name: http\n"+
			"          protocol: HTTP\n"+
			"          port: 80\n"+
			"          allowedRoutes:\n"+
			"            namespaces:\n"+
			"              from: All\n\n"+
			"  - or deploy cluster-local only:  func deploy --expose=none",
		head)
}
