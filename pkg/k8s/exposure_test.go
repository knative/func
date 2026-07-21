package k8s

import (
	"context"
	goerrors "errors"
	"strings"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	fakediscovery "k8s.io/client-go/discovery/fake"
	fakeclientset "k8s.io/client-go/kubernetes/fake"
	ktesting "k8s.io/client-go/testing"
	fn "knative.dev/func/pkg/functions"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwclientset "sigs.k8s.io/gateway-api/pkg/client/clientset/versioned"
)

// markGatewayAPIInstalled makes the fake cluster's discovery report the
// Gateway API HTTPRoute resource as installed, so GatewayAPIAvailable()
// returns (true, nil) against it.
func markGatewayAPIInstalled(kubeFake *fakeclientset.Clientset) {
	kubeFake.Discovery().(*fakediscovery.FakeDiscovery).Resources = []*metav1.APIResourceList{
		{
			GroupVersion: gatewayAPIGroupVersion,
			APIResources: []metav1.APIResource{{Kind: "HTTPRoute"}},
		},
	}
}

// --- discovery-miss: no provisioning, hard actionable error -----------------

// TestEnsureExposure_NoEligibleGatewayIsHardError proves that func never
// provisions a Gateway: a cluster-wide discovery miss (zero
// eligible Gateways, zero GatewayClasses or not) is unconditionally a hard
// error naming the remaining options (pin, ask an admin using the manifest
// example, or opt out).
func TestEnsureExposure_NoEligibleGatewayIsHardError(t *testing.T) {
	ctx := context.Background()
	kubeFake := fakeclientset.NewClientset()
	markGatewayAPIInstalled(kubeFake)
	gwFake := newGwFake() // zero Gateways, zero GatewayClasses

	d := NewDeployer()
	f := fn.Function{Name: "myfunc", Deploy: fn.DeploySpec{Deployer: KubernetesDeployerName}}

	_, err := d.ensureExposure(ctx, f, "default", kubeFake, gwFake, "")
	if err == nil {
		t.Fatal("expected a hard error when no eligible Gateway is found, got nil")
	}
	if !strings.Contains(err.Error(), "no eligible gateway found on this cluster") {
		t.Errorf("expected the discovery-miss message, got: %v", err)
	}
	if !strings.Contains(err.Error(), "func deploy --expose=gateway:<namespace>/<name>") {
		t.Errorf("expected the pin-a-Gateway option, got: %v", err)
	}
	if !strings.Contains(err.Error(), "gatewayClassName: <run: kubectl get gatewayclass>") {
		t.Errorf("expected the minimal ready-to-apply manifest example, got: %v", err)
	}
	if !strings.Contains(err.Error(), "func deploy --expose=none") {
		t.Errorf("expected the cluster-local opt-out option, got: %v", err)
	}
}

// --- annotation lifecycle ---------------------------------------------------

