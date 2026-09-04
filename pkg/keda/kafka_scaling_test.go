package keda

import (
	"testing"

	v1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	fn "knative.dev/func/pkg/functions"
)

func newScalingDynClient(objects ...runtime.Object) *dynamicfake.FakeDynamicClient {
	return dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
		runtime.NewScheme(),
		map[schema.GroupVersionResource]string{
			scaledObjectGVR: "ScaledObjectList",
			triggerAuthGVR:  "TriggerAuthenticationList",
		},
		objects...)
}

func unstructuredScaledObject(name, ns string) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "keda.sh/v1alpha1",
		"kind":       "ScaledObject",
		"metadata":   map[string]interface{}{"name": name, "namespace": ns},
	}}
}

func unstructuredTriggerAuth(name, ns string) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "keda.sh/v1alpha1",
		"kind":       "TriggerAuthentication",
		"metadata":   map[string]interface{}{"name": name, "namespace": ns},
	}}
}

func TestEnsureScaledObject_PreservesFinalizers(t *testing.T) {
	ns := "fn-ns"
	name := "f-kafka"

	existing := unstructuredScaledObject(name, ns)
	existing.SetFinalizers([]string{"scaledobject.keda.sh/finalizer"})
	dynClient := newScalingDynClient(existing)

	// A freshly-built ScaledObject, as buildScaledObject would produce: no
	// finalizers set at all.
	updated := unstructuredScaledObject(name, ns)
	if err := ensureScaledObject(t.Context(), dynClient, updated); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got, err := dynClient.Resource(scaledObjectGVR).Namespace(ns).Get(t.Context(), name, metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if finalizers := got.GetFinalizers(); len(finalizers) != 1 || finalizers[0] != "scaledobject.keda.sh/finalizer" {
		t.Errorf("expected KEDA's finalizer to survive the update, got %v", finalizers)
	}
}

func TestEnsureTriggerAuth_PreservesFinalizers(t *testing.T) {
	ns := "fn-ns"
	name := "f-kafka-auth"

	existing := unstructuredTriggerAuth(name, ns)
	existing.SetFinalizers([]string{"triggerauthentication.keda.sh/finalizer"})
	dynClient := newScalingDynClient(existing)

	updated := unstructuredTriggerAuth(name, ns)
	if err := ensureTriggerAuth(t.Context(), dynClient, updated); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got, err := dynClient.Resource(triggerAuthGVR).Namespace(ns).Get(t.Context(), name, metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if finalizers := got.GetFinalizers(); len(finalizers) != 1 || finalizers[0] != "triggerauthentication.keda.sh/finalizer" {
		t.Errorf("expected KEDA's finalizer to survive the update, got %v", finalizers)
	}
}

func TestDeleteScaledObject(t *testing.T) {
	ns := "fn-ns"
	name := "f-kafka"

	t.Run("removes an existing ScaledObject", func(t *testing.T) {
		dynClient := newScalingDynClient(unstructuredScaledObject(name, ns))
		if err := deleteScaledObject(t.Context(), dynClient, ns, name); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		_, err := dynClient.Resource(scaledObjectGVR).Namespace(ns).Get(t.Context(), name, metav1.GetOptions{})
		if !k8serrors.IsNotFound(err) {
			t.Errorf("expected ScaledObject to be gone, got err: %v", err)
		}
	})

	t.Run("not-found is not an error", func(t *testing.T) {
		dynClient := newScalingDynClient()
		if err := deleteScaledObject(t.Context(), dynClient, ns, name); err != nil {
			t.Fatalf("expected nil error for a non-existent ScaledObject, got: %v", err)
		}
	})
}

func TestDeleteTriggerAuth(t *testing.T) {
	ns := "fn-ns"
	name := "f-kafka-auth"

	t.Run("removes an existing TriggerAuthentication", func(t *testing.T) {
		dynClient := newScalingDynClient(unstructuredTriggerAuth(name, ns))
		if err := deleteTriggerAuth(t.Context(), dynClient, ns, name); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		_, err := dynClient.Resource(triggerAuthGVR).Namespace(ns).Get(t.Context(), name, metav1.GetOptions{})
		if !k8serrors.IsNotFound(err) {
			t.Errorf("expected TriggerAuthentication to be gone, got err: %v", err)
		}
	})

	t.Run("not-found is not an error", func(t *testing.T) {
		dynClient := newScalingDynClient()
		if err := deleteTriggerAuth(t.Context(), dynClient, ns, name); err != nil {
			t.Fatalf("expected nil error for a non-existent TriggerAuthentication, got: %v", err)
		}
	})
}

func TestTriggers_NoScale(t *testing.T) {
	f := fn.Function{Name: "test"}
	got := triggers(f)
	if len(got) != 1 || got[0].Type != "http" {
		t.Errorf("expected [http] fallback, got %v", got)
	}
}

func TestTriggers_Explicit(t *testing.T) {
	lag := int64(5)
	f := fn.Function{
		Name: "test",
		Scale: &fn.ScaleOptions{
			KEDA: &fn.KEDAScaleOptions{
				Triggers: []fn.KEDATrigger{
					{Type: "kafka", LagThreshold: &lag},
				},
			},
		},
	}
	got := triggers(f)
	if len(got) != 1 {
		t.Fatalf("expected 1 trigger, got %d", len(got))
	}
	if got[0].Type != "kafka" {
		t.Errorf("expected kafka, got %s", got[0].Type)
	}
	if *got[0].LagThreshold != 5 {
		t.Errorf("expected lagThreshold 5, got %d", *got[0].LagThreshold)
	}
}

// TestTriggers_ExplicitlyEmpty documents the precondition Deploy's
// len(triggers) == 0 guard depends on: scale.keda present with an
// explicitly empty triggers list returns an empty slice here (unlike a nil
// Scale/KEDA, which falls back to a plain http trigger), which would
// otherwise make Deploy skip both the HTTPScaledObject and Kafka
// ScaledObject paths and deploy with no scaler at all.
func TestTriggers_ExplicitlyEmpty(t *testing.T) {
	f := fn.Function{
		Name:  "test",
		Scale: &fn.ScaleOptions{KEDA: &fn.KEDAScaleOptions{Triggers: []fn.KEDATrigger{}}},
	}
	if got := triggers(f); len(got) != 0 {
		t.Fatalf("expected an explicitly empty triggers list to stay empty, got %v", got)
	}
}

// TestTriggers_KPAWithoutKEDA documents that a bypass caller setting
// scale.kpa (incompatible with deployer: keda) without scale.keda must not
// silently fall back to the default http trigger -- that would treat an
// invalid config as valid instead of letting Deploy's empty-triggers guard
// reject it.
func TestTriggers_KPAWithoutKEDA(t *testing.T) {
	f := fn.Function{
		Name:  "test",
		Scale: &fn.ScaleOptions{KPA: &fn.KPAScaleOptions{Metric: strPtr("concurrency")}},
	}
	if got := triggers(f); len(got) != 0 {
		t.Fatalf("expected no triggers for scale.kpa without scale.keda, got %v", got)
	}
}

// TestHasKafkaTrigger_WithoutRunKafka documents the precondition
// pkg/keda/deployer.go's Deploy guards against: a kafka trigger declared in
// scale.keda.triggers with no run.kafka configured is exactly the case
// ValidateScale already rejects, but Deploy must reject it too for callers
// that reach Deploy without going through Function.Validate first.
func TestHasKafkaTrigger_WithoutRunKafka(t *testing.T) {
	f := fn.Function{
		Name: "test",
		Scale: &fn.ScaleOptions{
			KEDA: &fn.KEDAScaleOptions{
				Triggers: []fn.KEDATrigger{{Type: "kafka"}},
			},
		},
		// Run.Kafka intentionally left nil.
	}
	if !hasKafkaTrigger(triggers(f)) {
		t.Fatal("expected hasKafkaTrigger to be true")
	}
	if f.Run.Kafka != nil {
		t.Fatal("expected Run.Kafka to be nil for this test")
	}
}

// TestHasHTTPKafkaTrigger_UnsupportedType documents the precondition
// Deploy's trigger-type guard depends on: a trigger of an unrecognized
// type makes both hasHTTPTrigger and hasKafkaTrigger false, which would
// otherwise make Deploy silently skip every scaler path instead of
// failing.
func TestHasHTTPKafkaTrigger_UnsupportedType(t *testing.T) {
	f := fn.Function{
		Name: "test",
		Scale: &fn.ScaleOptions{
			KEDA: &fn.KEDAScaleOptions{
				Triggers: []fn.KEDATrigger{{Type: "cron"}},
			},
		},
	}
	got := triggers(f)
	if hasHTTPTrigger(got) {
		t.Error("expected hasHTTPTrigger to be false for an unsupported type")
	}
	if hasKafkaTrigger(got) {
		t.Error("expected hasKafkaTrigger to be false for an unsupported type")
	}
}

func TestParseSecretRef(t *testing.T) {
	tests := []struct {
		input    string
		wantName string
		wantKey  string
	}{
		{"{{ secret:my-secret:my-key }}", "my-secret", "my-key"},
		{"{{ secret:foo:bar }}", "foo", "bar"},
		{"plaintext-value", "", ""},
		{"{{ configMap:cm:key }}", "", ""},
		{"{{ invalid }}", "", ""},
		// Trailing garbage after "}}": the old ad-hoc Trim/Split parsing only
		// stripped matching cutset characters from the string's own ends, so
		// this left "key }} extra" as the parsed key instead of rejecting the
		// whole value. fn.TemplateRefPattern anchors on "$" and rejects it.
		{"{{ secret:name:key }} extra", "", ""},
	}
	for _, tt := range tests {
		name, key := parseSecretRef(tt.input)
		if name != tt.wantName || key != tt.wantKey {
			t.Errorf("parseSecretRef(%q) = (%q, %q), want (%q, %q)", tt.input, name, key, tt.wantName, tt.wantKey)
		}
	}
}

func TestFindSecretForPath(t *testing.T) {
	secret := "my-cluster-ca"
	path := "/etc/kafka/ca"
	volumes := []fn.Volume{
		{Secret: &secret, Path: &path},
	}

	name, key := findSecretForPath("/etc/kafka/ca/ca.crt", volumes)
	if name != "my-cluster-ca" || key != "ca.crt" {
		t.Errorf("got (%q, %q), want (my-cluster-ca, ca.crt)", name, key)
	}

	name, _ = findSecretForPath("/other/path", volumes)
	if name != "" {
		t.Errorf("expected empty for non-matching path, got %q", name)
	}

	// Sibling directory sharing a prefix must not match (e.g. "/etc/kafka/ca"
	// is not a parent of "/etc/kafka/cab/ca.crt").
	name, _ = findSecretForPath("/etc/kafka/cab/ca.crt", volumes)
	if name != "" {
		t.Errorf("expected empty for sibling-directory path, got %q", name)
	}

	// certPath equal to the mount path names the directory, not a file in
	// it: no valid Secret data key, so this must not match.
	name, _ = findSecretForPath("/etc/kafka/ca", volumes)
	if name != "" {
		t.Errorf("expected empty when certPath is the mount path itself, got %q", name)
	}

	// A cert nested more than one level deep resolves to a rel containing a
	// separator, which isn't a valid Secret data key either.
	name, _ = findSecretForPath("/etc/kafka/ca/sub/ca.crt", volumes)
	if name != "" {
		t.Errorf("expected empty for a cert nested more than one level deep, got %q", name)
	}
}

func testDeployment() *v1.Deployment {
	return &v1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-func",
			Namespace: "default",
			UID:       types.UID("test-uid-123"),
		},
		Spec: v1.DeploymentSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{Name: "user-container"}},
				},
			},
		},
	}
}

