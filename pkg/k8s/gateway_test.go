package k8s

import (
	"context"
	goerrors "errors"
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	ktesting "k8s.io/client-go/testing"
	"knative.dev/func/pkg/deployer"
	fn "knative.dev/func/pkg/functions"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwclientset "sigs.k8s.io/gateway-api/pkg/client/clientset/versioned"
	gwfake "sigs.k8s.io/gateway-api/pkg/client/clientset/versioned/fake"
)

// --- fixture helpers --------------------------------------------------------
//
// Two traps in gwfake's object store (the in-memory stand-in for the API
// server) shape these helpers:
//   - Seed via Create() on the typed sub-clients, never via
//     NewSimpleClientset(objs...): the constructor guesses each object's
//     storage bucket from its Kind and mis-pluralizes Gateway to
//     "gatewaies", so the object lands where no typed read ever looks -
//     no error anywhere -> cluster reports zero Gateways.
//   - The deprecated NewSimpleClientset stays: NewClientset's tracker
//     resolves every operation through the generated apply-configuration
//     schema, which gateway-api doesn't ship for these types (broken
//     through at least v1.6.0) - everything fails with "no matches for
//     ... Resource=gateways", even Create().

// newGwFake builds the gateway-api fake every test in this package uses. The
// deprecated constructor is intentional (see the fixture note above), so the
// lint suppression lives in exactly one place.
func newGwFake() *gwfake.Clientset {
	return gwfake.NewSimpleClientset() //nolint:staticcheck // NewClientset is broken for gateway-api types (no generated apply configurations) through at least v1.6.0
}

// create new gateway on "cluster" (bucket) because we pass in fakeGW interface
func seedGateway(t *testing.T, cs gwclientset.Interface, gw *gwv1.Gateway) {
	t.Helper()
	if _, err := cs.GatewayV1().Gateways(gw.Namespace).Create(context.Background(), gw, metav1.CreateOptions{}); err != nil {
		t.Fatalf("failed to seed gateway %s/%s: %v", gw.Namespace, gw.Name, err)
	}
}

// create new route on "cluster" (bucket) because we pass in fakeGW interface
func seedHTTPRoute(t *testing.T, cs gwclientset.Interface, route *gwv1.HTTPRoute) {
	t.Helper()
	if _, err := cs.GatewayV1().HTTPRoutes(route.Namespace).Create(context.Background(), route, metav1.CreateOptions{}); err != nil {
		t.Fatalf("failed to seed httproute %s/%s: %v", route.Namespace, route.Name, err)
	}
}

// staticNSLabels builds an nsLabelGetter stub returning fixed labels
func staticNSLabels(l map[string]string) nsLabelGetter {
	return func() (labels.Set, error) { return labels.Set(l), nil }
}

// create new gateway in memory
func newGateway(name, ns string, programmed bool, listeners ...gwv1.Listener) *gwv1.Gateway {
	gw := &gwv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
		Spec:       gwv1.GatewaySpec{Listeners: listeners},
	}
	if programmed {
		gw.Status.Conditions = []metav1.Condition{
			{Type: string(gwv1.GatewayConditionProgrammed), Status: metav1.ConditionTrue, Reason: "Programmed", Message: "ok"},
		}
	}
	return gw
}

// allowedRoutes.namespace.from - "routes from which namespaces may attach
// to this listener" returned as GW object listener
func httpListenerFrom(from gwv1.FromNamespaces) gwv1.Listener {
	f := from
	return gwv1.Listener{
		Name:     "http",
		Protocol: gwv1.HTTPProtocolType,
		Port:     80,
		AllowedRoutes: &gwv1.AllowedRoutes{
			Namespaces: &gwv1.RouteNamespaces{From: &f},
		},
	}
}

// httpListenerUnsetFrom builds a listener with allowedRoutes.namespaces.from
// entirely unset, to exercise the Gateway API default (Same).
func httpListenerUnsetFrom() gwv1.Listener {
	return gwv1.Listener{Name: "http", Protocol: gwv1.HTTPProtocolType, Port: 80}
}

// listener requires extra labels on namespace to allow routes to attach to it
func httpListenerSelector(matchLabels map[string]string) gwv1.Listener {
	from := gwv1.NamespacesFromSelector
	return gwv1.Listener{
		Name:     "http",
		Protocol: gwv1.HTTPProtocolType,
		Port:     80,
		AllowedRoutes: &gwv1.AllowedRoutes{
			Namespaces: &gwv1.RouteNamespaces{
				From:     &from,
				Selector: &metav1.LabelSelector{MatchLabels: matchLabels},
			},
		},
	}
}

// listener with only allowed kinds 'kinds' to attach to it
func httpListenerKinds(from gwv1.FromNamespaces, kinds ...string) gwv1.Listener {
	l := httpListenerFrom(from)
	rgks := make([]gwv1.RouteGroupKind, len(kinds))
	for i, k := range kinds {
		rgks[i] = gwv1.RouteGroupKind{Kind: gwv1.Kind(k)}
	}
	l.AllowedRoutes.Kinds = rgks
	return l
}

func withHostname(l gwv1.Listener, hostname string) gwv1.Listener {
	h := gwv1.Hostname(hostname)
	l.Hostname = &h
	return l
}

