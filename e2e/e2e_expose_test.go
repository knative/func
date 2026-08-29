//go:build e2e

package e2e

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"os"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	k8slabels "k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"

	fn "knative.dev/func/pkg/functions"
	"knative.dev/func/pkg/k8s"
	fnlabels "knative.dev/func/pkg/k8s/labels"
	"knative.dev/func/pkg/keda"
)

// ---------------------------------------------------------------------------
// EXPOSE TESTS
// External exposure of a deployed function: --expose, its persistence, and
// the platform gate.
//
// Split by what the cluster can answer, and routed by IsOpenShift() as the
// first statement of each test. Anything asserting a Route needs the
// route.openshift.io API and so runs on OpenShift and skips elsewhere.
// Anything asserting func REFUSES a Route is only meaningful where that API is
// absent, so it runs on KinD and skips on OpenShift. Neither environment runs
// the whole file, by design: point the suite at each in turn and every test
// runs somewhere.
//
// The object graph behind an exposure - the Route's namespace, target, owner
// references, and its host's registration with the keda interceptor - is
// asserted in the pkg/ocproute and pkg/keda integration tests, which gate the
// same way. What is here is the CLI contract: what the user asked for, what
// they were told, and what func.yaml records afterwards.
//
// Local lifecycle tests use --builder=host so the suite is not waiting on
// pack. TestExpose_RouteAllBuilders is the one-shot expose on host, pack,
// and s2i. Remote deploys keep pack: host cannot build in-cluster.
// ---------------------------------------------------------------------------

// requiresOpenShift skips a test whose assertions need a real Route.
func requiresOpenShift(t *testing.T) {
	t.Helper()
	// The gate must answer for the suite's cluster, not the ambient one, and
	// IsOpenShift caches its first answer for the whole binary. setupEnv sets
	// this same value later; the probe needs it first.
	os.Setenv("KUBECONFIG", Kubeconfig)
	if !k8s.IsOpenShift() {
		t.Skip("not an OpenShift cluster: route.openshift.io is unavailable, " +
			"so there is no Route to assert on")
	}
}

// requiresNotOpenShift skips a test that asserts func declines to do something
// no non-OpenShift cluster can do. On OpenShift the refusal correctly does not
// happen, so the test has nothing to say there.
func requiresNotOpenShift(t *testing.T) {
	t.Helper()
	os.Setenv("KUBECONFIG", Kubeconfig) // see requiresOpenShift
	if k8s.IsOpenShift() {
		t.Skip("this is an OpenShift cluster, where a Route is possible, " +
			"so there is no refusal to assert on")
	}
}

// TestExpose_RouteRequiresOpenShift ensures asking for a Route on a cluster
// that has no Route API is refused, and refused BEFORE anything is created.
// Deploying and then failing would leave a running function the user believes
// is externally reachable and a func.yaml that never recorded the attempt.
//
//	func deploy --builder=host --expose=route
func TestExpose_RouteRequiresOpenShift(t *testing.T) {
	requiresNotOpenShift(t)

	name := "func-e2e-test-expose-gate"
	root := fromCleanEnv(t, name)

	if err := newCmd(t, "init", "-l=go").Run(); err != nil {
		t.Fatal(err)
	}

	out, err := newCmdOutput(t, "deploy", "--builder=host", "--deployer=raw", "--expose=route").CombinedOutput()
	if err == nil {
		t.Fatal("expected --expose=route to be refused on a cluster with no Route API")
	}
	if !strings.Contains(string(out), "route.openshift.io") {
		t.Errorf("expected the error to name the missing API, got:\n%s", out)
	}

	// Nothing should have been recorded, because nothing was deployed.
	f, err := fn.NewFunction(root)
	if err != nil {
		t.Fatal(err)
	}
	if f.Deploy.Namespace != "" {
		t.Errorf("expected no recorded deployment after a refused deploy, got namespace %q", f.Deploy.Namespace)
	}
}

// TestExpose_RemoteRouteRequiresOpenShift ensures the platform gate refuses
// --expose=route on a remote deploy too, and before any pipeline work: the
// refusal needs no Tekton on the cluster and leaves nothing behind.
//
//	func deploy --remote --expose=route
func TestExpose_RemoteRouteRequiresOpenShift(t *testing.T) {
	requiresNotOpenShift(t)

	name := "func-e2e-test-expose-remote-gate"
	root := fromCleanEnv(t, name)

	if err := newCmd(t, "init", "-l=go").Run(); err != nil {
		t.Fatal(err)
	}

	out, err := newCmdOutput(t, "deploy", "--remote", "--deployer=raw", "--expose=route",
		"--registry="+Registry).CombinedOutput()
	if err == nil {
		t.Fatal("expected a remote --expose=route to be refused on a cluster with no Route API")
	}
	if !strings.Contains(string(out), "route.openshift.io") {
		t.Errorf("expected the error to name the missing API, got:\n%s", out)
	}

	f, err := fn.NewFunction(root)
	if err != nil {
		t.Fatal(err)
	}
	if f.Deploy.Namespace != "" {
		t.Errorf("expected no recorded deployment after a refused remote deploy, got namespace %q", f.Deploy.Namespace)
	}
}

