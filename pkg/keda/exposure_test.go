package keda

import (
	"errors"
	"slices"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"

	"knative.dev/func/pkg/deployer"
	fn "knative.dev/func/pkg/functions"
	"knative.dev/func/pkg/k8s"
)

// errDenied fills the cause slot of the synthetic Forbidden errors the test
// reactors return. Only the 403 type matters to the code under test; the
// text is never read.
var errDenied = errors.New("denied")

// Test_interceptorNamespace: the interceptor's namespace depends on how keda
// was installed, and the two installs func supports disagree. Getting it wrong
// points the bridge Service and every exposing object at a namespace that
// holds nothing.
//
// It is resolved by looking for the interceptor Service, so the platform sets
// only the order tried and the answer given when nothing definite came back.
//
// Note: SetOpenShiftForTest mutates a package-level bool without a mutex, so
// this test must not run with t.Parallel() (see pkg/k8s/openshift.go).
func Test_interceptorNamespace(t *testing.T) {
	interceptorServiceIn := func(ns string) *corev1.Service {
		return &corev1.Service{ObjectMeta: metav1.ObjectMeta{
			Name: interceptorServiceName, Namespace: ns,
		}}
	}

	tests := []struct {
		name      string
		openShift bool
		installed []string
		want      string
		// wantRefusal is a substring of the refusal's error message; empty
		// means the namespace was confirmed.
		wantRefusal string
	}{
		{
			name: "CMA on OpenShift", openShift: true,
			installed: []string{interceptorNamespaceOpenShift},
			want:      interceptorNamespaceOpenShift,
		},
		{
			// OpenShift clusters can run either CMA or upstream keda, in
			// different namespaces; the probe finds what platform inference
			// alone would miss.
			name: "upstream keda on OpenShift", openShift: true,
			installed: []string{interceptorNamespaceUpstream},
			want:      interceptorNamespaceUpstream,
		},
		{
			name: "upstream keda off OpenShift", openShift: false,
			installed: []string{interceptorNamespaceUpstream},
			want:      interceptorNamespaceUpstream,
		},
		{
			// Both present is a broken install (CMA requires removing
			// community keda first), but the pick must still be
			// deterministic: CMA's namespace wins.
			name: "both installs present on OpenShift", openShift: true,
			installed: []string{interceptorNamespaceOpenShift, interceptorNamespaceUpstream},
			want:      interceptorNamespaceOpenShift,
		},
		{
			// Both candidates answered NotFound.
			name: "neither installed: platform default", openShift: true,
			installed: nil,
			want:      interceptorNamespaceOpenShift, wantRefusal: "was not found",
		},
		{
			name: "neither installed off OpenShift: platform default", openShift: false,
			installed: nil,
			want:      interceptorNamespaceUpstream, wantRefusal: "was not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cleanup := k8s.SetOpenShiftForTest(tt.openShift, nil)
			defer cleanup()

			// A fake cluster seeded with whatever interceptor Services this
			// row installs.
			objects := make([]runtime.Object, 0, len(tt.installed))
			for _, ns := range tt.installed {
				objects = append(objects, interceptorServiceIn(ns))
			}
			got, refusal := interceptorNamespace(t.Context(), fake.NewClientset(objects...))
			if got != tt.want {
				t.Errorf("interceptorNamespace() = %q, want %q", got, tt.want)
			}
			if tt.wantRefusal == "" {
				if refusal != nil {
					t.Errorf("interceptorNamespace() refusal = %v, want none", refusal)
				}
			} else if refusal == nil || !strings.Contains(refusal.Error(), tt.wantRefusal) {
				t.Errorf("interceptorNamespace() refusal = %v, want one containing %q", refusal, tt.wantRefusal)
			}
		})
	}
}