func withIPAddresses(gw *gwv1.Gateway, ips ...string) *gwv1.Gateway {
	ipType := gwv1.IPAddressType
	addrs := make([]gwv1.GatewayStatusAddress, len(ips))
	for i, ip := range ips {
		addrs[i] = gwv1.GatewayStatusAddress{Type: &ipType, Value: ip}
	}
	gw.Status.Addresses = addrs
	return gw
}

func withHostnameAddress(gw *gwv1.Gateway, hostname string) *gwv1.Gateway {
	hostType := gwv1.HostnameAddressType
	gw.Status.Addresses = []gwv1.GatewayStatusAddress{{Type: &hostType, Value: hostname}}
	return gw
}

func TestParseGatewayRef(t *testing.T) {
	tests := []struct {
		ref      string
		wantNs   string
		wantName string // empty for the trailing-slash (namespace-only) form
		wantErr  bool
	}{
		{ref: "infra/func-gateway", wantNs: "infra", wantName: "func-gateway"},
		{ref: "infra/", wantNs: "infra", wantName: ""},
		{ref: "func-gateway", wantErr: true},
		{ref: "/func-gateway", wantErr: true},
		{ref: "", wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.ref, func(t *testing.T) {
			ns, name, err := ParseGatewayRef(test.ref)
			if test.wantErr {
				if err == nil {
					t.Fatalf("expected error for ref %q, got nil", test.ref)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if ns != test.wantNs || name != test.wantName {
				t.Errorf("ParseGatewayRef(%q) = (%q, %q), want (%q, %q)",
					test.ref, ns, name, test.wantNs, test.wantName)
			}
		})
	}
}

func TestResolveGateway_ClusterWideDiscovery(t *testing.T) {
	ctx := context.Background()
	getLabels := staticNSLabels(nil)

	t.Run("zero eligible gateways is a hard error with remediation", func(t *testing.T) {
		gwFake := newGwFake()
		_, err := ResolveGateway(ctx, gwFake, "default", "", getLabels)
		if err == nil || !strings.Contains(err.Error(), "no eligible gateway found on this cluster") {
			t.Fatalf("expected no-eligible-gateway error, got %v", err)
		}
	})

	t.Run("one eligible gateway is selected", func(t *testing.T) {
		gwFake := newGwFake()
		seedGateway(t, gwFake, newGateway("shared", "infra", true, httpListenerFrom(gwv1.NamespacesFromAll)))
		gw, err := ResolveGateway(ctx, gwFake, "default", "", getLabels)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if gw.Name != "shared" || gw.Namespace != "infra" {
			t.Errorf("got %s/%s, want infra/shared", gw.Namespace, gw.Name)
		}
	})

	t.Run("Programmed=False gateway is filtered out", func(t *testing.T) {
		gwFake := newGwFake()
		seedGateway(t, gwFake, newGateway("not-ready", "infra", false, httpListenerFrom(gwv1.NamespacesFromAll)))
		_, err := ResolveGateway(ctx, gwFake, "default", "", getLabels)
		if err == nil || !strings.Contains(err.Error(), "no eligible gateway found on this cluster") {
			t.Fatalf("expected no-eligible-gateway error (Programmed=False filtered), got %v", err)
		}
	})

	t.Run("multiple eligible gateways is a hard error listing candidates", func(t *testing.T) {
		gwFake := newGwFake()
		seedGateway(t, gwFake, newGateway("gw1", "infra", true, httpListenerFrom(gwv1.NamespacesFromAll)))
		seedGateway(t, gwFake, newGateway("gw2", "other", true, httpListenerFrom(gwv1.NamespacesFromAll)))
		_, err := ResolveGateway(ctx, gwFake, "default", "", getLabels)
		if err == nil {
			t.Fatal("expected error for multiple candidates, got nil")
		}
		if !strings.Contains(err.Error(), "infra/gw1") || !strings.Contains(err.Error(), "other/gw2") {
			t.Errorf("expected error to list both candidates, got: %v", err)
		}
	})

	t.Run("Same admission excludes cross-namespace gateway", func(t *testing.T) {
		gwFake := newGwFake()
		seedGateway(t, gwFake, newGateway("gw", "infra", true, httpListenerFrom(gwv1.NamespacesFromSame)))
		_, err := ResolveGateway(ctx, gwFake, "default", "", getLabels)
		if err == nil || !strings.Contains(err.Error(), "no eligible gateway found on this cluster") {
			t.Fatalf("expected no-eligible-gateway error (Same excludes cross-ns), got %v", err)
		}
	})

	t.Run("Same admission (default, unset from) admits same-namespace gateway", func(t *testing.T) {
		gwFake := newGwFake()
		seedGateway(t, gwFake, newGateway("gw", "default", true, httpListenerUnsetFrom()))
		gw, err := ResolveGateway(ctx, gwFake, "default", "", getLabels)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if gw.Name != "gw" || gw.Namespace != "default" {
			t.Errorf("got %s/%s, want default/gw", gw.Namespace, gw.Name)
		}
	})

	t.Run("Selector admission matching namespace labels", func(t *testing.T) {
		labeled := staticNSLabels(map[string]string{"team": "a"})
		gwFake := newGwFake()
		seedGateway(t, gwFake, newGateway("gw", "infra", true, httpListenerSelector(map[string]string{"team": "a"})))
		gw, err := ResolveGateway(ctx, gwFake, "default", "", labeled)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if gw.Name != "gw" || gw.Namespace != "infra" {
			t.Errorf("got %s/%s, want infra/gw", gw.Namespace, gw.Name)
		}
	})

	t.Run("Selector admission not matching namespace labels excludes gateway", func(t *testing.T) {
		labeled := staticNSLabels(map[string]string{"team": "b"})
		gwFake := newGwFake()
		seedGateway(t, gwFake, newGateway("gw", "infra", true, httpListenerSelector(map[string]string{"team": "a"})))
		_, err := ResolveGateway(ctx, gwFake, "default", "", labeled)
		if err == nil || !strings.Contains(err.Error(), "no eligible gateway found on this cluster") {
			t.Fatalf("expected no-eligible-gateway error (selector mismatch), got %v", err)
		}
	})

	t.Run("allowedRoutes.kinds excluding HTTPRoute filters listener out", func(t *testing.T) {
		gwFake := newGwFake()
		seedGateway(t, gwFake, newGateway("gw", "infra", true, httpListenerKinds(gwv1.NamespacesFromAll, "GRPCRoute")))
		_, err := ResolveGateway(ctx, gwFake, "default", "", getLabels)
		if err == nil || !strings.Contains(err.Error(), "no eligible gateway found on this cluster") {
			t.Fatalf("expected no-eligible-gateway error (kinds excludes HTTPRoute), got %v", err)
		}
	})

	t.Run("allowedRoutes.kinds including HTTPRoute admits listener", func(t *testing.T) {
		gwFake := newGwFake()
		seedGateway(t, gwFake, newGateway("gw", "infra", true, httpListenerKinds(gwv1.NamespacesFromAll, "HTTPRoute")))
		gw, err := ResolveGateway(ctx, gwFake, "default", "", getLabels)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if gw.Name != "gw" || gw.Namespace != "infra" {
			t.Errorf("got %s/%s, want infra/gw", gw.Namespace, gw.Name)
		}
	})
}

func TestResolveGateway_NamespaceRestricted(t *testing.T) {
	ctx := context.Background()
	getLabels := staticNSLabels(nil)

	t.Run("restricts search to given namespace", func(t *testing.T) {
		gwFake := newGwFake()
		seedGateway(t, gwFake, newGateway("gw", "infra", true, httpListenerFrom(gwv1.NamespacesFromAll)))
		seedGateway(t, gwFake, newGateway("other", "other-ns", true, httpListenerFrom(gwv1.NamespacesFromAll)))
		gw, err := ResolveGateway(ctx, gwFake, "default", "infra/", getLabels)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if gw.Name != "gw" || gw.Namespace != "infra" {
			t.Errorf("got %s/%s, want infra/gw", gw.Namespace, gw.Name)
		}
	})

	t.Run("zero found in restricted namespace is a hard error naming the namespace, not provisioning", func(t *testing.T) {
		gwFake := newGwFake()
		_, err := ResolveGateway(ctx, gwFake, "default", "infra/", getLabels)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if strings.Contains(err.Error(), "on this cluster") {
			t.Fatalf("namespace-restricted miss must name the searched namespace, not claim a cluster-wide miss: %v", err)
		}
		if !strings.Contains(err.Error(), `no eligible gateway found in namespace "infra"`) {
			t.Errorf("expected the error to name the searched namespace, got: %v", err)
		}
		if !strings.Contains(err.Error(), "func deploy --expose=none") {
			t.Errorf("expected the cluster-local opt-out option, got: %v", err)
		}
	})

	t.Run("multiple in restricted namespace is a hard error", func(t *testing.T) {
		gwFake := newGwFake()
		seedGateway(t, gwFake, newGateway("gw1", "infra", true, httpListenerFrom(gwv1.NamespacesFromAll)))
		seedGateway(t, gwFake, newGateway("gw2", "infra", true, httpListenerFrom(gwv1.NamespacesFromAll)))
		_, err := ResolveGateway(ctx, gwFake, "default", "infra/", getLabels)
		if err == nil {
			t.Fatal("expected error for multiple candidates, got nil")
		}
	})
}

func TestResolveGateway_ExplicitRef(t *testing.T) {
	ctx := context.Background()
	getLabels := staticNSLabels(nil)

	t.Run("nonexistent gateway is a hard error", func(t *testing.T) {
		gwFake := newGwFake()
		_, err := ResolveGateway(ctx, gwFake, "default", "infra/missing", getLabels)
		if err == nil || !strings.Contains(err.Error(), "does not exist") {
			t.Fatalf("expected 'does not exist' error, got %v", err)
		}
	})

	t.Run("not Programmed is a hard error naming the check", func(t *testing.T) {
		gwFake := newGwFake()
		seedGateway(t, gwFake, newGateway("gw", "infra", false, httpListenerFrom(gwv1.NamespacesFromAll)))
		_, err := ResolveGateway(ctx, gwFake, "default", "infra/gw", getLabels)
		if err == nil || !strings.Contains(err.Error(), "Programmed") {
			t.Fatalf("expected error naming Programmed check, got %v", err)
		}
	})

	t.Run("no admitting listener is a hard error naming the check", func(t *testing.T) {
		gwFake := newGwFake()
		seedGateway(t, gwFake, newGateway("gw", "infra", true, httpListenerFrom(gwv1.NamespacesFromSame)))
		_, err := ResolveGateway(ctx, gwFake, "default", "infra/gw", getLabels)
		if err == nil || !strings.Contains(err.Error(), "listener") {
			t.Fatalf("expected error naming listener check, got %v", err)
		}
	})

	t.Run("valid explicit ref succeeds", func(t *testing.T) {
		gwFake := newGwFake()
		seedGateway(t, gwFake, newGateway("gw", "infra", true, httpListenerFrom(gwv1.NamespacesFromAll)))
		gw, err := ResolveGateway(ctx, gwFake, "default", "infra/gw", getLabels)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if gw.Name != "gw" || gw.Namespace != "infra" {
			t.Errorf("got %s/%s, want infra/gw", gw.Namespace, gw.Name)
		}
	})
}

func TestResolveHostname(t *testing.T) {
	f := fn.Function{Name: "myfunc"}

	t.Run("f.Domain set mints name.ns.domain", func(t *testing.T) {
		gw := newGateway("gw", "infra", true)
		listener := httpListenerFrom(gwv1.NamespacesFromAll)
		fWithDomain := f
		fWithDomain.Domain = "example.com"
		host, err := ResolveHostname(fWithDomain, "default", gw, &listener)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if host != "myfunc.default.example.com" {
			t.Errorf("got %q, want myfunc.default.example.com", host)
		}
	})

	t.Run("f.Domain must intersect listener hostname or hard error", func(t *testing.T) {
		gw := newGateway("gw", "infra", true)
		listener := withHostname(httpListenerFrom(gwv1.NamespacesFromAll), "other.example.net")
		fWithDomain := f
		fWithDomain.Domain = "example.com"
		_, err := ResolveHostname(fWithDomain, "default", gw, &listener)
		if err == nil {
			t.Fatal("expected hard error for non-intersecting listener hostname, got nil")
		}
	})

	t.Run("f.Domain intersecting exact listener hostname succeeds", func(t *testing.T) {
		gw := newGateway("gw", "infra", true)
		listener := withHostname(httpListenerFrom(gwv1.NamespacesFromAll), "myfunc.default.example.com")
		fWithDomain := f
		fWithDomain.Domain = "example.com"
		host, err := ResolveHostname(fWithDomain, "default", gw, &listener)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if host != "myfunc.default.example.com" {
			t.Errorf("got %q, want myfunc.default.example.com", host)
		}
	})

	t.Run("wildcard listener hostname mints single-label hostname", func(t *testing.T) {
		gw := newGateway("gw", "infra", true)
		listener := withHostname(httpListenerFrom(gwv1.NamespacesFromAll), "*.example.com")
		host, err := ResolveHostname(f, "default", gw, &listener)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if host != "myfunc-default.example.com" {
			t.Errorf("got %q, want myfunc-default.example.com", host)
		}
	})

	t.Run("IP gateway address mints sslip.io hostname", func(t *testing.T) {
		gw := withIPAddresses(newGateway("gw", "infra", true), "172.18.0.5")
		listener := httpListenerFrom(gwv1.NamespacesFromAll)
		host, err := ResolveHostname(f, "default", gw, &listener)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if host != "myfunc-default.172.18.0.5.sslip.io" {
			t.Errorf("got %q, want myfunc-default.172.18.0.5.sslip.io", host)
		}
	})

	t.Run("DNS-name gateway address is a hard error asking for --domain", func(t *testing.T) {
		gw := withHostnameAddress(newGateway("gw", "infra", true), "abc.elb.amazonaws.com")
		listener := httpListenerFrom(gwv1.NamespacesFromAll)
		_, err := ResolveHostname(f, "default", gw, &listener)
		if err == nil {
			t.Fatal("expected hard error for DNS-name gateway address, got nil")
		}
		if !strings.Contains(err.Error(), "--domain") {
			t.Errorf("expected error to mention --domain, got: %v", err)
		}
	})

	t.Run("no domain, no wildcard, no address is a hard error", func(t *testing.T) {
		gw := newGateway("gw", "infra", true)
		listener := httpListenerFrom(gwv1.NamespacesFromAll)
		_, err := ResolveHostname(f, "default", gw, &listener)
		if err == nil {
			t.Fatal("expected hard error, got nil")
		}
	})
}

// TestGatewayExternalIP proves the IPv4/IPv6 sslip minting preference:
// IPv4 is always preferred when present, and an IPv6-only address is
// dash-encoded (never a raw colon, which is not a valid hostname character).
func TestGatewayExternalIP(t *testing.T) {
	t.Run("IPv6 first, IPv4 later: IPv4 is preferred", func(t *testing.T) {
		gw := withIPAddresses(newGateway("gw", "infra", true), "2a01:4f8::1", "172.18.0.5")
		if got := gatewayExternalIP(gw); got != "172.18.0.5" {
			t.Errorf("gatewayExternalIP() = %q, want the IPv4 address 172.18.0.5", got)
		}
	})

	t.Run("IPv6-only mints a dash-encoded valid DNS label", func(t *testing.T) {
		gw := withIPAddresses(newGateway("gw", "infra", true), "2a01:4f8::1")
		got := gatewayExternalIP(gw)
		if strings.Contains(got, ":") {
			t.Fatalf("gatewayExternalIP() = %q, must not contain ':'", got)
		}
		if got != "2a01-4f8--1" {
			t.Errorf("gatewayExternalIP() = %q, want 2a01-4f8--1", got)
		}
	})

	t.Run("IPv6-only mints a full hostname with no colon end-to-end", func(t *testing.T) {
		gw := withIPAddresses(newGateway("gw", "infra", true), "2a01:4f8::1")
		listener := httpListenerFrom(gwv1.NamespacesFromAll)
		f := fn.Function{Name: "myfunc"}
		host, err := ResolveHostname(f, "default", gw, &listener)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if strings.Contains(host, ":") {
			t.Errorf("minted hostname %q must not contain ':'", host)
		}
		if host != "myfunc-default.2a01-4f8--1.sslip.io" {
			t.Errorf("got %q, want myfunc-default.2a01-4f8--1.sslip.io", host)
		}
	})

	t.Run("IPv4-mapped IPv6 address is canonicalized to dotted-quad", func(t *testing.T) {
		gw := withIPAddresses(newGateway("gw", "infra", true), "::ffff:192.0.2.1")
		got := gatewayExternalIP(gw)
		if strings.Contains(got, ":") {
			t.Fatalf("gatewayExternalIP() = %q, must not contain ':'", got)
		}
		if got != "192.0.2.1" {
			t.Errorf("gatewayExternalIP() = %q, want 192.0.2.1", got)
		}
	})
}

// TestResolveHostname_MultiListenerUsesAdmittingListener proves selectListener()
// and ResolveHostname() operate on the SAME listener: with a non-admitting
// listener first and an admitting one second, the minted hostname must come
// from the second (admitting) listener, not the first.
func TestResolveHostname_MultiListenerUsesAdmittingListener(t *testing.T) {
	nonAdmitting := withHostname(httpListenerFrom(gwv1.NamespacesFromSame), "*.not-admitting.example.com")
	admitting := withHostname(httpListenerFrom(gwv1.NamespacesFromAll), "*.admitting.example.com")
	gw := newGateway("gw", "infra", true, nonAdmitting, admitting)

	listener, err := selectListener(gw, "default", staticNSLabels(nil))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if listener == nil {
		t.Fatal("expected an admitting listener, got nil")
	}

	f := fn.Function{Name: "myfunc"}
	host, err := ResolveHostname(f, "default", gw, listener)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if host != "myfunc-default.admitting.example.com" {
		t.Errorf("got %q, want myfunc-default.admitting.example.com (minted from the admitting listener, not the first)", host)
	}
}

// TestSelectListener_UnevaluatableIsNonAdmitting proves an admission check
// failure (e.g. an RBAC error resolving a Selector) does not abort
// evaluation: it is treated as non-admitting and a later listener still gets
// a chance to admit.
func TestSelectListener_UnevaluatableIsNonAdmitting(t *testing.T) {
	forbidden := httpListenerSelector(map[string]string{"team": "payments"})
	fallback := httpListenerFrom(gwv1.NamespacesFromAll)
	gw := newGateway("gw", "infra", true, forbidden, fallback)

	forbiddenLabels := func() (labels.Set, error) {
		return nil, apierrors.NewForbidden(corev1.Resource("namespaces"), "default", goerrors.New("rbac"))
	}

	listener, err := selectListener(gw, "default", forbiddenLabels)
	if err != nil {
		t.Fatalf("unexpected error: %v (an RBAC error on one listener must not abort evaluation of later listeners)", err)
	}
	if listener == nil {
		t.Fatal("expected the from:All listener to admit despite the earlier RBAC error")
	}
}

// TestSelectListener_NoListenerAdmitsSurfacesSwallowedError proves that when
// every listener fails to admit AND at least one admission check itself
// failed, that swallowed error surfaces as context rather than being
// silently dropped.
func TestSelectListener_NoListenerAdmitsSurfacesSwallowedError(t *testing.T) {
	forbidden := httpListenerSelector(map[string]string{"team": "payments"})
	gw := newGateway("gw", "infra", true, forbidden)

	forbiddenLabels := func() (labels.Set, error) {
		return nil, apierrors.NewForbidden(corev1.Resource("namespaces"), "default", goerrors.New("rbac"))
	}

	listener, err := selectListener(gw, "default", forbiddenLabels)
	if listener != nil {
		t.Fatalf("expected no admitting listener, got %+v", listener)
	}
	if err == nil {
		t.Fatal("expected the swallowed RBAC error to surface since no listener admitted")
	}
	if !strings.Contains(err.Error(), "rbac") {
		t.Errorf("expected the swallowed error as context, got: %v", err)
	}
}

// TestSelectListener_HostnamePreference proves selectListener()'s hostname
// preference (M4): among admitting listeners, one with a usable (wildcard)
// hostname is preferred regardless of position, falling back to the first
// admitting listener (the IP/sslip path) only when none has one.
func TestSelectListener_HostnamePreference(t *testing.T) {
	f := fn.Function{Name: "myfunc"}
	getLabels := staticNSLabels(nil)

	t.Run("bare listener before wildcard mints from the wildcard", func(t *testing.T) {
		bare := httpListenerFrom(gwv1.NamespacesFromAll)
		wildcard := withHostname(httpListenerFrom(gwv1.NamespacesFromAll), "*.example.com")
		gw := newGateway("gw", "infra", true, bare, wildcard)

		listener, err := selectListener(gw, "default", getLabels)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		host, err := ResolveHostname(f, "default", gw, listener)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if host != "myfunc-default.example.com" {
			t.Errorf("got %q, want myfunc-default.example.com (from the wildcard, not the earlier bare listener)", host)
		}
	})

	t.Run("wildcard-only", func(t *testing.T) {
		wildcard := withHostname(httpListenerFrom(gwv1.NamespacesFromAll), "*.example.com")
		gw := newGateway("gw", "infra", true, wildcard)

		listener, err := selectListener(gw, "default", getLabels)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		host, err := ResolveHostname(f, "default", gw, listener)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if host != "myfunc-default.example.com" {
			t.Errorf("got %q, want myfunc-default.example.com", host)
		}
	})

	t.Run("bare-only falls back to the first admitting listener (IP/sslip path)", func(t *testing.T) {
		bare := httpListenerFrom(gwv1.NamespacesFromAll)
		gw := withIPAddresses(newGateway("gw", "infra", true, bare), "172.18.0.5")

		listener, err := selectListener(gw, "default", getLabels)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		host, err := ResolveHostname(f, "default", gw, listener)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if host != "myfunc-default.172.18.0.5.sslip.io" {
			t.Errorf("got %q, want myfunc-default.172.18.0.5.sslip.io", host)
		}
	})

	t.Run("mixed order: wildcard before bare still mints from the wildcard", func(t *testing.T) {
		wildcard := withHostname(httpListenerFrom(gwv1.NamespacesFromAll), "*.example.com")
		bare := httpListenerFrom(gwv1.NamespacesFromAll)
		gw := newGateway("gw", "infra", true, wildcard, bare)

		listener, err := selectListener(gw, "default", getLabels)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		host, err := ResolveHostname(f, "default", gw, listener)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if host != "myfunc-default.example.com" {
			t.Errorf("got %q, want myfunc-default.example.com", host)
		}
	})
}

func newHTTPRoute(name, ns string, generation int64, parents ...gwv1.RouteParentStatus) *gwv1.HTTPRoute {
	return &gwv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns, Generation: generation},
		Status:     gwv1.HTTPRouteStatus{RouteStatus: gwv1.RouteStatus{Parents: parents}},
	}
}