// TestExpose_ClusterLocalByDefault ensures a function deployed without the
// flag is cluster-local, and stays that way in the record. Knative functions
// get an external URL by default, so this is the default most likely to
// surprise a deployer-switching user: raw exposes only when asked.
//
//	func deploy --builder=host --deployer=raw
func TestExpose_ClusterLocalByDefault(t *testing.T) {
	name := "func-e2e-test-expose-default"
	root := fromCleanEnv(t, name)

	if err := newCmd(t, "init", "-l=go").Run(); err != nil {
		t.Fatal(err)
	}
	if err := newCmd(t, "deploy", "--builder=host", "--deployer=raw").Run(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = newCmd(t, "delete").Run() })

	f, err := fn.NewFunction(root)
	if err != nil {
		t.Fatal(err)
	}
	if f.Expose != "" {
		t.Errorf("expected no exposure intent recorded, got %q", f.Expose)
	}
	if f.Deploy.Expose != "" {
		t.Errorf("expected no exposure applied, got %q", f.Deploy.Expose)
	}
}

// TestExpose_KedaRejectsLongName ensures a function name that is legal on its
// own but too long once keda's bridge suffix is added is refused up front,
// rather than by an opaque API rejection after the Deployment already exists.
//
//	func deploy --builder=host --deployer=keda
func TestExpose_KedaRejectsLongName(t *testing.T) {
	// 45 characters: legal as a function name (DNS-1035 allows 63), one over
	// what keda's "-interceptor-bridge" suffix leaves room for (63-19=44).
	name := "func-e2e-test-expose-name-too-long-for-kedaxx"
	if len(name) != 45 {
		t.Fatalf("test setup: expected a 45 character name, got %d", len(name))
	}
	fromCleanEnv(t, name)

	if err := newCmd(t, "init", "-l=go").Run(); err != nil {
		t.Fatal(err)
	}

	out, err := newCmdOutput(t, "deploy", "--builder=host", "--deployer=keda").CombinedOutput()
	if err == nil {
		t.Fatal("expected a name too long for keda's bridge Service to be refused")
	}
	if !strings.Contains(string(out), "too long") {
		t.Errorf("expected the error to explain the length limit, got:\n%s", out)
	}
}

