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

	interceptorServiceName = "keda-add-ons-http-interceptor-proxy"

	// interceptorBridgeSuffix is appended to the function's name to name its
	// bridge Service.
	interceptorBridgeSuffix = "-interceptor-bridge"

	// maxKedaFunctionName is what a DNS-1035 label's 63 characters leave for the
	// function's name once the bridge suffix is taken. Derived, so the two
	// cannot drift apart.
	maxKedaFunctionName = 63 - len(interceptorBridgeSuffix)

	// interceptorServicePortName is the port name on the interceptor's Service:
	// "proxy", not the "http" a function's Service uses.
	interceptorServicePortName = "proxy"
)

// interceptorNamespaceCandidates lists where the interceptor might run, most
// likely first. CMA installs to openshift-keda and exists only on OpenShift;
// the upstream manifests install to keda and run anywhere, OpenShift included.
// So the platform orders the candidates; the probe supplies the answer.
func interceptorNamespaceCandidates() []string {
	if k8s.IsOpenShift() {
		return []string{interceptorNamespaceOpenShift, interceptorNamespaceUpstream}
	}
	return []string{interceptorNamespaceUpstream, interceptorNamespaceOpenShift}
}

// interceptorPresence is what a probe for the interceptor Service established.
// The zero value is interceptorUndetermined, so a presence never set says "I do
// not know" rather than asserting either answer.
type interceptorPresence int

const (
	// interceptorUndetermined: the lookup could not be answered, Forbidden in
	// practice. A restricted account is told the same thing whether the
	// namespace holds an interceptor or not, so this must never be reported as
	// absence.
	interceptorUndetermined interceptorPresence = iota
	// interceptorConfirmed: the Service was read back. The only presence that
	// makes the namespace a fact rather than a guess.
	interceptorConfirmed
	// interceptorAbsent: every candidate answered NotFound. Definite, and not
	// the same as undetermined however similar the outcome looks.
	interceptorAbsent
)

// interceptorNamespace resolves the namespace the keda interceptor runs in by
// looking for its Service. Three answers: the Service reads back (definite),
// NotFound (candidate ruled out), and anything else - Forbidden in practice -
// which teaches nothing, so denial is never read as absence. With nothing
// definite the platform default stands, and a candidate that could not be
// ruled out beats one that was. Anything but interceptorConfirmed is a guess:
// good enough for the cluster-local bridge, not good enough to build a Route
// to, which the router would admit and which would then serve nothing.
func interceptorNamespace(ctx context.Context, clientset kubernetes.Interface) (ns string, presence interceptorPresence) {
	candidates := interceptorNamespaceCandidates()

	var undetermined []string
	for _, candidate := range candidates {
		_, err := clientset.CoreV1().Services(candidate).Get(ctx, interceptorServiceName, metav1.GetOptions{})
		if err == nil {
			return candidate, interceptorConfirmed
		}
		if !k8serrors.IsNotFound(err) {
			undetermined = append(undetermined, candidate)
		}
	}

	// Something could not be read, so nothing here is ruled out: the caller was
	// denied, not answered.
	if len(undetermined) > 0 {
		return undetermined[0], interceptorUndetermined
	}
	// Every candidate answered NotFound. That is an answer.
	return candidates[0], interceptorAbsent
}

// interceptorExposureName builds the name of the object exposing a keda
// function. Every keda function's exposure lands in the one interceptor
// namespace, so the name carries the function's namespace too; without it two
// functions of the same name in different namespaces would collide.
func interceptorExposureName(name, namespace string) string {
	return fmt.Sprintf("%s-%s", name, namespace)
}

// functionURLs returns the URLs to report for a keda function, primary first.
// Every host an HTTPScaledObject registers is a cluster-local bridge address on
// :8080, except the exposed hostname: that is registered there so the
// interceptor matches requests carrying it, but it is reached over https
// through the exposing object. An exposed function leads with that URL, since
// it is the reachable one. exposedHost is empty for a cluster-local function.
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
// nothing garbage collects this Route; Deploy and Remove call Unexpose.
func (d *Deployer) interceptorExposure(f fn.Function, namespace, interceptorNS string) deployer.Exposure {
	return deployer.Exposure{
		Function: f,
		// Different from the Route's namespace, and it is what identifies the
		// Route as this function's inside a namespace shared with every other
		// keda function.
		FunctionNamespace: namespace,
		Name:              interceptorExposureName(f.Name, namespace),
		Namespace:         interceptorNS,
		TargetService:     interceptorServiceName,
		TargetPort:        interceptorServicePortName,
		Owner:             nil,
		Decorator:         d.decorator,
	}
}

// interceptorExposureRef identifies a keda function's Route without describing
// how to build one, which is what teardown needs: Deploy when exposure is
// switched off, and Remove on delete. The Route lives beside the interceptor
// rather than beside the function, so the caller supplies interceptorNS.
func interceptorExposureRef(name, namespace, interceptorNS string) deployer.ExposureRef {
	return deployer.ExposureRef{
		FunctionName:      name,
		FunctionNamespace: namespace,
		Namespace:         interceptorNS,
	}
}

// validateBridgeName refuses a function whose bridge Service name would not
// be a valid DNS-1035 label: the suffix leaves maxKedaFunctionName for the
// function, past which the API server rejects the Service on a plain keda
// deploy. Needs only the name, so it runs before the namespace is resolved;
// validateExposureName covers the part that cannot.
func (d *Deployer) validateBridgeName(f fn.Function) error {
	bridge := d.interceptorBridgeServiceName(f)
	if errs := validation.IsDNS1035Label(bridge); len(errs) > 0 {
		return fmt.Errorf(
			"function name %q is too long for the keda deployer: its bridge Service would be named %q, which is not a valid Service name (%s). Keda limits function names to %d characters",
			f.Name, bridge, strings.Join(errs, "; "), maxKedaFunctionName)
	}
	return nil
}

// validateExposureName refuses a Route name Kubernetes would not accept; it
// needs the resolved namespace, hence separate from validateBridgeName. The
// minted hostname's own 63-character budget is deliberately NOT checked here:
// the arithmetic and the reasoning live beside generate in pkg/ocproute.
func validateExposureName(f fn.Function, namespace string) error {
	route := interceptorExposureName(f.Name, namespace)
	if errs := validation.IsDNS1123Subdomain(route); len(errs) > 0 {
		return fmt.Errorf(
			"function %q cannot be exposed: its Route would be named %q, which is not a valid Route name (%s)",
			f.Name, route, strings.Join(errs, "; "))
	}
	return nil
}