// TestOwnerReferences_OmitBlockOwnerDeletion guards against reintroducing
// blockOwnerDeletion: true on the TriggerAuthentication/ScaledObject owner
// references. That flag makes OpenShift's OwnerReferencesPermissionEnforcement
// admission plugin require a finalizers-update grant on the owning
// Deployment that the deploying/pipeline service account doesn't hold by
// default, rejecting the create outright -- the same problem already fixed
// once for the Service owner reference in pkg/k8s/deployer.go.
func TestOwnerReferences_OmitBlockOwnerDeletion(t *testing.T) {
	f := fn.Function{
		Name: "test-func",
		Run: fn.RunSpec{
			Kafka: &fn.KafkaConfig{
				Brokers: "broker:9093", Topic: "t", ConsumerGroup: "g",
				SASL: &fn.KafkaSASL{Mechanism: "PLAIN", Password: "{{ secret:s:k }}"},
			},
		},
	}
	deployment := testDeployment()

	ta, err := buildTriggerAuth(f, deployment, "default")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ta == nil {
		t.Fatal("expected TriggerAuthentication, got nil")
	}
	for _, ref := range ta.GetOwnerReferences() {
		if ref.BlockOwnerDeletion != nil && *ref.BlockOwnerDeletion {
			t.Errorf("TriggerAuthentication owner reference must not set blockOwnerDeletion: true, got %+v", ref)
		}
	}

	so := buildScaledObject(f, fn.KEDATrigger{Type: "kafka"}, deployment, "default", 0, 10)
	if so == nil {
		t.Fatal("expected ScaledObject, got nil")
	}
	for _, ref := range so.GetOwnerReferences() {
		if ref.BlockOwnerDeletion != nil && *ref.BlockOwnerDeletion {
			t.Errorf("ScaledObject owner reference must not set blockOwnerDeletion: true, got %+v", ref)
		}
	}
}

