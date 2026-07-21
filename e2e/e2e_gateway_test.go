//go:build e2e

package e2e

import (
	"fmt"
	"os/exec"
	"strings"
	"testing"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	gwclientset "sigs.k8s.io/gateway-api/pkg/client/clientset/versioned"

	"knative.dev/func/pkg/k8s"
)

// TestGateway_ExposureHappyPath exercises the raw deployer's default Gateway
// API exposure end to end: deploy, assert the HTTPRoute reaches
// Accepted=True, reach the function THROUGH the gateway from inside the
// cluster, then delete and confirm the HTTPRoute is garbage-collected via
// its Deployment ownerRef.
//
// The Gateway's address and the minted hostname are both read back from the
// live cluster/CLI rather than assumed: the address comes from the
// Gateway's own status.addresses after Programmed=True, and the hostname
// comes from func's own 'describe --output url', which reflects exactly
// what ResolveHostname minted from that address (see pkg/k8s/httproute.go).
// This keeps the test valid across clusters with different address-pool
// ranges, and proves the exact URL func told the user actually works - an
// in-cluster curl is the only reachability check trusted here, since on
// kind/WSL a host-to-LoadBalancer-IP connection is known-intermittent and a
// host-side failure is not a feature defect.
func TestGateway_ExposureHappyPath(t *testing.T) {
	ctx := t.Context()

	// fromCleanEnv (via setupEnv) sets KUBECONFIG to the E2E test cluster -
	// must run before any client is constructed, or GetClientConfig() below
	// falls back to the default kubeconfig (a different cluster entirely).
	functionName := "func-e2e-test-gateway"
	fromCleanEnv(t, functionName)

	restConfig, err := k8s.GetClientConfig().ClientConfig()
	if err != nil {
		t.Fatal(err)
	}
	clientset, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		t.Fatal(err)
	}
	gwClient, err := gwclientset.NewForConfig(restConfig)
	if err != nil {
		t.Fatal(err)
	}

	available, err := k8s.GatewayAPIAvailable(clientset)
	if err != nil {
		t.Fatal(err)
	}
	if !available {
		t.Skip("Gateway API is not installed on this cluster; clusters built by " +
			"'func cluster create' or hack/cluster.sh install it as part of networking - rebuild with either to run this test")
	}

	if err := newCmd(t, "init", "-l=go", "-t=http").Run(); err != nil {
		t.Fatal(err)
	}

	// Default expose is "gateway" (cluster-wide auto-discovery); deploy with
	// the raw deployer so the exposure reconciliation path is exercised
	// (see pkg/k8s/deployer.go reconcileExposure/ensureExposure).
	if err := newCmd(t, "deploy", "--deployer", "raw").Run(); err != nil {
		t.Fatal(err)
	}
	defer clean(t, functionName, Namespace)

	waitForDeployment(t, Namespace, functionName)

	gwName, gwNamespace := resolveFunctionGateway(t, gwClient, functionName, Namespace)
	t.Logf("HTTPRoute is attached to Gateway %s/%s", gwNamespace, gwName)

	if err := k8s.WaitForRouteAccepted(ctx, gwClient, Namespace, functionName, gwName, gwNamespace, 60*time.Second); err != nil {
		t.Fatalf("HTTPRoute was not accepted: %v", err)
	}
	t.Log("HTTPRoute Accepted=True")

	gwAddress := gatewayAddress(t, gwClient, gwNamespace, gwName)
	hostname := mintedHostname(t, functionName)
	t.Logf("Gateway address %s, minted hostname %s", gwAddress, hostname)

	curlThroughGateway(t, gwAddress, hostname)

	if err := newCmd(t, "delete", functionName, "--namespace", Namespace).Run(); err != nil {
		t.Fatal(err)
	}

	waitForHTTPRouteGone(t, gwClient, Namespace, functionName)
}