// TestExpose_Route walks the raw exposure lifecycle in the order a user
// would: deploy cluster-local, decide later to expose, opt out again,
// re-expose, delete. Each leg asserts the Route and the Service records that
// deploy leaves behind. The last leg asserts delete takes the Route with it,
// which for the raw deployer happens through its owner reference rather than
// by remover code, so the disappearance is polled, not read once.
//
//	func deploy --builder=host ; --expose=route ; --expose=none ; --expose=route ; func delete
func TestExpose_Route(t *testing.T) {
	requiresOpenShift(t)

	name := "func-e2e-test-expose-route"
	root := fromCleanEnv(t, name)

	if err := newCmd(t, "init", "-l=go").Run(); err != nil {
		t.Fatal(err)
	}

	// Cluster-local first: no intent, nothing applied, no Route.
	if err := newCmd(t, "deploy", "--builder=host", "--deployer=raw").Run(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = newCmd(t, "delete").Run() })

	f, err := fn.NewFunction(root)
	if err != nil {
		t.Fatal(err)
	}
	ns := f.Deploy.Namespace
	if f.Deploy.Expose != "" {
		t.Errorf("expected no exposure applied on a flagless deploy, got %q", f.Deploy.Expose)
	}
	if n := routeCount(t, ns, name, ns); n != 0 {
		t.Fatalf("expected no Route for a cluster-local function, found %d in %q", n, ns)
	}

	// Exposing an already-running function: the Route appears and the Service
	// records both halves of the exposure. The raw deployer's Route sits
	// beside its function.
	if err := newCmd(t, "deploy", "--builder=host", "--deployer=raw", "--expose=route").Run(); err != nil {
		t.Fatal(err)
	}
	if f, err = fn.NewFunction(root); err != nil {
		t.Fatal(err)
	}
	if f.Expose != fn.ExposeRoute {
		t.Errorf("expected intent %q, got %q", fn.ExposeRoute, f.Expose)
	}
	if f.Deploy.Expose != fn.ExposeRoute {
		t.Errorf("expected applied exposure %q, got %q", fn.ExposeRoute, f.Deploy.Expose)
	}
	ann := serviceAnnotations(t, ns, name)
	if ann[k8s.RouteHostnameAnnotation] == "" {
		t.Error("expected the Service to record the exposed hostname")
	}
	if got := ann[k8s.RouteNamespaceAnnotation]; got != ns {
		t.Errorf("expected the Route recorded in the function's namespace %q, got %q", ns, got)
	}
	if n := routeCount(t, ns, name, ns); n != 1 {
		t.Fatalf("expected 1 Route in %q, found %d", ns, n)
	}

	// Turning it off must take the Route away, clear the records, and leave
	// the function running.
	if err := newCmd(t, "deploy", "--builder=host", "--deployer=raw", "--expose=none").Run(); err != nil {
		t.Fatal(err)
	}
	if f, err = fn.NewFunction(root); err != nil {
		t.Fatal(err)
	}
	if f.Deploy.Expose != "" {
		t.Errorf("expected applied exposure cleared after opting out, got %q", f.Deploy.Expose)
	}
	if n := routeCount(t, ns, name, ns); n != 0 {
		t.Errorf("expected the Route removed on opt-out, found %d in %q", n, ns)
	}
	ann = serviceAnnotations(t, ns, name) // the Service surviving is itself the liveness assert
	if v := ann[k8s.RouteHostnameAnnotation]; v != "" {
		t.Errorf("expected the hostname record cleared on opt-out, got %q", v)
	}
	if v := ann[k8s.RouteNamespaceAnnotation]; v != "" {
		t.Errorf("expected the namespace record cleared on opt-out, got %q", v)
	}

	// On again, so delete below removes an exposed function.
	if err := newCmd(t, "deploy", "--builder=host", "--deployer=raw", "--expose=route").Run(); err != nil {
		t.Fatal(err)
	}
	if n := routeCount(t, ns, name, ns); n != 1 {
		t.Fatalf("expected the Route back after re-exposing, found %d in %q", n, ns)
	}

	// Delete while exposed: the Route goes by owner reference, so garbage
	// collection is given a deadline rather than one look.
	if err := newCmd(t, "delete").Run(); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(60 * time.Second)
	for {
		if n := routeCount(t, ns, name, ns); n == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("expected garbage collection to remove the Route after delete, still present in %q", ns)
		}
		time.Sleep(3 * time.Second)
	}
}

// TestExpose_RouteAllBuilders is one successful --expose=route on each local
// builder. Lifecycle, keda, and domain stay on host; this is the builder
// spread. Subtests are sequential: fromCleanEnv changes process cwd.
//
//	func deploy --builder={host,pack,s2i} --deployer=raw --expose=route
func TestExpose_RouteAllBuilders(t *testing.T) {
	requiresOpenShift(t)

	for _, builder := range []string{"host", "pack", "s2i"} {
		t.Run(builder, func(t *testing.T) {
			if builder == "pack" && (runtime.GOARCH == "arm64" || runtime.GOARCH == "arm") {
				t.Skip("Paketo buildpacks do not currently support ARM64 architecture")
			}

			name := "func-e2e-expose-" + builder
			root := fromCleanEnv(t, name)

			if err := newCmd(t, "init", "-l=go").Run(); err != nil {
				t.Fatal(err)
			}
			if err := newCmd(t, "deploy", "--builder="+builder, "--deployer=raw", "--expose=route").Run(); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = newCmd(t, "delete").Run() })

			f, err := fn.NewFunction(root)
			if err != nil {
				t.Fatal(err)
			}
			if f.Deploy.Expose != fn.ExposeRoute {
				t.Errorf("expected applied exposure %q, got %q", fn.ExposeRoute, f.Deploy.Expose)
			}
			ns := f.Deploy.Namespace
			ann := serviceAnnotations(t, ns, name)
			if ann[k8s.RouteHostnameAnnotation] == "" {
				t.Error("expected the Service to record the exposed hostname")
			}
			if got := ann[k8s.RouteNamespaceAnnotation]; got != ns {
				t.Errorf("expected the Route recorded in the function's namespace %q, got %q", ns, got)
			}
			if n := routeCount(t, ns, name, ns); n != 1 {
				t.Fatalf("expected 1 Route in %q, found %d", ns, n)
			}
		})
	}
}