func TestBuildTriggerAuth(t *testing.T) {
	f := fn.Function{
		Name: "test-func",
		Run: fn.RunSpec{
			Kafka: &fn.KafkaConfig{
				Brokers:          "broker:9093",
				Topic:            "topic",
				ConsumerGroup:    "group",
				SecurityProtocol: "SASL_SSL",
				TLS: &fn.KafkaTLS{
					CACert: "/etc/kafka/ca/ca.crt",
				},
				SASL: &fn.KafkaSASL{
					Mechanism: "SCRAM-SHA-512",
					User:      "admin",
					Password:  "{{ secret:my-user:password }}",
				},
			},
			Volumes: []fn.Volume{
				{Secret: strPtr("my-cluster-ca"), Path: strPtr("/etc/kafka/ca")},
			},
		},
	}

	ta, err := buildTriggerAuth(f, testDeployment(), "default")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ta == nil {
		t.Fatal("expected TriggerAuthentication, got nil")
	}

	if ta.GetName() != "test-func-kafka-auth" {
		t.Errorf("name = %q, want test-func-kafka-auth", ta.GetName())
	}

	spec, ok := ta.Object["spec"].(map[string]interface{})
	if !ok {
		t.Fatal("missing spec")
	}
	refs, ok := spec["secretTargetRef"].([]interface{})
	if !ok {
		t.Fatal("missing secretTargetRef")
	}
	if len(refs) != 2 {
		t.Fatalf("expected 2 secretTargetRef entries, got %d", len(refs))
	}

	ref0 := refs[0].(map[string]interface{})
	if ref0["parameter"] != "password" || ref0["name"] != "my-user" || ref0["key"] != "password" {
		t.Errorf("unexpected password ref: %v", ref0)
	}

	ref1 := refs[1].(map[string]interface{})
	if ref1["parameter"] != "ca" || ref1["name"] != "my-cluster-ca" || ref1["key"] != "ca.crt" {
		t.Errorf("unexpected ca ref: %v", ref1)
	}

	envs, ok := spec["env"].([]interface{})
	if !ok {
		t.Fatal("missing env")
	}
	if len(envs) != 1 {
		t.Fatalf("expected 1 env entry, got %d", len(envs))
	}
	env0 := envs[0].(map[string]interface{})
	if env0["parameter"] != "username" || env0["name"] != "KAFKA_SASL_USER" {
		t.Errorf("unexpected env ref: %v", env0)
	}
}

