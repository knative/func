package keda

import (
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	fn "knative.dev/func/pkg/functions"
	"knative.dev/func/pkg/k8s"
)

// Test_interceptorNamespace asserts openshift-keda and keda stay distinct and
// that interceptorNamespace returns the one matching the cluster type. Both
// branches are forced rather than read from the ambient kubeconfig, so
// openshift-keda is proven on CI too, where detection always reports
// not-OpenShift and that branch would otherwise never run.
func Test_interceptorNamespace(t *testing.T) {
	if interceptorNamespaceOpenShift == interceptorNamespaceUpstream {
		t.Fatal("the two install namespaces must differ, or resolving between them is pointless")
	}
	if interceptorNamespaceOpenShift != "openshift-keda" {
		t.Errorf("expected the Custom Metrics Autoscaler namespace, got %q", interceptorNamespaceOpenShift)
	}
	if interceptorNamespaceUpstream != "keda" {
		t.Errorf("expected the upstream helm chart namespace, got %q", interceptorNamespaceUpstream)
	}

	// Not parallel: SetOpenShiftForTest mutates a package-level bool without a
	// mutex. See openshift.go:SetOpenShiftForTest.
	for _, tt := range []struct {
		name      string
		openShift bool
		want      string
	}{
		{"OpenShift runs the Custom Metrics Autoscaler", true, interceptorNamespaceOpenShift},
		{"elsewhere runs the upstream helm chart", false, interceptorNamespaceUpstream},
	} {
		t.Run(tt.name, func(t *testing.T) {
			defer k8s.SetOpenShiftForTest(tt.openShift)()
			if got := interceptorNamespace(); got != tt.want {
				t.Errorf("expected %q, got %q", tt.want, got)
			}
		})
	}
}

// Test_interceptorBridgeService asserts the bridge Service is an ExternalName
// addressing the interceptor in interceptorNamespace, sits in the function's
// own namespace, and is owned by the function's Deployment.
func Test_interceptorBridgeService(t *testing.T) {
	d := NewDeployer()
	f := fn.Function{Name: "f", Runtime: "go"}
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "f", Namespace: "fn-ns", UID: "deployment-uid"},
	}

	// Full literals, never interceptorNamespace(): deriving the expectation from
	// the code under test would pass just as happily if the resolver returned
	// the wrong namespace. The non-OpenShift value must stay byte-identical to
	// the constant this rung replaced, or the KinD keda path regresses.
	// Not parallel: SetOpenShiftForTest mutates a package-level bool without a
	// mutex. See openshift.go:SetOpenShiftForTest.
	for _, tt := range []struct {
		name      string
		openShift bool
		want      string
	}{
		{"OpenShift", true, "keda-add-ons-http-interceptor-proxy.openshift-keda.svc.cluster.local"},
		{"elsewhere", false, "keda-add-ons-http-interceptor-proxy.keda.svc.cluster.local"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			defer k8s.SetOpenShiftForTest(tt.openShift)()
			if got := d.interceptorBridgeService(f, "fn-ns", deployment).Spec.ExternalName; got != tt.want {
				t.Errorf("expected the bridge to address %q, got %q", tt.want, got)
			}
		})
	}

	svc := d.interceptorBridgeService(f, "fn-ns", deployment)

	if svc.Spec.Type != corev1.ServiceTypeExternalName {
		t.Errorf("expected an ExternalName Service, got %q", svc.Spec.Type)
	}
	// The bridge lives in the FUNCTION's namespace, not the interceptor's - it
	// is the function's own entrypoint, and it is ownerRef'd to the function's
	// Deployment for GC, which only works same-namespace.
	if svc.Namespace != "fn-ns" {
		t.Errorf("expected the bridge in the function's own namespace, got %q", svc.Namespace)
	}
	if len(svc.OwnerReferences) != 1 || svc.OwnerReferences[0].UID != "deployment-uid" {
		t.Errorf("expected an ownerRef to the function's Deployment, got %+v", svc.OwnerReferences)
	}
}