// TestExpose_KedaRoute ensures a keda function can be exposed and unexposed
// through the CLI. Keda's Route is the one nothing garbage collects: a Route
// left behind by an opt-out survives forever.
//
//	func deploy --builder=host --deployer=keda --expose=route
func TestExpose_KedaRoute(t *testing.T) {
	requiresOpenShift(t)

	name := "func-e2e-test-expose-keda"
	root := fromCleanEnv(t, name)

	if err := newCmd(t, "init", "-l=go").Run(); err != nil {
		t.Fatal(err)
	}
	if err := newCmd(t, "deploy", "--builder=host", "--deployer=keda", "--expose=route").Run(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = newCmd(t, "delete").Run() })

	f, err := fn.NewFunction(root)
	if err != nil {
		t.Fatal(err)
	}
	if f.Deploy.Deployer != "keda" {
		t.Fatalf("expected the keda deployer to be recorded, got %q", f.Deploy.Deployer)
	}
	if f.Deploy.Expose != fn.ExposeRoute {
		t.Errorf("expected applied exposure %q, got %q", fn.ExposeRoute, f.Deploy.Expose)
	}

	// An exposed keda function must lead with its external URL. The bridge
	// addresses stay listed, but they are cluster-local and answer nothing
	// from outside.
	out, err := newCmdOutput(t, "describe", "-o=plain").CombinedOutput()
	if err != nil {
		t.Fatalf("describe failed: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "https://") {
		t.Errorf("expected describe to report an https URL for an exposed function, got:\n%s", out)
	}

	// Opting out must take the Route away. Nothing else ever will.
	if err := newCmd(t, "deploy", "--builder=host", "--deployer=keda", "--expose=none").Run(); err != nil {
		t.Fatal(err)
	}
	if f, err = fn.NewFunction(root); err != nil {
		t.Fatal(err)
	}
	if f.Deploy.Expose != "" {
		t.Errorf("expected applied exposure cleared after opting out, got %q", f.Deploy.Expose)
	}
}

// routeCount reports how many Routes in ns carry this function's identity
// labels. The lookup mirrors the one func delete uses, so these tests fail if
// the identity rules change.
func routeCount(t *testing.T, ns, fnName, fnNamespace string) int {
	t.Helper()
	client, err := k8s.NewDynamicClient()
	if err != nil {
		t.Fatal(err)
	}
	gvr := schema.GroupVersionResource{Group: "route.openshift.io", Version: "v1", Resource: "routes"}
	sel := k8slabels.SelectorFromSet(k8slabels.Set{
		fnlabels.FunctionKey:          "true",
		fnlabels.FunctionNameKey:      fnName,
		fnlabels.FunctionNamespaceKey: fnNamespace,
	}).String()
	list, err := client.Resource(gvr).Namespace(ns).List(context.Background(), metav1.ListOptions{LabelSelector: sel})
	if err != nil {
		t.Fatal(err)
	}
	return len(list.Items)
}

// serviceAnnotations returns the function Service's annotations, where deploy
// records the exposed hostname and the Route's namespace.
func serviceAnnotations(t *testing.T, ns, name string) map[string]string {
	t.Helper()
	clientset, err := k8s.NewKubernetesClientset()
	if err != nil {
		t.Fatal(err)
	}
	svc, err := clientset.CoreV1().Services(ns).Get(context.Background(), name, metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	return svc.Annotations
}

// TestExpose_KedaToggle ensures exposure turns off and on again in place.
// Off must remove the Route and both Service records while the function keeps
// running; on must bring them back. Keda's Route is the one nothing garbage
// collects, so off doing its half is what keeps an opt-out from leaking.
//
//	func deploy --builder=host --deployer=keda --expose=route ; --expose=none ; --expose=route
func TestExpose_KedaToggle(t *testing.T) {
	requiresOpenShift(t)

	name := "func-e2e-expose-keda-toggle"
	root := fromCleanEnv(t, name)

	if err := newCmd(t, "init", "-l=go").Run(); err != nil {
		t.Fatal(err)
	}
	if err := newCmd(t, "deploy", "--builder=host", "--deployer=keda", "--expose=route").Run(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = newCmd(t, "delete").Run() })

	f, err := fn.NewFunction(root)
	if err != nil {
		t.Fatal(err)
	}
	ns := f.Deploy.Namespace

	ann := serviceAnnotations(t, ns, name)
	recordedNS := ann[k8s.RouteNamespaceAnnotation]
	if recordedNS == "" {
		t.Fatal("expected the Service to record the Route's namespace")
	}
	if ann[k8s.RouteHostnameAnnotation] == "" {
		t.Error("expected the Service to record the exposed hostname")
	}
	if n := routeCount(t, recordedNS, name, ns); n != 1 {
		t.Fatalf("expected 1 Route in %q, found %d", recordedNS, n)
	}

	// Off: the Route and both records go; the function does not.
	if err := newCmd(t, "deploy", "--builder=host", "--deployer=keda", "--expose=none").Run(); err != nil {
		t.Fatal(err)
	}
	ann = serviceAnnotations(t, ns, name) // the Service surviving is itself the liveness assert
	if v := ann[k8s.RouteNamespaceAnnotation]; v != "" {
		t.Errorf("expected the namespace record cleared on opt-out, got %q", v)
	}
	if v := ann[k8s.RouteHostnameAnnotation]; v != "" {
		t.Errorf("expected the hostname record cleared on opt-out, got %q", v)
	}
	if n := routeCount(t, recordedNS, name, ns); n != 0 {
		t.Errorf("expected the Route removed on opt-out, found %d in %q", n, recordedNS)
	}

	// On again: both come back.
	if err := newCmd(t, "deploy", "--builder=host", "--deployer=keda", "--expose=route").Run(); err != nil {
		t.Fatal(err)
	}
	if n := routeCount(t, recordedNS, name, ns); n != 1 {
		t.Errorf("expected the Route back after re-exposing, found %d in %q", n, recordedNS)
	}
	if serviceAnnotations(t, ns, name)[k8s.RouteNamespaceAnnotation] == "" {
		t.Error("expected the namespace record back after re-exposing")
	}
}