// Test_interceptorNamespace_AllDeniedUsesPlatformDefault: a restricted
// account that cannot read any candidate gets pure platform inference, the
// same answer probing nothing would give. Denial degrades the probe to a
// no-op, never to an error or a wrong claim.
//
// This case cannot distinguish reading Forbidden as "absent" from reading it
// as "unknown", because both answer with the same namespace when every
// candidate is denied. Test_interceptorNamespace_DeniedBeatsRuledOut is the
// case that separates them; this one only holds the floor.
func Test_interceptorNamespace_AllDeniedUsesPlatformDefault(t *testing.T) {
	cleanup := k8s.SetOpenShiftForTest(true, nil)
	defer cleanup()

	// An interceptor really is installed; denial hides it, so the guess
	// below is genuinely blind.
	clientset := fake.NewClientset(&corev1.Service{ObjectMeta: metav1.ObjectMeta{
		Name: interceptorServiceName, Namespace: interceptorNamespaceUpstream,
	}})
	clientset.PrependReactor("get", "services", func(action k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, k8serrors.NewForbidden(
			schema.GroupResource{Resource: "services"}, interceptorServiceName, errDenied)
	})

	got, refusal := interceptorNamespace(t.Context(), clientset)
	if got != interceptorNamespaceOpenShift {
		t.Errorf("interceptorNamespace() = %q, want the platform default %q when every lookup is denied",
			got, interceptorNamespaceOpenShift)
	}
	// The namespace is the same as the absent case; the refusal is what must
	// differ, since it is the only thing telling the caller it never looked.
	if refusal == nil || !strings.Contains(refusal.Error(), "could not determine") {
		t.Errorf("refusal = %v, want one saying it could not determine: a denied lookup must not report absence", refusal)
	}
}

// Test_interceptorNamespace_DeniedBeatsRuledOut: when one candidate is
// definitely absent and the other cannot be seen, the one that could not be
// ruled out wins. Answering with a namespace known to hold nothing would be
// strictly worse than admitting ignorance.
func Test_interceptorNamespace_DeniedBeatsRuledOut(t *testing.T) {
	cleanup := k8s.SetOpenShiftForTest(true, nil)
	defer cleanup()

	clientset := fake.NewClientset()
	clientset.PrependReactor("get", "services", func(action k8stesting.Action) (bool, runtime.Object, error) {
		if action.GetNamespace() == interceptorNamespaceUpstream {
			return true, nil, k8serrors.NewForbidden(
				schema.GroupResource{Resource: "services"}, interceptorServiceName, errDenied)
		}
		return false, nil, nil // openshift-keda falls through to a real NotFound
	})

	got, refusal := interceptorNamespace(t.Context(), clientset)
	if got != interceptorNamespaceUpstream {
		t.Errorf("interceptorNamespace() = %q, want %q: the ruled-out candidate must lose to the unseen one",
			got, interceptorNamespaceUpstream)
	}
	if refusal == nil || !strings.Contains(refusal.Error(), "could not determine") {
		t.Errorf("refusal = %v, want one saying it could not determine: one candidate was never ruled out", refusal)
	}
}

// Test_interceptorExposureName: every keda function's exposing object lands in
// the one interceptor namespace, so the name has to separate two functions
// that share a name in different namespaces. Without the namespace in the
// name, the second deploy would retarget the first function's object.
func Test_interceptorExposureName(t *testing.T) {
	a := interceptorExposureName("f", "alice")
	b := interceptorExposureName("f", "bob")
	if a == b {
		t.Fatalf("same-named functions in different namespaces collided on %q", a)
	}
	if a != "f-alice" {
		t.Errorf("interceptorExposureName = %q, want %q", a, "f-alice")
	}
}

// Test_interceptorExposure: keda's exposure must target the shared interceptor
// Service, not the function's own. A Route to the function's Service would
// bypass the interceptor, and a function scaled to zero would answer nothing.
// It must also carry no owner reference, since Kubernetes rejects one across
// namespaces and the interceptor's namespace is not the function's.
func Test_interceptorExposure(t *testing.T) {
	f := fn.Function{Name: "f", Runtime: "go"}
	labels, err := deployer.GenerateCommonLabels(f, nil)
	if err != nil {
		t.Fatal(err)
	}
	anns := deployer.GenerateCommonAnnotations(f, nil, false, KedaDeployerName)
	e := interceptorExposure(deployer.NewExposureRef(f.Name, "ns", interceptorNamespaceOpenShift), labels, anns)

	if e.TargetService != interceptorServiceName {
		t.Errorf("TargetService = %q, want the interceptor %q; targeting anything else, like the function's own Service, would bypass scale-from-zero",
			e.TargetService, interceptorServiceName)
	}
	if e.TargetPort != interceptorServicePortName {
		t.Errorf("TargetPort = %q, want %q", e.TargetPort, interceptorServicePortName)
	}
	if e.Namespace != interceptorNamespaceOpenShift {
		t.Errorf("Namespace = %q, want %q", e.Namespace, interceptorNamespaceOpenShift)
	}
	if e.Name != "f-ns" {
		t.Errorf("Name = %q, want %q", e.Name, "f-ns")
	}
	if e.Owner != nil {
		t.Errorf("Owner = %+v, want nil: an owner reference cannot cross namespaces", e.Owner)
	}
}