// TestGateway_ExposureCustomDomain exercises the raw deployer's --domain
// flow: ResolveHostname's f.Domain branch mints <name>.<namespace>.<domain>
// instead of an sslip.io hostname off the Gateway's IP (see
// pkg/k8s/httproute.go ResolveHostname). Reached with a plain HOST-side
// curl, not an in-cluster pod - deterministic here, unlike the sslip/
// LoadBalancer-IP path TestGateway_ExposureHappyPath curls in-cluster only:
// localtest.me is a public wildcard DNS name resolving to 127.0.0.1, and
// kind's host port mapping (80 -> the shared envoy Service) is a fixed
// forward, not a per-cluster LoadBalancer IP that can flake on kind/WSL -
// the same stable path the Knative e2e tests already host-curl through.
func TestGateway_ExposureCustomDomain(t *testing.T) {
	ctx := t.Context()

	functionName := "func-e2e-test-gateway-domain"
	fromCleanEnv(t, functionName)

	restConfig, err := k8s.GetClientConfig().ClientConfig()
	if err != nil {
		t.Fatal(err)
	}
	clientset, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		t.Fatal(err)
	}
	gwClient, err := gwclientset.NewForConfig(restConfig)
	if err != nil {
		t.Fatal(err)
	}

	available, err := k8s.GatewayAPIAvailable(clientset)
	if err != nil {
		t.Fatal(err)
	}
	if !available {
		t.Skip("Gateway API is not installed on this cluster; clusters built by " +
			"'func cluster create' or hack/cluster.sh install it as part of networking - rebuild with either to run this test")
	}

	if err := newCmd(t, "init", "-l=go", "-t=http").Run(); err != nil {
		t.Fatal(err)
	}

	if err := newCmd(t, "deploy", "--deployer", "raw", "--domain", "localtest.me").Run(); err != nil {
		t.Fatal(err)
	}
	defer clean(t, functionName, Namespace)

	waitForDeployment(t, Namespace, functionName)

	wantHostname := fmt.Sprintf("%s.%s.localtest.me", functionName, Namespace)
	if gotHostname := mintedHostname(t, functionName); gotHostname != wantHostname {
		t.Fatalf("minted hostname = %q, want %q", gotHostname, wantHostname)
	}

	gwName, gwNamespace := resolveFunctionGateway(t, gwClient, functionName, Namespace)
	if err := k8s.WaitForRouteAccepted(ctx, gwClient, Namespace, functionName, gwName, gwNamespace, 60*time.Second); err != nil {
		t.Fatalf("HTTPRoute was not accepted: %v", err)
	}

	if !waitFor(t, "http://"+wantHostname) {
		t.Fatalf("host-side curl to %s never succeeded", wantHostname)
	}
	t.Logf("host-side curl to %s succeeded", wantHostname)

	if err := newCmd(t, "delete", functionName, "--namespace", Namespace).Run(); err != nil {
		t.Fatal(err)
	}

	waitForHTTPRouteGone(t, gwClient, Namespace, functionName)
}

// resolveFunctionGateway reads the Gateway that the function's own
// HTTPRoute targets (spec.ParentRefs, set by func at generation time - see
// GenerateHTTPRoute) rather than assuming a name, so this works regardless
// of how many Gateways exist on the cluster.
func resolveFunctionGateway(t *testing.T, gwClient gwclientset.Interface, functionName, namespace string) (gwName, gwNamespace string) {
	t.Helper()

	route, err := gwClient.GatewayV1().HTTPRoutes(namespace).Get(t.Context(), functionName, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("failed to get HTTPRoute %q: %v", functionName, err)
	}
	if len(route.Spec.ParentRefs) == 0 {
		t.Fatalf("HTTPRoute %q has no parentRefs", functionName)
	}

	ref := route.Spec.ParentRefs[0]
	gwName = string(ref.Name)
	gwNamespace = namespace
	if ref.Namespace != nil && *ref.Namespace != "" {
		gwNamespace = string(*ref.Namespace)
	}
	return gwName, gwNamespace
}