func TestWriteRouteHostnameAnnotation_Set(t *testing.T) {
	ctx := context.Background()
	kubeFake := fakeclientset.NewClientset(&corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "myfunc", Namespace: "default"},
	})

	if err := writeRouteHostnameAnnotation(ctx, kubeFake, "default", "myfunc", "myfunc.default.example.com"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	svc, err := kubeFake.CoreV1().Services("default").Get(ctx, "myfunc", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if svc.Annotations[RouteHostnameAnnotation] != "myfunc.default.example.com" {
		t.Errorf("expected annotation to be set, got %v", svc.Annotations)
	}
}

func TestWriteRouteHostnameAnnotation_Clear(t *testing.T) {
	ctx := context.Background()
	kubeFake := fakeclientset.NewClientset(&corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "myfunc",
			Namespace:   "default",
			Annotations: map[string]string{RouteHostnameAnnotation: "myfunc.default.example.com"},
		},
	})

	if err := writeRouteHostnameAnnotation(ctx, kubeFake, "default", "myfunc", ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	svc, err := kubeFake.CoreV1().Services("default").Get(ctx, "myfunc", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := svc.Annotations[RouteHostnameAnnotation]; ok {
		t.Errorf("expected annotation to be cleared, got %v", svc.Annotations)
	}
}

func TestWriteRouteHostnameAnnotation_ClearMissingServiceIsNotError(t *testing.T) {
	ctx := context.Background()
	kubeFake := fakeclientset.NewClientset()

	if err := writeRouteHostnameAnnotation(ctx, kubeFake, "default", "missing", ""); err != nil {
		t.Fatalf("expected nil error for a missing service, got: %v", err)
	}
}

func TestWriteRouteHostnameAnnotation_SetMissingServiceIsError(t *testing.T) {
	ctx := context.Background()
	kubeFake := fakeclientset.NewClientset()

	if err := writeRouteHostnameAnnotation(ctx, kubeFake, "default", "missing", "missing.default.example.com"); err == nil {
		t.Fatal("expected an error recording a hostname against a nonexistent service, got nil")
	}
}

func Test_generateService_CarriesOverRouteHostnameAnnotation(t *testing.T) {
	d := &Deployer{}
	f := fn.Function{Name: "myfunc"}
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "myfunc", Namespace: "default", UID: types.UID("uid")},
	}
	existing := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{RouteHostnameAnnotation: "myfunc.default.example.com"},
		},
	}

	svc, err := d.generateService(f, "default", false, deployment, existing)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if svc.Annotations[RouteHostnameAnnotation] != "myfunc.default.example.com" {
		t.Errorf("expected the annotation to be carried over from the existing service, got %v", svc.Annotations)
	}
}

func Test_generateService_CreateHasNoExposureAnnotation(t *testing.T) {
	d := &Deployer{}
	f := fn.Function{Name: "myfunc"}
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "myfunc", Namespace: "default", UID: types.UID("uid")},
	}

	svc, err := d.generateService(f, "default", false, deployment, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := svc.Annotations[RouteHostnameAnnotation]; ok {
		t.Errorf("expected no exposure annotation on a fresh create (nil existingService), got %v", svc.Annotations)
	}
}

// --- reconcileExposure()'s Exposed signal ------------------------------------
//
// fn.DeploymentResult.Exposed drives client.go's conditional deploy-success
// wording ("exposed at URL" vs "reachable in-cluster only"); these prove the
// raw deployer sets it correctly for the three reconcileExposure() branches.