// TestExpose_KedaDeleteCleansRoute ensures func delete removes an exposed keda
// function's Route. The Route lives in the interceptor's namespace with no
// owner, so delete is the only thing that ever removes it; one left behind is
// an orphan forever.
//
//	func deploy --builder=host --deployer=keda --expose=route ; func delete
func TestExpose_KedaDeleteCleansRoute(t *testing.T) {
	requiresOpenShift(t)

	name := "func-e2e-expose-keda-delete"
	root := fromCleanEnv(t, name)

	if err := newCmd(t, "init", "-l=go").Run(); err != nil {
		t.Fatal(err)
	}
	if err := newCmd(t, "deploy", "--builder=host", "--deployer=keda", "--expose=route").Run(); err != nil {
		t.Fatal(err)
	}

	f, err := fn.NewFunction(root)
	if err != nil {
		t.Fatal(err)
	}
	ns := f.Deploy.Namespace

	recordedNS := serviceAnnotations(t, ns, name)[k8s.RouteNamespaceAnnotation]
	if recordedNS == "" {
		t.Fatal("expected the Service to record the Route's namespace")
	}
	if n := routeCount(t, recordedNS, name, ns); n != 1 {
		t.Fatalf("expected 1 Route before delete, found %d in %q", n, recordedNS)
	}

	if err := newCmd(t, "delete").Run(); err != nil {
		t.Fatal(err)
	}

	if n := routeCount(t, recordedNS, name, ns); n != 0 {
		t.Errorf("expected delete to remove the Route, found %d left in %q", n, recordedNS)
	}
	clientset, err := k8s.NewKubernetesClientset()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := clientset.CoreV1().Services(ns).Get(context.Background(), name, metav1.GetOptions{}); !apierrors.IsNotFound(err) {
		t.Errorf("expected the function's Service gone after delete, got %v", err)
	}
}

// TestExpose_KedaRouteDomain ensures a custom domain rides keda's exposure:
// the Route in the interceptor's namespace carries the domain as its host,
// the Service records it as the exposed hostname, and the HTTPScaledObject
// registers it, since the interceptor 404s any Host header its
// HTTPScaledObject does not list. TLS and traffic for a custom domain are
// TestExpose_RouteDomainTLS's assert; the Route machinery is shared.
//
//	func deploy --builder=host --deployer=keda --expose=route --domain=<custom>
func TestExpose_KedaRouteDomain(t *testing.T) {
	requiresOpenShift(t)

	name := "func-e2e-expose-keda-domain"
	root := fromCleanEnv(t, name)
	const domain = "func-e2e-expose-keda-domain.test"

	if err := newCmd(t, "init", "-l=go").Run(); err != nil {
		t.Fatal(err)
	}
	if err := newCmd(t, "deploy", "--builder=host", "--deployer=keda", "--expose=route", "--domain="+domain).Run(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = newCmd(t, "delete").Run() })

	f, err := fn.NewFunction(root)
	if err != nil {
		t.Fatal(err)
	}
	ns := f.Deploy.Namespace

	ann := serviceAnnotations(t, ns, name)
	if got := ann[k8s.RouteHostnameAnnotation]; got != domain {
		t.Errorf("expected the custom domain %q recorded as the exposed hostname, got %q", domain, got)
	}
	recordedNS := ann[k8s.RouteNamespaceAnnotation]
	if recordedNS == "" {
		t.Fatal("expected the Service to record the Route's namespace")
	}
	route := routeFor(t, recordedNS, name, ns)
	if host, _, _ := unstructured.NestedString(route.Object, "spec", "host"); host != domain {
		t.Errorf("expected spec.host %q, got %q", domain, host)
	}

	hsoClient, err := keda.NewHTTPScaledObjectClientset()
	if err != nil {
		t.Fatal(err)
	}
	hso, err := hsoClient.HttpV1alpha1().HTTPScaledObjects(ns).Get(context.Background(), name, metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(hso.Spec.Hosts, domain) {
		t.Errorf("expected the HTTPScaledObject to register the domain %q, hosts: %v", domain, hso.Spec.Hosts)
	}
}