func parentStatus(gwName, gwNamespace string, status metav1.ConditionStatus, reason, message string, observedGeneration int64) gwv1.RouteParentStatus {
	var nsPtr *gwv1.Namespace
	if gwNamespace != "" {
		n := gwv1.Namespace(gwNamespace)
		nsPtr = &n
	}
	return gwv1.RouteParentStatus{
		ParentRef:      gwv1.ParentReference{Name: gwv1.ObjectName(gwName), Namespace: nsPtr},
		ControllerName: "example.com/controller",
		Conditions: []metav1.Condition{
			{
				Type:               string(gwv1.RouteConditionAccepted),
				Status:             status,
				Reason:             reason,
				Message:            message,
				ObservedGeneration: observedGeneration,
			},
		},
	}
}

func TestParentRefMatches(t *testing.T) {
	// test that HTTPRoute holds correct GW info
	gwKind := gwv1.Kind("Gateway")
	svcKind := gwv1.Kind("Service")
	gwGroup := gwv1.Group(gwv1.GroupName)
	coreGroup := gwv1.Group("")

	tests := []struct {
		name string
		ref  gwv1.ParentReference
		want bool
	}{
		{"unset Kind/Group defaults to Gateway", gwv1.ParentReference{Name: "gw"}, true},
		{"explicit Gateway Kind + gateway-api Group", gwv1.ParentReference{Name: "gw", Kind: &gwKind, Group: &gwGroup}, true},
		{"Service Kind is rejected", gwv1.ParentReference{Name: "gw", Kind: &svcKind}, false},
		{"core Group is rejected", gwv1.ParentReference{Name: "gw", Group: &coreGroup}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := parentRefMatches(tt.ref, "gw", "default", "default"); got != tt.want {
				t.Errorf("parentRefMatches(%+v) = %v, want %v", tt.ref, got, tt.want)
			}
		})
	}
}