func TestNeedsTriggerAuth_MutualTLSOnly(t *testing.T) {
	// mTLS with no CA cert and no SASL: needsTriggerAuth must still be true,
	// or buildTriggerAuth is never even consulted and the client cert/key
	// never make it into a TriggerAuthentication.
	kafka := &fn.KafkaConfig{
		TLS: &fn.KafkaTLS{
			ClientCert: "/etc/kafka/tls/tls.crt",
			ClientKey:  "/etc/kafka/tls/tls.key",
		},
	}
	if !needsTriggerAuth(kafka) {
		t.Error("expected needsTriggerAuth to be true for mTLS-only config")
	}
}

// TestBuildTriggerAuth_UnresolvedCACertReturnsError documents that an
// explicitly-configured TLS path that doesn't match any configured volume
// is an error, not a silent skip: pkg/keda/deployer.go's Deploy relies on
// this to fail fast instead of proceeding to create a ScaledObject whose
// authenticationRef points at a TriggerAuthentication that was never
// created (or, before this fix, one missing the credential it needs).
func TestBuildTriggerAuth_UnresolvedCACertReturnsError(t *testing.T) {
	f := fn.Function{
		Name: "test-func",
		Run: fn.RunSpec{
			Kafka: &fn.KafkaConfig{
				Brokers:       "broker:9093",
				Topic:         "topic",
				ConsumerGroup: "group",
				TLS: &fn.KafkaTLS{
					CACert: "/etc/kafka/ca/ca.crt",
				},
			},
			// No volume backs /etc/kafka/ca: findSecretForPath finds nothing.
		},
	}

	if !needsTriggerAuth(f.Run.Kafka) {
		t.Fatal("expected needsTriggerAuth to be true")
	}
	ta, err := buildTriggerAuth(f, testDeployment(), "default")
	if err == nil {
		t.Fatal("expected an error when the CA cert path matches no volume")
	}
	if ta != nil {
		t.Errorf("expected nil TriggerAuthentication alongside the error, got %v", ta)
	}
}