// routeFor returns the one Route carrying this function's identity labels.
func routeFor(t *testing.T, ns, fnName, fnNamespace string) *unstructured.Unstructured {
	t.Helper()
	client, err := k8s.NewDynamicClient()
	if err != nil {
		t.Fatal(err)
	}
	gvr := schema.GroupVersionResource{Group: "route.openshift.io", Version: "v1", Resource: "routes"}
	sel := k8slabels.SelectorFromSet(k8slabels.Set{
		fnlabels.FunctionKey:          "true",
		fnlabels.FunctionNameKey:      fnName,
		fnlabels.FunctionNamespaceKey: fnNamespace,
	}).String()
	list, err := client.Resource(gvr).Namespace(ns).List(context.Background(), metav1.ListOptions{LabelSelector: sel})
	if err != nil {
		t.Fatal(err)
	}
	if len(list.Items) != 1 {
		t.Fatalf("expected exactly 1 Route for %s in %s, found %d", fnName, ns, len(list.Items))
	}
	return &list.Items[0]
}

// selfSignedCert returns a PEM cert and key for domain plus a pool trusting
// them: the test's stand-in for what cert-manager would issue.
func selfSignedCert(t *testing.T, domain string) (certPEM, keyPEM string, pool *x509.CertPool) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: domain},
		DNSNames:              []string{domain},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	der, err := x509.CreateCertificate(rand.Reader, &tmpl, &tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	certPEM = string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	keyPEM = string(pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}))
	pool = x509.NewCertPool()
	pool.AppendCertsFromPEM([]byte(certPEM))
	return certPEM, keyPEM, pool
}

// TestExpose_RouteDomainTLS proves the custom-domain chain end to end with
// neither cert-manager nor DNS: the test plays the certificate controller by
// injecting a self-signed cert into the Route, a redeploy proves func carries
// the injection over, and an HTTPS request dialed straight at the router
// (DNS bypassed) must be served with exactly that certificate.
//
//	func deploy --builder=host --deployer=raw --expose=route --domain=<custom>
func TestExpose_RouteDomainTLS(t *testing.T) {
	requiresOpenShift(t)

	name := "func-e2e-expose-domain"
	root := fromCleanEnv(t, name)
	const domain = "func-e2e-expose-domain.test"

	if err := newCmd(t, "init", "-l=go").Run(); err != nil {
		t.Fatal(err)
	}
	// First without a domain: the minted host is how the router is found,
	// since the custom name deliberately has no DNS.
	if err := newCmd(t, "deploy", "--builder=host", "--deployer=raw", "--expose=route").Run(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = newCmd(t, "delete").Run() })

	f, err := fn.NewFunction(root)
	if err != nil {
		t.Fatal(err)
	}
	ns := f.Deploy.Namespace
	minted := serviceAnnotations(t, ns, name)[k8s.RouteHostnameAnnotation]
	if minted == "" {
		t.Fatal("expected a minted hostname to locate the router with")
	}
	routerAddrs, err := net.LookupHost(minted)
	if err != nil || len(routerAddrs) == 0 {
		t.Fatalf("could not resolve the router via %q: %v", minted, err)
	}

	// The domain lands verbatim on the Route (a recreate: in-place host
	// updates are permission-gated) and in the record.
	if err := newCmd(t, "deploy", "--builder=host", "--deployer=raw", "--expose=route", "--domain="+domain).Run(); err != nil {
		t.Fatal(err)
	}
	if got := serviceAnnotations(t, ns, name)[k8s.RouteHostnameAnnotation]; got != domain {
		t.Fatalf("expected the custom domain %q recorded, got %q", domain, got)
	}
	route := routeFor(t, ns, name, ns)
	if host, _, _ := unstructured.NestedString(route.Object, "spec", "host"); host != domain {
		t.Fatalf("expected spec.host %q, got %q", domain, host)
	}

	// Play the certificate controller: inject a self-signed cert for the
	// domain, exactly as cert-manager's openshift-routes plugin would.
	certPEM, keyPEM, pool := selfSignedCert(t, domain)
	client, err := k8s.NewDynamicClient()
	if err != nil {
		t.Fatal(err)
	}
	if err := unstructured.SetNestedField(route.Object, certPEM, "spec", "tls", "certificate"); err != nil {
		t.Fatal(err)
	}
	if err := unstructured.SetNestedField(route.Object, keyPEM, "spec", "tls", "key"); err != nil {
		t.Fatal(err)
	}
	gvr := schema.GroupVersionResource{Group: "route.openshift.io", Version: "v1", Resource: "routes"}
	if _, err := client.Resource(gvr).Namespace(ns).Update(context.Background(), route, metav1.UpdateOptions{}); err != nil {
		t.Fatal(err)
	}

	// A redeploy must not wipe the injected material.
	if err := newCmd(t, "deploy", "--builder=host", "--deployer=raw", "--expose=route", "--domain="+domain).Run(); err != nil {
		t.Fatal(err)
	}
	route = routeFor(t, ns, name, ns)
	if cert, _, _ := unstructured.NestedString(route.Object, "spec", "tls", "certificate"); cert != certPEM {
		t.Fatal("expected the injected certificate to survive a redeploy")
	}

	// HTTPS through the router, DNS bypassed, trust anchored only at the
	// injected cert: a 200 here proves routing AND that the router serves
	// the user's certificate for the custom domain.
	dial := func(ctx context.Context, network, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, network, net.JoinHostPort(routerAddrs[0], "443"))
	}
	httpClient := &http.Client{
		Transport: &http.Transport{DialContext: dial, TLSClientConfig: &tls.Config{RootCAs: pool}},
		Timeout:   10 * time.Second,
	}
	deadline := time.Now().Add(90 * time.Second)
	for {
		resp, getErr := httpClient.Get("https://" + domain + "/")
		if getErr == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				break
			}
			getErr = fmt.Errorf("status %d", resp.StatusCode)
		}
		if time.Now().After(deadline) {
			t.Fatalf("the router never served the custom domain with the injected cert: %v", getErr)
		}
		time.Sleep(3 * time.Second)
	}
}