// gatewayAddress reads back the Gateway's own status.addresses - the
// address MetalLB actually handed out, whatever pool/range that came from.
func gatewayAddress(t *testing.T, gwClient gwclientset.Interface, namespace, name string) string {
	t.Helper()

	gw, err := gwClient.GatewayV1().Gateways(namespace).Get(t.Context(), name, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("failed to get Gateway %s/%s: %v", namespace, name, err)
	}
	if len(gw.Status.Addresses) == 0 {
		t.Fatalf("Gateway %s/%s has no status.addresses", namespace, name)
	}
	return gw.Status.Addresses[0].Value
}

// mintedHostname reads the hostname func itself minted for the function, via
// 'func describe --output url' (which reflects the RouteHostnameAnnotation
// written at exposure time - see pkg/k8s/describer.go). Reading it back
// rather than recomputing it independently proves the exact URL func told
// the user is the one that works.
func mintedHostname(t *testing.T, functionName string) string {
	t.Helper()

	cmd := exec.Command(Bin, "describe", functionName, "--namespace", Namespace, "--output", "url")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("func describe --output url failed: %v", err)
	}

	url := strings.TrimSpace(string(out))
	hostname := strings.TrimPrefix(strings.TrimPrefix(url, "http://"), "https://")
	if hostname == "" || hostname == url {
		t.Fatalf("could not parse a hostname from describe output: %q", url)
	}
	return hostname
}

// curlThroughGateway is the authoritative reachability check: it runs a
// one-shot curl Pod inside the cluster against the Gateway's address,
// setting the Host header to the hostname func minted. A host-side
// (kind/WSL) connection to the Gateway's LoadBalancer IP is known
// intermittent and is never asserted here - only the in-cluster path is
// trusted.
func curlThroughGateway(t *testing.T, gatewayAddress, hostname string) {
	t.Helper()

	deadline := time.Now().Add(2 * time.Minute)
	var lastErr error
	var lastOutput []byte

	for attempt := 0; time.Now().Before(deadline); attempt++ {
		podName := fmt.Sprintf("func-e2e-gateway-curl-%d", attempt)
		cmd := exec.Command("kubectl", "run", podName,
			"--namespace", Namespace,
			"--image=curlimages/curl",
			"--restart=Never",
			"--rm", "-i",
			"--command", "--",
			"curl", "-sS", "-f", "--max-time", "10",
			"-H", "Host: "+hostname,
			fmt.Sprintf("http://%s/", gatewayAddress))

		out, err := cmd.CombinedOutput()
		if err == nil {
			t.Logf("curl through gateway succeeded: %s", strings.TrimSpace(string(out)))
			return
		}
		lastErr, lastOutput = err, out
		time.Sleep(5 * time.Second)
	}
	t.Fatalf("curl through gateway %s (Host: %s) never succeeded: %v\noutput: %s",
		gatewayAddress, hostname, lastErr, lastOutput)
}

// waitForHTTPRouteGone polls until the function's HTTPRoute is deleted,
// confirming that Kubernetes garbage-collected it via its Deployment
// ownerRef (func's raw deployer delete path never touches the HTTPRoute
// directly - see pkg/k8s/remover.go). GC is asynchronous, hence the
// generous poll rather than an immediate assertion.
func waitForHTTPRouteGone(t *testing.T, gwClient gwclientset.Interface, namespace, name string) {
	t.Helper()

	deadline := time.Now().Add(2 * time.Minute)
	for time.Now().Before(deadline) {
		_, err := gwClient.GatewayV1().HTTPRoutes(namespace).Get(t.Context(), name, metav1.GetOptions{})
		if apierrors.IsNotFound(err) {
			t.Log("HTTPRoute was garbage-collected")
			return
		}
		time.Sleep(5 * time.Second)
	}
	t.Fatalf("HTTPRoute %s/%s was not garbage-collected within timeout", namespace, name)
}