// TestValidateKafkaTLSPaths documents that this check depends only on
// static inputs (kafka config + volumes, no live deployment) -- the
// property pkg/keda/deployer.go's Deploy relies on to run it as a
// preflight guard before the raw Deployment/Service exist, instead of
// only inside buildTriggerAuth after they already do.
func TestValidateKafkaTLSPaths(t *testing.T) {
	volumes := []fn.Volume{
		{Secret: strPtr("my-cluster-ca"), Path: strPtr("/etc/kafka/ca")},
	}

	if err := validateKafkaTLSPaths(nil, volumes); err != nil {
		t.Errorf("nil kafka config: expected no error, got %v", err)
	}
	if err := validateKafkaTLSPaths(&fn.KafkaConfig{}, volumes); err != nil {
		t.Errorf("no TLS configured: expected no error, got %v", err)
	}
	if err := validateKafkaTLSPaths(&fn.KafkaConfig{
		TLS: &fn.KafkaTLS{CACert: "/etc/kafka/ca/ca.crt"},
	}, volumes); err != nil {
		t.Errorf("resolvable CA cert: expected no error, got %v", err)
	}
	if err := validateKafkaTLSPaths(&fn.KafkaConfig{
		TLS: &fn.KafkaTLS{CACert: "/no/such/path/ca.crt"},
	}, volumes); err == nil {
		t.Error("unresolvable CA cert: expected an error")
	}
}

// TestBuildTriggerAuth_PartiallyResolvedMutualTLSReturnsError covers the
// case Copilot flagged: the CA cert resolves to a Secret, but the client
// cert doesn't match any configured volume. Each TLS field used to be
// optionalized independently, so this returned a TriggerAuthentication
// containing only the "ca" entry -- needsTriggerAuth was satisfied by the
// CA alone, so the ScaledObject would reference a TriggerAuthentication
// missing the client cert/key mTLS needs, and the scaler would silently
// fail to authenticate. An explicitly-configured path that fails to
// resolve must error regardless of whether other fields resolved fine.
func TestBuildTriggerAuth_PartiallyResolvedMutualTLSReturnsError(t *testing.T) {
	f := fn.Function{
		Name: "test-func",
		Run: fn.RunSpec{
			Kafka: &fn.KafkaConfig{
				Brokers:       "broker:9093",
				Topic:         "topic",
				ConsumerGroup: "group",
				TLS: &fn.KafkaTLS{
					CACert:     "/etc/kafka/ca/ca.crt",
					ClientCert: "/etc/kafka/tls/tls.crt",
					ClientKey:  "/etc/kafka/tls/tls.key",
				},
			},
			Volumes: []fn.Volume{
				// Only the CA volume is configured; client cert/key are not.
				{Secret: strPtr("my-cluster-ca"), Path: strPtr("/etc/kafka/ca")},
			},
		},
	}

	ta, err := buildTriggerAuth(f, testDeployment(), "default")
	if err == nil {
		t.Fatal("expected an error when the client cert path matches no volume, even though the CA resolved")
	}
	if ta != nil {
		t.Errorf("expected nil TriggerAuthentication alongside the error, got %v", ta)
	}
}