// TestExpose_RemoteRoute ensures the exposure chain holds when the deploy
// runs in-cluster: intent travels in func.yaml, the pipeline's func-util
// creates the Route, and the CLI records what the pipeline's describer read
// back rather than what was asked for. Presumes Tekton on the cluster, like
// every remote test, and a func-util image built from this source
// (make FUNC_UTILS_IMG=<img>): a published image that predates expose
// deploys cluster-local, which is exactly the drift this test catches.
//
//	func deploy --remote --deployer=raw --expose=route
func TestExpose_RemoteRoute(t *testing.T) {
	requiresOpenShift(t)

	name := "func-e2e-expose-remote"
	root := fromCleanEnv(t, name)

	if err := newCmd(t, "init", "-l=go").Run(); err != nil {
		t.Fatal(err)
	}
	if err := newCmd(t, "deploy", "--remote", "--builder=pack", "--registry="+Registry,
		"--deployer=raw", "--expose=route").Run(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = newCmd(t, "delete").Run() })

	f, err := fn.NewFunction(root)
	if err != nil {
		t.Fatal(err)
	}
	// Recorded from the cluster by the pipeline describer; empty here means
	// the pipeline ran a func-util that ignored the intent.
	if f.Deploy.Expose != fn.ExposeRoute {
		t.Fatalf("expected applied exposure %q read back from the cluster, got %q", fn.ExposeRoute, f.Deploy.Expose)
	}
	ns := f.Deploy.Namespace
	ann := serviceAnnotations(t, ns, name)
	if ann[k8s.RouteHostnameAnnotation] == "" {
		t.Error("expected the Service to record the exposed hostname")
	}
	if got := ann[k8s.RouteNamespaceAnnotation]; got != ns {
		t.Errorf("expected the Route recorded in the function's namespace %q, got %q", ns, got)
	}
	if n := routeCount(t, ns, name, ns); n != 1 {
		t.Fatalf("expected 1 Route in %q, found %d", ns, n)
	}
}

// TestExpose_RemoteKedaRoute is TestExpose_RemoteRoute for the keda deployer:
// the same pipeline/func-util/describer chain, but the Route lives in the
// interceptor's namespace and an HTTPScaledObject must exist or the
// interceptor 404s the public Host. The pipeline ServiceAccount has no
// rights there by default (edit is namespaced), so suite setup grants Route
// CRUD and Service reads in the interceptor namespace.
//
//	func deploy --remote --deployer=keda --expose=route
func TestExpose_RemoteKedaRoute(t *testing.T) {
	requiresOpenShift(t)

	name := "func-e2e-expose-remote-keda"
	root := fromCleanEnv(t, name)

	interceptorNS := liveInterceptorNamespace(t)
	grantPipelineSAInterceptorAccess(t, interceptorNS)

	if err := newCmd(t, "init", "-l=go").Run(); err != nil {
		t.Fatal(err)
	}
	if err := newCmd(t, "deploy", "--remote", "--builder=pack", "--registry="+Registry,
		"--deployer=keda", "--expose=route").Run(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = newCmd(t, "delete").Run() })

	f, err := fn.NewFunction(root)
	if err != nil {
		t.Fatal(err)
	}
	if f.Deploy.Deployer != "keda" {
		t.Fatalf("expected the keda deployer to be recorded, got %q", f.Deploy.Deployer)
	}
	if f.Deploy.Expose != fn.ExposeRoute {
		t.Fatalf("expected applied exposure %q read back from the cluster, got %q", fn.ExposeRoute, f.Deploy.Expose)
	}

	ns := f.Deploy.Namespace
	ann := serviceAnnotations(t, ns, name)
	if ann[k8s.RouteHostnameAnnotation] == "" {
		t.Error("expected the Service to record the exposed hostname")
	}
	if got := ann[k8s.RouteNamespaceAnnotation]; got != interceptorNS {
		t.Errorf("expected the Route recorded in the interceptor namespace %q, got %q", interceptorNS, got)
	}
	if interceptorNS == ns {
		t.Fatal("keda's Route must not land in the function's own namespace")
	}
	if n := routeCount(t, interceptorNS, name, ns); n != 1 {
		t.Fatalf("expected 1 Route in interceptor namespace %q, found %d", interceptorNS, n)
	}

	hsoClient, err := keda.NewHTTPScaledObjectClientset()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := hsoClient.HttpV1alpha1().HTTPScaledObjects(ns).Get(context.Background(), name, metav1.GetOptions{}); err != nil {
		t.Fatalf("expected an HTTPScaledObject in %s/%s: %v", ns, name, err)
	}
}