func TestRemoveManagedHTTPRoute(t *testing.T) {
	ctx := context.Background()

	t.Run("route not found is a no-op", func(t *testing.T) {
		gwFake := newGwFake()
		removed, err := RemoveManagedHTTPRoute(ctx, gwFake, "default", "myfunc")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if removed {
			t.Error("expected removed=false when the route doesn't exist")
		}
	})

	t.Run("managed route (label AND raw deployer annotation) is deleted", func(t *testing.T) {
		gwFake := newGwFake()
		route := &gwv1.HTTPRoute{
			ObjectMeta: metav1.ObjectMeta{
				Name: "myfunc", Namespace: "default",
				Labels:      map[string]string{"boson.dev/function": "true"},
				Annotations: map[string]string{deployer.DeployerNameAnnotation: KubernetesDeployerName},
			},
		}
		seedHTTPRoute(t, gwFake, route)

		removed, err := RemoveManagedHTTPRoute(ctx, gwFake, "default", "myfunc")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !removed {
			t.Error("expected removed=true for a func-managed route")
		}
		if _, err := gwFake.GatewayV1().HTTPRoutes("default").Get(ctx, "myfunc", metav1.GetOptions{}); !apierrors.IsNotFound(err) {
			t.Errorf("expected the route to be gone, got err=%v", err)
		}
	})

	t.Run("label-only route (no deployer annotation) is now unmanaged and left in place", func(t *testing.T) {
		gwFake := newGwFake()
		route := &gwv1.HTTPRoute{
			ObjectMeta: metav1.ObjectMeta{
				Name: "myfunc", Namespace: "default",
				Labels: map[string]string{"boson.dev/function": "true"},
			},
		}
		seedHTTPRoute(t, gwFake, route)

		removed, err := RemoveManagedHTTPRoute(ctx, gwFake, "default", "myfunc")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if removed {
			t.Error("expected removed=false: the label alone no longer proves func manages this route")
		}
	})

	t.Run("foreign-annotation route (deployer=keda, with label) is now unmanaged and left in place", func(t *testing.T) {
		gwFake := newGwFake()
		route := &gwv1.HTTPRoute{
			ObjectMeta: metav1.ObjectMeta{
				Name: "myfunc", Namespace: "default",
				Labels:      map[string]string{"boson.dev/function": "true"},
				Annotations: map[string]string{deployer.DeployerNameAnnotation: "keda"},
			},
		}
		seedHTTPRoute(t, gwFake, route)

		removed, err := RemoveManagedHTTPRoute(ctx, gwFake, "default", "myfunc")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if removed {
			t.Error("expected removed=false: a foreign deployer annotation must not be treated as raw-managed")
		}
	})

	t.Run("CRITICAL: unmanaged route with the same name is left in place", func(t *testing.T) {
		gwFake := newGwFake()
		route := &gwv1.HTTPRoute{
			ObjectMeta: metav1.ObjectMeta{
				Name: "myfunc", Namespace: "default",
				// No boson.dev/function label, no deployer annotation -
				// e.g. a manually wired keda-interceptor route.
				Labels: map[string]string{"app": "manual-keda-route"},
			},
		}
		seedHTTPRoute(t, gwFake, route)

		removed, err := RemoveManagedHTTPRoute(ctx, gwFake, "default", "myfunc")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if removed {
			t.Fatal("must NOT delete an unmanaged HTTPRoute that happens to share the function's name")
		}
		if _, err := gwFake.GatewayV1().HTTPRoutes("default").Get(ctx, "myfunc", metav1.GetOptions{}); err != nil {
			t.Errorf("expected the unmanaged route to still exist, got err=%v", err)
		}
	})
}