func TestBuildTriggerAuth_MutualTLS(t *testing.T) {
	f := fn.Function{
		Name: "test-func",
		Run: fn.RunSpec{
			Kafka: &fn.KafkaConfig{
				Brokers:          "broker:9093",
				Topic:            "topic",
				ConsumerGroup:    "group",
				SecurityProtocol: "SSL",
				TLS: &fn.KafkaTLS{
					CACert:     "/etc/kafka/ca/ca.crt",
					ClientCert: "/etc/kafka/tls/tls.crt",
					ClientKey:  "/etc/kafka/tls/tls.key",
				},
			},
			Volumes: []fn.Volume{
				{Secret: strPtr("my-cluster-ca"), Path: strPtr("/etc/kafka/ca")},
				{Secret: strPtr("my-client-tls"), Path: strPtr("/etc/kafka/tls")},
			},
		},
	}

	ta, err := buildTriggerAuth(f, testDeployment(), "default")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ta == nil {
		t.Fatal("expected TriggerAuthentication, got nil")
	}

	spec, ok := ta.Object["spec"].(map[string]interface{})
	if !ok {
		t.Fatal("missing spec")
	}
	refs, ok := spec["secretTargetRef"].([]interface{})
	if !ok {
		t.Fatal("missing secretTargetRef")
	}
	if len(refs) != 3 {
		t.Fatalf("expected 3 secretTargetRef entries (ca, cert, key), got %d: %v", len(refs), refs)
	}

	want := map[string][2]string{
		"ca":   {"my-cluster-ca", "ca.crt"},
		"cert": {"my-client-tls", "tls.crt"},
		"key":  {"my-client-tls", "tls.key"},
	}
	for _, r := range refs {
		ref := r.(map[string]interface{})
		param := ref["parameter"].(string)
		exp, ok := want[param]
		if !ok {
			t.Errorf("unexpected parameter %q in secretTargetRef", param)
			continue
		}
		if ref["name"] != exp[0] || ref["key"] != exp[1] {
			t.Errorf("parameter %q: got (name=%v, key=%v), want (%q, %q)", param, ref["name"], ref["key"], exp[0], exp[1])
		}
		delete(want, param)
	}
	if len(want) != 0 {
		t.Errorf("missing secretTargetRef parameters: %v", want)
	}
}

func TestBuildTriggerAuth_PlaintextPassword(t *testing.T) {
	f := fn.Function{
		Name: "test-func",
		Run: fn.RunSpec{
			Kafka: &fn.KafkaConfig{
				Brokers:          "broker:9093",
				Topic:            "topic",
				ConsumerGroup:    "group",
				SecurityProtocol: "SASL_SSL",
				SASL: &fn.KafkaSASL{
					Mechanism: "SCRAM-SHA-512",
					User:      "admin",
					// plaintext, not a {{ secret:... }} reference -- allowed, at
					// least for debugging purposes.
					Password: "plaintext-password",
				},
			},
		},
	}

	ta, err := buildTriggerAuth(f, testDeployment(), "default")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ta == nil {
		t.Fatal("expected TriggerAuthentication, got nil")
	}

	spec, ok := ta.Object["spec"].(map[string]interface{})
	if !ok {
		t.Fatal("missing spec")
	}

	// A plaintext password must not be silently dropped: it should be wired
	// up as an env-based auth reference pointing at the KAFKA_SASL_PASSWORD
	// env var that pkg/k8s/deployer.go sets on the function's container.
	if _, ok := spec["secretTargetRef"]; ok {
		t.Error("did not expect secretTargetRef for a plaintext password")
	}

	envs, ok := spec["env"].([]interface{})
	if !ok {
		t.Fatal("missing env")
	}
	if len(envs) != 2 {
		t.Fatalf("expected 2 env entries (username + password), got %d", len(envs))
	}

	var sawUser, sawPassword bool
	for _, e := range envs {
		entry := e.(map[string]interface{})
		switch entry["parameter"] {
		case "username":
			sawUser = true
			if entry["name"] != "KAFKA_SASL_USER" {
				t.Errorf("username env name = %v, want KAFKA_SASL_USER", entry["name"])
			}
		case "password":
			sawPassword = true
			if entry["name"] != "KAFKA_SASL_PASSWORD" {
				t.Errorf("password env name = %v, want KAFKA_SASL_PASSWORD", entry["name"])
			}
		}
	}
	if !sawUser {
		t.Error("expected a username env ref")
	}
	if !sawPassword {
		t.Error("expected a password env ref")
	}
}