// liveInterceptorNamespace is where the interceptor Service actually is:
// openshift-keda on CMA, keda on the upstream chart. Skip if neither answers;
// this test has nothing to say without an interceptor.
func liveInterceptorNamespace(t *testing.T) string {
	t.Helper()
	// Same Service keda.interceptorNamespace looks for.
	const interceptorServiceName = "keda-add-ons-http-interceptor-proxy"
	clientset, err := k8s.NewKubernetesClientset()
	if err != nil {
		t.Fatal(err)
	}
	for _, ns := range []string{"openshift-keda", "keda"} {
		_, err := clientset.CoreV1().Services(ns).Get(context.Background(), interceptorServiceName, metav1.GetOptions{})
		if err == nil {
			return ns
		}
	}
	t.Skip("keda interceptor Service not found in openshift-keda or keda")
	return ""
}

var grantPipelineInterceptorOnce sync.Once

// grantPipelineSAInterceptorAccess gives the pipeline ServiceAccount (and
// default, for upstream Tekton) Route CRUD plus Service reads in the
// interceptor namespace. edit is namespaced and does not reach openshift-keda.
func grantPipelineSAInterceptorAccess(t *testing.T, interceptorNS string) {
	t.Helper()
	grantPipelineInterceptorOnce.Do(func() {
		clientset, err := k8s.NewKubernetesClientset()
		if err != nil {
			t.Fatal(err)
		}
		role := &rbacv1.Role{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "func-e2e-pipeline-interceptor-routes",
				Namespace: interceptorNS,
			},
			Rules: []rbacv1.PolicyRule{
				{
					APIGroups: []string{"route.openshift.io"},
					Resources: []string{"routes", "routes/status", "routes/custom-host"},
					Verbs:     []string{"get", "list", "watch", "create", "update", "patch", "delete"},
				},
				{
					APIGroups: []string{""},
					Resources: []string{"services"},
					Verbs:     []string{"get", "list", "watch"},
				},
			},
		}
		if _, err := clientset.RbacV1().Roles(interceptorNS).Create(context.Background(), role, metav1.CreateOptions{}); err != nil && !apierrors.IsAlreadyExists(err) {
			t.Fatalf("creating interceptor Route Role: %v", err)
		}
		binding := &rbacv1.RoleBinding{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "func-e2e-pipeline-interceptor-routes",
				Namespace: interceptorNS,
			},
			Subjects: []rbacv1.Subject{
				{Kind: "ServiceAccount", Name: "pipeline", Namespace: Namespace},
				{Kind: "ServiceAccount", Name: "default", Namespace: Namespace},
			},
			RoleRef: rbacv1.RoleRef{
				APIGroup: "rbac.authorization.k8s.io",
				Kind:     "Role",
				Name:     "func-e2e-pipeline-interceptor-routes",
			},
		}
		if _, err := clientset.RbacV1().RoleBindings(interceptorNS).Create(context.Background(), binding, metav1.CreateOptions{}); err != nil && !apierrors.IsAlreadyExists(err) {
			t.Fatalf("creating interceptor Route RoleBinding: %v", err)
		}
	})
}

// Not covered here, deliberately:
//
//   - Route object shape, admission, and the refusal to adopt a hand-authored
//     Route. Those assert on cluster objects rather than on the CLI contract,
//     so they belong in pkg/ocproute and pkg/keda integration tests, which
//     gate on IsOpenShift() the same way.
//   - Anything involving an account without rights in the interceptor's
//     namespace. Every permission-shaped defect in this feature was found by
//     hand because no rig here builds a restricted Role; exercising one needs
//     a cluster with a deliberately limited account.