func TestReconcileExposure_ExposedSignal(t *testing.T) {
	ctx := context.Background()

	t.Run("expose:none sets Exposed=false with the cluster-local URL", func(t *testing.T) {
		kubeFake := fakeclientset.NewClientset()
		gwFake := newGwFake()
		d := NewDeployer()
		f := fn.Function{Name: "myfunc", Deploy: fn.DeploySpec{Deployer: KubernetesDeployerName, Expose: "none"}}

		url, exposed, err := d.reconcileExposure(ctx, f, "default", kubeFake, gwFake)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if exposed {
			t.Error("expected Exposed=false for expose:none")
		}
		if url != "http://myfunc.default.svc" {
			t.Errorf("expected the cluster-local URL, got %q", url)
		}
	})

	t.Run("exposure-disabled deployer (embedded keda) sets Exposed=false", func(t *testing.T) {
		kubeFake := fakeclientset.NewClientset()
		gwFake := newGwFake()
		d := NewDeployer(WithDeployerExposureDisabled())
		f := fn.Function{Name: "myfunc", Deploy: fn.DeploySpec{Deployer: "keda"}}

		url, exposed, err := d.reconcileExposure(ctx, f, "default", kubeFake, gwFake)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if exposed {
			t.Error("expected Exposed=false for the embedded/deployer-switch path")
		}
		if url != "http://myfunc.default.svc" {
			t.Errorf("expected the cluster-local URL, got %q", url)
		}
	})

	t.Run("successful gateway exposure sets Exposed=true", func(t *testing.T) {
		// the exposure chain needs both of these to already exist: the
		// Deployment becomes the HTTPRoute's owner, and the Service receives
		// the hostname annotation at the very end
		kubeFake := fakeclientset.NewClientset(
			&appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "myfunc", Namespace: "default"}},
			&corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "myfunc", Namespace: "default"}},
		)
		// set GatewayAPI as installed - groupVersion + API resource HTTPRoute
		markGatewayAPIInstalled(kubeFake)

		gwFake := newGwFake()
		// put gateway into a in-memory bucket
		seedGateway(t, gwFake, withIPAddresses(newGateway("gw", "infra", true, httpListenerFrom(gwv1.NamespacesFromAll)), "172.18.0.5"))

		// This reactor plays the gateway controller, which a fake cluster
		// doesn't have. The route gets fetched twice: first EnsureHTTPRoute()
		// asks "does it exist yet?" - we return false -> stay out of that one
		// so the store honestly answers "no HTTPRoute resource in bucket" and
		// the code really creates the route. Then WaitForRouteAccepted() starts
		// polling for a verdict; those reads we answer ourselves with an
		// already-accepted route, exactly what a live controller would have
		// written, so the test doesn't sit out the acceptance timeout.
		getCalls := 0
		gwFake.PrependReactor("get", "httproutes", func(action ktesting.Action) (bool, runtime.Object, error) {
			getCalls++
			if getCalls == 1 {
				return false, nil, nil
			}
			// forge the route as a GET would see it after a live controller
			// accepted it
			return true, newHTTPRoute("myfunc", "default", 1,
				parentStatus("gw", "infra", metav1.ConditionTrue, "Accepted", "ok", 0)), nil
		})

		d := NewDeployer()
		f := fn.Function{Name: "myfunc", Deploy: fn.DeploySpec{Deployer: KubernetesDeployerName, Expose: "gateway"}}

		// From here it's all real production code. reconcileExposure() sees
		// expose=gateway and hands off to ensureExposure(), which walks the
		// whole chain: checks the Gateway API is installed (faked above),
		// discovers the seeded gateway, picks its listener and mints the
		// sslip hostname from the IP, creates the HTTPRoute (the reactor's
		// first read, then a real create), waits for acceptance (the
		// reactor's later reads), and finally stamps the hostname onto the
		// Service.
		url, exposed, err := d.reconcileExposure(ctx, f, "default", kubeFake, gwFake)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !exposed {
			t.Error("expected Exposed=true for a successful gateway exposure")
		}
		if !strings.Contains(url, "172.18.0.5") {
			t.Errorf("expected the sslip URL derived from the Gateway's IP, got %q", url)
		}
	})
}

// --- removeExposure(): Forbidden semantics differ by enforce -----------------

// TestRemoveExposure_ForbiddenSemantics proves the two removal directions
// treat a Forbidden GET on the HTTPRoute differently: the keda-switch path
// (enforce=false) was Gateway-API-RBAC-free before this feature existed and
// must stay green, so it warns and continues; the expose:none path
// (enforce=true) is the user's explicit unexposure demand, so an inability
// to verify/remove is the honest hard-error outcome.
func TestRemoveExposure_ForbiddenSemantics(t *testing.T) {
	ctx := context.Background()
	forbiddenGwFake := func() gwclientset.Interface {
		gwFake := newGwFake()
		gwFake.PrependReactor("get", "httproutes", func(action ktesting.Action) (bool, runtime.Object, error) {
			return true, nil, apierrors.NewForbidden(gwv1.Resource("httproutes"), "myfunc", goerrors.New("rbac"))
		})
		return gwFake
	}

	t.Run("keda-switch path (enforce=false): Forbidden warns and continues", func(t *testing.T) {
		d := NewDeployer()
		err := d.removeExposure(ctx, fakeclientset.NewClientset(), forbiddenGwFake(), "default", "myfunc", false)
		if err != nil {
			t.Fatalf("expected nil error (warn-and-continue) for Forbidden under enforce=false, got: %v", err)
		}
	})

	t.Run("expose:none path (enforce=true): Forbidden is a hard error", func(t *testing.T) {
		d := NewDeployer()
		err := d.removeExposure(ctx, fakeclientset.NewClientset(), forbiddenGwFake(), "default", "myfunc", true)
		if err == nil {
			t.Fatal("expected a hard error for Forbidden under enforce=true, got nil")
		}
	})
}