func TestBuildScaledObject(t *testing.T) {
	lag := int64(20)
	f := fn.Function{
		Name: "test-func",
		Run: fn.RunSpec{
			Kafka: &fn.KafkaConfig{
				Brokers:          "broker:9093",
				Topic:            "my-topic",
				ConsumerGroup:    "my-group",
				SecurityProtocol: "SASL_SSL",
				TLS:              &fn.KafkaTLS{CACert: "/etc/kafka/ca/ca.crt"},
				SASL:             &fn.KafkaSASL{Mechanism: "SCRAM-SHA-512", Password: "{{ secret:s:k }}"},
			},
		},
	}
	trigger := fn.KEDATrigger{Type: "kafka", LagThreshold: &lag}

	so := buildScaledObject(f, trigger, testDeployment(), "default", 0, 10)
	if so == nil {
		t.Fatal("expected ScaledObject, got nil")
	}

	if so.GetName() != "test-func-kafka" {
		t.Errorf("name = %q, want test-func-kafka", so.GetName())
	}

	spec := so.Object["spec"].(map[string]interface{})
	if spec["minReplicaCount"] != int64(0) {
		t.Errorf("minReplicaCount = %v, want 0", spec["minReplicaCount"])
	}
	if spec["maxReplicaCount"] != int64(10) {
		t.Errorf("maxReplicaCount = %v, want 10", spec["maxReplicaCount"])
	}

	triggers := spec["triggers"].([]interface{})
	if len(triggers) != 1 {
		t.Fatalf("expected 1 trigger, got %d", len(triggers))
	}
	trigger0 := triggers[0].(map[string]interface{})
	meta := trigger0["metadata"].(map[string]interface{})
	if meta["bootstrapServers"] != "broker:9093" {
		t.Errorf("bootstrapServers = %v", meta["bootstrapServers"])
	}
	if meta["lagThreshold"] != "20" {
		t.Errorf("lagThreshold = %v, want 20", meta["lagThreshold"])
	}
	if meta["tls"] != "enable" {
		t.Errorf("tls = %v, want enable", meta["tls"])
	}
	if meta["sasl"] != "scram_sha512" {
		t.Errorf("sasl = %v, want scram_sha512", meta["sasl"])
	}

	authRef := trigger0["authenticationRef"].(map[string]interface{})
	if authRef["name"] != "test-func-kafka-auth" {
		t.Errorf("authenticationRef name = %v", authRef["name"])
	}
}

func TestBuildScaledObject_TLSFromSecurityProtocol(t *testing.T) {
	// SecurityProtocol: SSL with no explicit run.kafka.tls block (relying on
	// the system's CA trust store) must still enable KEDA's tls handshake --
	// gating on kafka.TLS != nil would leave it plaintext for this config.
	f := fn.Function{
		Name: "test-func",
		Run: fn.RunSpec{
			Kafka: &fn.KafkaConfig{
				Brokers:          "broker:9093",
				Topic:            "t",
				ConsumerGroup:    "g",
				SecurityProtocol: "SSL",
			},
		},
	}
	trigger := fn.KEDATrigger{Type: "kafka"}

	so := buildScaledObject(f, trigger, testDeployment(), "default", 0, 10)
	if so == nil {
		t.Fatal("expected ScaledObject, got nil")
	}
	spec := so.Object["spec"].(map[string]interface{})
	trigger0 := spec["triggers"].([]interface{})[0].(map[string]interface{})
	meta := trigger0["metadata"].(map[string]interface{})
	if meta["tls"] != "enable" {
		t.Errorf("tls = %v, want enable for securityProtocol SSL with no explicit tls block", meta["tls"])
	}
}

