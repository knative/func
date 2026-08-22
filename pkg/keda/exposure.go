package keda

import (
	"context"
	"fmt"
	"strings"

	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/client-go/kubernetes"

	"knative.dev/func/pkg/deployer"
	fn "knative.dev/func/pkg/functions"
	"knative.dev/func/pkg/k8s"
)

const (
	// Where the keda http-add-on installs its interceptor. The OpenShift Custom
	// Metrics Autoscaler uses openshift-keda; the upstream helm chart, which
	// pkg/cluster/keda.go and hack/cluster.sh set up on KinD, uses keda. Call
	// interceptorNamespace() rather than picking one.
	interceptorNamespaceOpenShift = "openshift-keda"
	interceptorNamespaceUpstream  = "keda"

	// interceptorServiceName is the Service the documented http-add-on
	// install creates. A nonstandard helm release name would change it, and
	// func would not find the interceptor.
	interceptorServiceName = "keda-add-ons-http-interceptor-proxy"

	// interceptorBridgeSuffix is appended to the function's name to name its
	// bridge Service. Unique vs the function's own Service in the same ns.
	interceptorBridgeSuffix = "-interceptor-bridge"

	// maxKedaFunctionName is what a DNS-1035 label's 63 characters leave for the
	// function's name once the bridge suffix is taken. Derived, so the two
	// cannot drift apart.
	maxKedaFunctionName = 63 - len(interceptorBridgeSuffix)

	// interceptorServicePortName is the port name on the interceptor's Service:
	// "proxy", not the "http" a function's Service uses.
	interceptorServicePortName = "proxy"
)

// interceptorNamespaceCandidates lists where the interceptor can run. CMA
// installs to openshift-keda and is an OpenShift-only product; the upstream
// chart documents installing to keda and runs anywhere, OpenShift included.
// Off OpenShift only keda is possible, so only keda is probed. Order matters
// when every probe is denied: Forbidden says nothing about existence, so the
// first candidate becomes the guess.
func interceptorNamespaceCandidates() []string {
	if k8s.IsOpenShift() {
		return []string{interceptorNamespaceOpenShift, interceptorNamespaceUpstream}
	}
	return []string{interceptorNamespaceUpstream}
}

// interceptorNamespace looks for the interceptor's Service in each candidate
// namespace, in platform preference order; the first read-back wins. When no
// candidate read back, the guessed namespace is still returned, good enough
// for the cluster-local bridge, and exposeRefusal says why it is not good
// enough to build a Route to.
func interceptorNamespace(ctx context.Context,
	clientset kubernetes.Interface) (ns string, exposeRefusal error) {

	candidates := interceptorNamespaceCandidates()

	var undetermined []string
	for _, candidate := range candidates {
		_, err := clientset.CoreV1().Services(candidate).Get(ctx, interceptorServiceName, metav1.GetOptions{})
		if err == nil {
			return candidate, nil
		}
		if !k8serrors.IsNotFound(err) {
			undetermined = append(undetermined, candidate)
		}
	}

	// Something could not be read, so it is not ruled out: the caller was
	// denied, not answered. Only the namespaces actually in doubt are named.
	if len(undetermined) > 0 {
		return undetermined[0], fmt.Errorf(
			"could not determine whether the keda interceptor Service %q exists in %s, "+
				"so its Route might point at nothing; this is usually a permissions problem "+
				"rather than a missing interceptor, and needs read access to that namespace",
			interceptorServiceName, strings.Join(undetermined, " or "))
	}
	// Every candidate answered NotFound. That is an answer.
	return candidates[0], fmt.Errorf(
		"the keda interceptor Service %q was not found in %s, so its Route would point at nothing",
		interceptorServiceName, strings.Join(candidates, " or "))
}

// interceptorExposureName builds the name of the object func creates to
// expose a keda function; the interceptor itself is keda's, func only routes
// through it. Every keda function's exposure lands in the one interceptor
// namespace, so the name carries the function's namespace too; without it two
// functions of the same name in different namespaces would collide.
func interceptorExposureName(name, namespace string) string {
	return fmt.Sprintf("%s-%s", name, namespace)
}

// functionURLs returns the URLs to report for a keda function, primary first.
// Every host an HTTPScaledObject registers is a cluster-local bridge address
// answering on :8080, except the exposed hostname: it is registered only so
// the interceptor recognizes requests carrying it, and is reached over https
// through the exposing object. An exposed function leads with that URL, the
// only one reachable from outside. exposedHost is empty for a cluster-local
// function.
func functionURLs(hosts []string, exposedHost string) (primary string, all []string) {
	all = make([]string, 0, len(hosts)+1)
	if exposedHost != "" {
		all = append(all, fmt.Sprintf("https://%s", exposedHost))
	}
	for _, host := range hosts {
		if host == exposedHost {
			continue
		}
		all = append(all, fmt.Sprintf("http://%s:8080", host))
	}
	if len(all) == 0 {
		return "", nil
	}
	return all[0], all
}

// interceptorExposure describes the external address wanted for a keda
// function. It targets the shared interceptor Service, not the function's
// own: a Route straight to the function would bypass scale-from-zero. Owner
// is nil because Kubernetes rejects cross-namespace owner references, so
// nothing garbage collects this Route; it is removed explicitly instead,
// when exposure is switched off and when the function is deleted.
func interceptorExposure(ref deployer.ExposureRef, labels, annotations map[string]string) deployer.Exposure {
	return deployer.Exposure{
		FunctionName: ref.FunctionName,
		// Different from the Route's namespace, and it is what identifies the
		// Route as this function's inside a namespace shared with every other
		// keda function.
		FunctionNamespace: ref.FunctionNamespace,
		Name:              interceptorExposureName(ref.FunctionName, ref.FunctionNamespace),
		Namespace:         ref.Namespace,
		TargetService:     interceptorServiceName,
		TargetPort:        interceptorServicePortName,
		Owner:             nil,
		Labels:            labels,
		Annotations:       annotations,
	}
}

// validateBridgeName refuses a function whose bridge Service name would not
// be a valid DNS-1035 label: the suffix leaves maxKedaFunctionName
// characters for the function's name, past which the API server rejects the
// Service on a plain keda deploy. This check needs only the name, so it runs
// before the namespace is resolved; validateExposureName handles the name
// that cannot be built until then.
func validateBridgeName(name string) error {
	bridge := interceptorBridgeServiceName(name)
	if errs := validation.IsDNS1035Label(bridge); len(errs) > 0 {
		return fmt.Errorf(
			"function name %q is too long for the keda deployer: its bridge Service would be named %q, which is not a valid Service name (%s). Keda limits function names to %d characters",
			name, bridge, strings.Join(errs, "; "), maxKedaFunctionName)
	}
	return nil
}

// validateExposureName refuses a Route name Kubernetes would not accept. It
// needs the resolved namespace, which is why it is separate from
// validateBridgeName. The minted hostname's own 63-character budget is
// deliberately not checked here: the arithmetic and the reasoning live in
// pkg/ocproute, beside the code that builds the host.
func validateExposureName(f fn.Function, namespace string) error {
	route := interceptorExposureName(f.Name, namespace)
	if errs := validation.IsDNS1123Subdomain(route); len(errs) > 0 {
		return fmt.Errorf(
			"function %q cannot be exposed: its Route would be named %q, which is not a valid Route name (%s)",
			f.Name, route, strings.Join(errs, "; "))
	}
	return nil
}