// Test_functionURLs: the exposed hostname is registered among the
// HTTPScaledObject's hosts so the interceptor matches requests carrying it,
// which means the reporting paths cannot treat every host alike. The exposed
// one is reached over https and never on :8080, and it leads, being the only
// address reachable from outside the cluster.
func Test_functionURLs(t *testing.T) {
	bridges := []string{"f-interceptor-bridge.ns.svc", "f-interceptor-bridge"}

	t.Run("cluster-local reports only bridge addresses", func(t *testing.T) {
		primary, all := functionURLs(bridges, "")
		want := []string{"http://f-interceptor-bridge.ns.svc:8080", "http://f-interceptor-bridge:8080"}
		if !slices.Equal(all, want) {
			t.Errorf("all = %v, want %v", all, want)
		}
		if primary != want[0] {
			t.Errorf("primary = %q, want %q", primary, want[0])
		}
	})

	t.Run("exposed leads with https and never repeats the host on :8080", func(t *testing.T) {
		const host = "f-ns.apps.example.com"
		primary, all := functionURLs(append(slices.Clone(bridges), host), host)

		if primary != "https://"+host {
			t.Errorf("primary = %q, want %q", primary, "https://"+host)
		}
		want := []string{
			"https://" + host,
			"http://f-interceptor-bridge.ns.svc:8080",
			"http://f-interceptor-bridge:8080",
		}
		if !slices.Equal(all, want) {
			t.Errorf("all = %v, want %v", all, want)
		}
		if slices.Contains(all, "http://"+host+":8080") {
			t.Error("exposed host reported as a bridge address on :8080")
		}
	})

	t.Run("no hosts at all", func(t *testing.T) {
		if primary, all := functionURLs(nil, ""); primary != "" || len(all) != 0 {
			t.Errorf("expected no URLs, got primary %q and %v", primary, all)
		}
	})
}

// Test_validateBridgeName: a function name that ValidateFunctionName accepts
// can still produce a bridge Service name the API server will not, because
// the suffix pushes it past a DNS-1035 label's 63 characters.
func Test_validateBridgeName(t *testing.T) {
	name := func(n int) string {
		s := "a"
		for len(s) < n {
			s += "b"
		}
		return s
	}

	t.Run("ordinary name", func(t *testing.T) {
		if err := validateBridgeName("myfunc"); err != nil {
			t.Errorf("expected an ordinary name to pass, got %v", err)
		}
	})

	t.Run("longest name that fits", func(t *testing.T) {
		if err := validateBridgeName(name(maxKedaFunctionName)); err != nil {
			t.Errorf("expected a %d character name to pass, got %v", maxKedaFunctionName, err)
		}
	})

	t.Run("one character too long", func(t *testing.T) {
		n := name(maxKedaFunctionName + 1)
		err := validateBridgeName(n)
		if err == nil {
			t.Fatalf("expected a %d character name to be refused", maxKedaFunctionName+1)
		}
		if !strings.Contains(err.Error(), n) {
			t.Errorf("expected the error to name the function, got %v", err)
		}
	})

	t.Run("the longest name func itself accepts", func(t *testing.T) {
		// utils.ValidateFunctionName admits any DNS-1035 label, so 63 is
		// legal as a function name and still too long here.
		if err := validateBridgeName(name(63)); err == nil {
			t.Error("expected a 63 character name, legal as a function name, to be refused by keda")
		}
	})
}

// Test_validateExposureName: the Route's name is built from the function's
// name AND its namespace, so it cannot be checked until the namespace is
// resolved. An unresolved namespace yields a name ending in a hyphen, which is
// not a valid DNS-1123 subdomain, and checking too early would refuse a first
// deploy that is perfectly fine.
func Test_validateExposureName(t *testing.T) {
	f := fn.Function{Name: "myfunc"}

	if err := validateExposureName(f, "myns"); err != nil {
		t.Errorf("expected a resolved namespace to pass, got %v", err)
	}

	if err := validateExposureName(f, ""); err == nil {
		t.Error("expected an unresolved namespace to be refused rather than silently producing 'myfunc-'")
	}
}