func TestBuildScaledObject_UnsafeSslFromSkipVerify(t *testing.T) {
	f := fn.Function{
		Name: "test-func",
		Run: fn.RunSpec{
			Kafka: &fn.KafkaConfig{
				Brokers:          "broker:9093",
				Topic:            "t",
				ConsumerGroup:    "g",
				SecurityProtocol: "SSL",
				TLS:              &fn.KafkaTLS{SkipVerify: true},
			},
		},
	}
	trigger := fn.KEDATrigger{Type: "kafka"}

	so := buildScaledObject(f, trigger, testDeployment(), "default", 0, 10)
	if so == nil {
		t.Fatal("expected ScaledObject, got nil")
	}
	spec := so.Object["spec"].(map[string]interface{})
	trigger0 := spec["triggers"].([]interface{})[0].(map[string]interface{})
	meta := trigger0["metadata"].(map[string]interface{})
	if meta["unsafeSsl"] != "true" {
		t.Errorf("unsafeSsl = %v, want \"true\" when run.kafka.tls.skipVerify is set", meta["unsafeSsl"])
	}
}

func TestBuildScaledObject_NoUnsafeSslWithoutSkipVerify(t *testing.T) {
	f := fn.Function{
		Name: "test-func",
		Run: fn.RunSpec{
			Kafka: &fn.KafkaConfig{
				Brokers:          "broker:9093",
				Topic:            "t",
				ConsumerGroup:    "g",
				SecurityProtocol: "SSL",
				TLS:              &fn.KafkaTLS{CACert: "/etc/kafka/ca/ca.crt"},
			},
		},
	}
	trigger := fn.KEDATrigger{Type: "kafka"}

	so := buildScaledObject(f, trigger, testDeployment(), "default", 0, 10)
	if so == nil {
		t.Fatal("expected ScaledObject, got nil")
	}
	spec := so.Object["spec"].(map[string]interface{})
	trigger0 := spec["triggers"].([]interface{})[0].(map[string]interface{})
	meta := trigger0["metadata"].(map[string]interface{})
	if _, ok := meta["unsafeSsl"]; ok {
		t.Errorf("expected no unsafeSsl key when skipVerify is false, got %v", meta["unsafeSsl"])
	}
}

func TestBuildScaledObject_NoTLSForPlaintext(t *testing.T) {
	f := fn.Function{
		Name: "test-func",
		Run: fn.RunSpec{
			Kafka: &fn.KafkaConfig{
				Brokers:          "broker:9092",
				Topic:            "t",
				ConsumerGroup:    "g",
				SecurityProtocol: "PLAINTEXT",
			},
		},
	}
	trigger := fn.KEDATrigger{Type: "kafka"}

	so := buildScaledObject(f, trigger, testDeployment(), "default", 0, 10)
	if so == nil {
		t.Fatal("expected ScaledObject, got nil")
	}
	spec := so.Object["spec"].(map[string]interface{})
	trigger0 := spec["triggers"].([]interface{})[0].(map[string]interface{})
	meta := trigger0["metadata"].(map[string]interface{})
	if _, ok := meta["tls"]; ok {
		t.Errorf("expected no tls key for securityProtocol PLAINTEXT, got %v", meta["tls"])
	}
}

func TestBuildScaledObject_DefaultLag(t *testing.T) {
	f := fn.Function{
		Name: "test-func",
		Run: fn.RunSpec{
			Kafka: &fn.KafkaConfig{
				Brokers:       "broker:9092",
				Topic:         "t",
				ConsumerGroup: "g",
			},
		},
	}
	trigger := fn.KEDATrigger{Type: "kafka"}

	so := buildScaledObject(f, trigger, testDeployment(), "default", 1, 5)
	if so == nil {
		t.Fatal("expected ScaledObject, got nil")
	}

	spec := so.Object["spec"].(map[string]interface{})
	triggers := spec["triggers"].([]interface{})
	trigger0 := triggers[0].(map[string]interface{})
	meta := trigger0["metadata"].(map[string]interface{})
	if meta["lagThreshold"] != "10" {
		t.Errorf("default lagThreshold = %v, want 10", meta["lagThreshold"])
	}

	// No TLS/SASL, so no authenticationRef
	if _, ok := trigger0["authenticationRef"]; ok {
		t.Error("expected no authenticationRef for plaintext Kafka")
	}
}

func TestKedaSASLType(t *testing.T) {
	tests := map[string]string{
		"SCRAM-SHA-256": "scram_sha256",
		"SCRAM-SHA-512": "scram_sha512",
		"PLAIN":         "plaintext",
		"UNKNOWN":       "",
	}
	for in, want := range tests {
		if got := kedaSASLType(in); got != want {
			t.Errorf("kedaSASLType(%q) = %q, want %q", in, got, want)
		}
	}
}

func strPtr(s string) *string { return &s }
