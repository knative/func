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
	"strings"
	"testing"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	k8slabels "k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"

	fn "knative.dev/func/pkg/functions"
	"knative.dev/func/pkg/k8s"
	fnlabels "knative.dev/func/pkg/k8s/labels"
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
//	func deploy --expose=route
func TestExpose_RouteRequiresOpenShift(t *testing.T) {
	requiresNotOpenShift(t)

	name := "func-e2e-test-expose-gate"
	root := fromCleanEnv(t, name)

	if err := newCmd(t, "init", "-l=go").Run(); err != nil {
		t.Fatal(err)
	}

	out, err := newCmdOutput(t, "deploy", "--deployer=raw", "--expose=route").CombinedOutput()
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

// TestExpose_ClusterLocalByDefault ensures a function deployed without the
// flag is cluster-local, and stays that way in the record. This is the
// behaviour change most likely to surprise: earlier builds exposed by default
// on OpenShift.
//
//	func deploy
func TestExpose_ClusterLocalByDefault(t *testing.T) {
	name := "func-e2e-test-expose-default"
	root := fromCleanEnv(t, name)

	if err := newCmd(t, "init", "-l=go").Run(); err != nil {
		t.Fatal(err)
	}
	if err := newCmd(t, "deploy", "--deployer=raw").Run(); err != nil {
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

// TestExpose_NonePersistsAsIntent ensures an explicit opt-out is recorded as
// intent and survives a redeploy that does not repeat the flag, since the
// flag defaults to the persisted value.
//
//	func deploy --expose=none
func TestExpose_NonePersistsAsIntent(t *testing.T) {
	name := "func-e2e-test-expose-none"
	root := fromCleanEnv(t, name)

	if err := newCmd(t, "init", "-l=go").Run(); err != nil {
		t.Fatal(err)
	}
	if err := newCmd(t, "deploy", "--deployer=raw", "--expose=none").Run(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = newCmd(t, "delete").Run() })

	f, err := fn.NewFunction(root)
	if err != nil {
		t.Fatal(err)
	}
	if f.Expose != fn.ExposeNone {
		t.Fatalf("expected intent %q, got %q", fn.ExposeNone, f.Expose)
	}
	// Status records what was applied, and cluster-local applies nothing.
	if f.Deploy.Expose != "" {
		t.Errorf("expected no exposure applied, got %q", f.Deploy.Expose)
	}

	// Redeploy without the flag: intent must survive.
	if err := newCmd(t, "deploy", "--deployer=raw").Run(); err != nil {
		t.Fatal(err)
	}
	if f, err = fn.NewFunction(root); err != nil {
		t.Fatal(err)
	}
	if f.Expose != fn.ExposeNone {
		t.Errorf("expected intent %q to round-trip, got %q", fn.ExposeNone, f.Expose)
	}
}

// TestExpose_KedaRejectsLongName ensures a function name that is legal on its
// own but too long once keda's bridge suffix is added is refused up front,
// rather than by an opaque API rejection after the Deployment already exists.
//
//	func deploy --deployer=keda
func TestExpose_KedaRejectsLongName(t *testing.T) {
	// 45 characters: legal as a function name (DNS-1035 allows 63), one over
	// what keda's "-interceptor-bridge" suffix leaves room for.
	name := "func-e2e-test-expose-name-far-too-long-for-ke"
	if len(name) != 45 {
		t.Fatalf("test setup: expected a 45 character name, got %d", len(name))
	}
	fromCleanEnv(t, name)

	if err := newCmd(t, "init", "-l=go").Run(); err != nil {
		t.Fatal(err)
	}

	out, err := newCmdOutput(t, "deploy", "--deployer=keda").CombinedOutput()
	if err == nil {
		t.Fatal("expected a name too long for keda's bridge Service to be refused")
	}
	if !strings.Contains(string(out), "too long") {
		t.Errorf("expected the error to explain the length limit, got:\n%s", out)
	}
}

// TestExpose_Route ensures the full raw exposure round trip on OpenShift:
// a Route is created, the function answers on it, and turning exposure off
// takes it away again.
//
//	func deploy --expose=route ; func deploy --expose=none
func TestExpose_Route(t *testing.T) {
	requiresOpenShift(t)

	name := "func-e2e-test-expose-route"
	root := fromCleanEnv(t, name)

	if err := newCmd(t, "init", "-l=go").Run(); err != nil {
		t.Fatal(err)
	}
	if err := newCmd(t, "deploy", "--deployer=raw", "--expose=route").Run(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = newCmd(t, "delete").Run() })

	f, err := fn.NewFunction(root)
	if err != nil {
		t.Fatal(err)
	}
	if f.Expose != fn.ExposeRoute {
		t.Errorf("expected intent %q, got %q", fn.ExposeRoute, f.Expose)
	}
	if f.Deploy.Expose != fn.ExposeRoute {
		t.Errorf("expected applied exposure %q, got %q", fn.ExposeRoute, f.Deploy.Expose)
	}

	// The raw deployer's Route sits beside its function, and the Service
	// records both halves of the exposure.
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

	// Turning it off must take the Route away, clear the records, and leave
	// the function running.
	if err := newCmd(t, "deploy", "--deployer=raw", "--expose=none").Run(); err != nil {
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
}

// TestExpose_KedaRoute ensures a keda function can be exposed and unexposed
// through the CLI. Keda's Route is the one nothing garbage collects, so the
// round trip matters more here than for the raw deployer: a Route left behind
// by an opt-out survives forever.
//
//	func deploy --deployer=keda --expose=route
func TestExpose_KedaRoute(t *testing.T) {
	requiresOpenShift(t)

	name := "func-e2e-test-expose-keda"
	root := fromCleanEnv(t, name)

	if err := newCmd(t, "init", "-l=go").Run(); err != nil {
		t.Fatal(err)
	}
	if err := newCmd(t, "deploy", "--deployer=keda", "--expose=route").Run(); err != nil {
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
	if err := newCmd(t, "deploy", "--deployer=keda", "--expose=none").Run(); err != nil {
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
//	func deploy --deployer=keda --expose=route ; --expose=none ; --expose=route
func TestExpose_KedaToggle(t *testing.T) {
	requiresOpenShift(t)

	name := "func-e2e-expose-keda-toggle"
	root := fromCleanEnv(t, name)

	if err := newCmd(t, "init", "-l=go").Run(); err != nil {
		t.Fatal(err)
	}
	if err := newCmd(t, "deploy", "--deployer=keda", "--expose=route").Run(); err != nil {
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
	if err := newCmd(t, "deploy", "--deployer=keda", "--expose=none").Run(); err != nil {
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
	if err := newCmd(t, "deploy", "--deployer=keda", "--expose=route").Run(); err != nil {
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
//	func deploy --deployer=keda --expose=route ; func delete
func TestExpose_KedaDeleteCleansRoute(t *testing.T) {
	requiresOpenShift(t)

	name := "func-e2e-expose-keda-delete"
	root := fromCleanEnv(t, name)

	if err := newCmd(t, "init", "-l=go").Run(); err != nil {
		t.Fatal(err)
	}
	if err := newCmd(t, "deploy", "--deployer=keda", "--expose=route").Run(); err != nil {
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
//	func deploy --deployer=raw --expose=route --domain=<custom>
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
	if err := newCmd(t, "deploy", "--deployer=raw", "--expose=route").Run(); err != nil {
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
	if err := newCmd(t, "deploy", "--deployer=raw", "--expose=route", "--domain="+domain).Run(); err != nil {
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
	if err := newCmd(t, "deploy", "--deployer=raw", "--expose=route", "--domain="+domain).Run(); err != nil {
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

// Not covered here, deliberately:
//
//   - Route object shape, admission, and the refusal to adopt a hand-authored
//     Route. Those assert on cluster objects rather than on the CLI contract,
//     so they belong in pkg/ocproute and pkg/keda integration tests, which
//     gate on IsOpenShift() the same way.
//   - Anything involving an account without rights in the interceptor's
//     namespace. Every permission-shaped defect in this feature was found by
//     hand because no rig builds a restricted Role. See section 7 of
//     records/test-plan-exposure.md.