func TestIsManagedHTTPRoute(t *testing.T) {
	tests := []struct {
		name  string
		route *gwv1.HTTPRoute
		want  bool
	}{
		{
			"label AND raw deployer annotation",
			&gwv1.HTTPRoute{ObjectMeta: metav1.ObjectMeta{
				Labels:      map[string]string{"boson.dev/function": "true"},
				Annotations: map[string]string{deployer.DeployerNameAnnotation: "raw"},
			}},
			true,
		},
		{
			"boson.dev/function label only (no deployer annotation) is now unmanaged",
			&gwv1.HTTPRoute{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"boson.dev/function": "true"}}},
			false,
		},
		{
			"raw deployer annotation only (no label) is now unmanaged",
			&gwv1.HTTPRoute{ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{deployer.DeployerNameAnnotation: "raw"}}},
			false,
		},
		{
			"label + foreign deployer annotation is unmanaged",
			&gwv1.HTTPRoute{ObjectMeta: metav1.ObjectMeta{
				Labels:      map[string]string{"boson.dev/function": "true"},
				Annotations: map[string]string{deployer.DeployerNameAnnotation: "keda"},
			}},
			false,
		},
		{"neither", &gwv1.HTTPRoute{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "something-else"}}}, false},
		{"no metadata at all", &gwv1.HTTPRoute{}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isManagedHTTPRoute(tt.route); got != tt.want {
				t.Errorf("isManagedHTTPRoute() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestWaitForRouteAccepted(t *testing.T) {
	ctx := context.Background()

	t.Run("Accepted=False fails immediately with reason and message", func(t *testing.T) {
		route := newHTTPRoute("myfunc", "default", 1,
			parentStatus("gw", "", metav1.ConditionFalse, "NotAllowedByListeners", "no listener admits this route", 1))
		gwFake := newGwFake()
		seedHTTPRoute(t, gwFake, route)

		start := time.Now()
		err := WaitForRouteAccepted(ctx, gwFake, "default", "myfunc", "gw", "default", 30*time.Second)
		elapsed := time.Since(start)
		if err == nil {
			t.Fatal("expected error for Accepted=False, got nil")
		}
		if !strings.Contains(err.Error(), "NotAllowedByListeners") {
			t.Errorf("expected error to include reason, got: %v", err)
		}
		if elapsed > 2*time.Second {
			t.Errorf("Accepted=False should fail fast, took %s", elapsed)
		}
	})

	t.Run("Accepted=True succeeds", func(t *testing.T) {
		route := newHTTPRoute("myfunc", "default", 1,
			parentStatus("gw", "", metav1.ConditionTrue, "Accepted", "ok", 1))
		gwFake := newGwFake()
		seedHTTPRoute(t, gwFake, route)

		if err := WaitForRouteAccepted(ctx, gwFake, "default", "myfunc", "gw", "default", 30*time.Second); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("Accepted=Unknown then True succeeds (route still loading)", func(t *testing.T) {
		gwFake := newGwFake()
		seedHTTPRoute(t, gwFake, newHTTPRoute("myfunc", "default", 1,
			parentStatus("gw", "", metav1.ConditionUnknown, "Pending", "waiting for controller", 1)))

		getCalls := 0
		gwFake.PrependReactor("get", "httproutes", func(action ktesting.Action) (bool, runtime.Object, error) {
			getCalls++
			if getCalls == 1 {
				return true, newHTTPRoute("myfunc", "default", 1,
					parentStatus("gw", "", metav1.ConditionUnknown, "Pending", "waiting for controller", 1)), nil
			}
			return true, newHTTPRoute("myfunc", "default", 1,
				parentStatus("gw", "", metav1.ConditionTrue, "Accepted", "ok", 1)), nil
		})

		if err := WaitForRouteAccepted(ctx, gwFake, "default", "myfunc", "gw", "default", 5*time.Second); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("Accepted=Unknown polls until timeout, not fail-fast", func(t *testing.T) {
		route := newHTTPRoute("myfunc", "default", 1,
			parentStatus("gw", "", metav1.ConditionUnknown, "Pending", "waiting for controller", 1))
		gwFake := newGwFake()
		seedHTTPRoute(t, gwFake, route)

		err := WaitForRouteAccepted(ctx, gwFake, "default", "myfunc", "gw", "default", 0)
		if err == nil {
			t.Fatal("expected timeout error, got nil")
		}
		if !strings.Contains(err.Error(), "was not accepted") {
			t.Errorf("expected the generic timeout message (Unknown must not fail fast), got: %v", err)
		}
	})

	t.Run("parentRef with no namespace defaults to the route's own namespace", func(t *testing.T) {
		// parentRef.Namespace unset ("") - must still match gwNamespace == route namespace.
		route := newHTTPRoute("myfunc", "default", 1,
			parentStatus("gw", "", metav1.ConditionTrue, "Accepted", "ok", 1))
		gwFake := newGwFake()
		seedHTTPRoute(t, gwFake, route)

		if err := WaitForRouteAccepted(ctx, gwFake, "default", "myfunc", "gw", "default", 30*time.Second); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("parentRef from a different Gateway is ignored (foreign status)", func(t *testing.T) {
		// Only a stale/foreign parent (different Gateway) reports status;
		// the resolved Gateway "gw" has no entry yet - must time out, not
		// false-succeed off the foreign parent.
		route := newHTTPRoute("myfunc", "default", 1,
			parentStatus("other-gw", "other-ns", metav1.ConditionTrue, "Accepted", "ok", 1))
		gwFake := newGwFake()
		seedHTTPRoute(t, gwFake, route)

		err := WaitForRouteAccepted(ctx, gwFake, "default", "myfunc", "gw", "default", 0)
		if err == nil {
			t.Fatal("expected timeout error (foreign parent must not satisfy), got nil")
		}
	})

	t.Run("stale observedGeneration is ignored (mid Gateway-switch race)", func(t *testing.T) {
		// Status reflects an older spec generation than the route's current
		// generation (e.g. a Gateway switch just happened) - must not be
		// trusted even though it says Accepted=True.
		route := newHTTPRoute("myfunc", "default", 2,
			parentStatus("gw", "", metav1.ConditionTrue, "Accepted", "ok", 1))
		gwFake := newGwFake()
		seedHTTPRoute(t, gwFake, route)

		err := WaitForRouteAccepted(ctx, gwFake, "default", "myfunc", "gw", "default", 0)
		if err == nil {
			t.Fatal("expected timeout error (stale generation must not satisfy), got nil")
		}
	})

	t.Run("observedGeneration=0 (unreported) is accepted", func(t *testing.T) {
		// Some controllers never populate ObservedGeneration (zero value) -
		// a hard equality requirement would lock them out entirely, turning
		// a real Accepted=True (or Accepted=False) into a blind timeout.
		route := newHTTPRoute("myfunc", "default", 5,
			parentStatus("gw", "", metav1.ConditionTrue, "Accepted", "ok", 0))
		gwFake := newGwFake()
		seedHTTPRoute(t, gwFake, route)

		if err := WaitForRouteAccepted(ctx, gwFake, "default", "myfunc", "gw", "default", 30*time.Second); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("a Service-kind parent with the same name is ignored (mesh status)", func(t *testing.T) {
		// A mesh implementation (e.g. Istio ambient) can report route status
		// against a Service parent that happens to share the Gateway's name -
		// must not be mistaken for our Gateway parent.
		serviceKind := gwv1.Kind("Service")
		coreGroup := gwv1.Group("")
		parent := parentStatus("gw", "", metav1.ConditionTrue, "Accepted", "ok", 1)
		parent.ParentRef.Kind = &serviceKind
		parent.ParentRef.Group = &coreGroup
		route := newHTTPRoute("myfunc", "default", 1, parent)
		gwFake := newGwFake()
		seedHTTPRoute(t, gwFake, route)

		err := WaitForRouteAccepted(ctx, gwFake, "default", "myfunc", "gw", "default", 0)
		if err == nil {
			t.Fatal("expected timeout error (Service-kind parent must not satisfy), got nil")
		}
	})

	t.Run("no status times out", func(t *testing.T) {
		route := newHTTPRoute("myfunc", "default", 1)
		gwFake := newGwFake()
		seedHTTPRoute(t, gwFake, route)

		err := WaitForRouteAccepted(ctx, gwFake, "default", "myfunc", "gw", "default", 0)
		if err == nil {
			t.Fatal("expected timeout error, got nil")
		}
	})

	t.Run("context cancellation is honored", func(t *testing.T) {
		route := newHTTPRoute("myfunc", "default", 1)
		gwFake := newGwFake()
		seedHTTPRoute(t, gwFake, route)

		cancelCtx, cancel := context.WithCancel(ctx)
		cancel()
		start := time.Now()
		err := WaitForRouteAccepted(cancelCtx, gwFake, "default", "myfunc", "gw", "default", 30*time.Second)
		elapsed := time.Since(start)
		if err == nil {
			t.Fatal("expected error from cancelled context, got nil")
		}
		if elapsed > 2*time.Second {
			t.Errorf("cancelled context should return promptly, took %s", elapsed)
		}
	})
}
