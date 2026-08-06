package k8s

import (
	"os"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/sets"
	dynamicfakeclient "k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/kubernetes/fake"
	fn "knative.dev/func/pkg/functions"
)

func Test_SetHealthEndpoints(t *testing.T) {
	f := fn.Function{
		Name: "testing",
		Deploy: fn.DeploySpec{
			HealthEndpoints: fn.HealthEndpoints{
				Liveness:  "/lively",
				Readiness: "/readyAsIllEverBe",
			},
		},
	}
	c := corev1.Container{}
	SetHealthEndpoints(f, &c)
	got := c.LivenessProbe.HTTPGet.Path
	if got != "/lively" {
		t.Errorf("expected \"/lively\" but got %v", got)
	}
	got = c.ReadinessProbe.HTTPGet.Path
	if got != "/readyAsIllEverBe" {
		t.Errorf("expected \"readyAsIllEverBe\" but got %v", got)
	}
}

func Test_SetHealthEndpointDefaults(t *testing.T) {
	f := fn.Function{
		Name: "testing",
	}
	c := corev1.Container{}
	SetHealthEndpoints(f, &c)
	got := c.LivenessProbe.HTTPGet.Path
	if got != DefaultLivenessEndpoint {
		t.Errorf("expected \"%v\" but got %v", DefaultLivenessEndpoint, got)
	}
	got = c.ReadinessProbe.HTTPGet.Path
	if got != DefaultReadinessEndpoint {
		t.Errorf("expected \"%v\" but got %v", DefaultReadinessEndpoint, got)
	}
}

func Test_processValue(t *testing.T) {
	testEnvVarOld, testEnvVarOldExists := os.LookupEnv("TEST_KNATIVE_DEPLOYER")
	os.Setenv("TEST_KNATIVE_DEPLOYER", "VALUE_FOR_TEST_KNATIVE_DEPLOYER")
	defer func() {
		if testEnvVarOldExists {
			os.Setenv("TEST_KNATIVE_DEPLOYER", testEnvVarOld)
		} else {
			os.Unsetenv("TEST_KNATIVE_DEPLOYER")
		}
	}()

	unsetVarOld, unsetVarOldExists := os.LookupEnv("UNSET_VAR")
	os.Unsetenv("UNSET_VAR")
	defer func() {
		if unsetVarOldExists {
			os.Setenv("UNSET_VAR", unsetVarOld)
		}
	}()

	tests := []struct {
		name    string
		arg     string
		want    string
		wantErr bool
	}{
		{name: "simple value", arg: "A_VALUE", want: "A_VALUE", wantErr: false},
		{name: "using envvar value", arg: "{{ env:TEST_KNATIVE_DEPLOYER }}", want: "VALUE_FOR_TEST_KNATIVE_DEPLOYER", wantErr: false},
		{name: "bad context", arg: "{{secret:S}}", want: "", wantErr: true},
		{name: "unset envvar", arg: "{{env:SOME_UNSET_VAR}}", want: "", wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := processLocalEnvValue(test.arg)
			if (err != nil) != test.wantErr {
				t.Errorf("processValue() error = %v, wantErr %v", err, test.wantErr)
				return
			}
			if got != test.want {
				t.Errorf("processValue() got = %v, want %v", got, test.want)
			}
		})
	}
}

func Test_ImagePullSecrets(t *testing.T) {
	t.Run("empty secret returns nil", func(t *testing.T) {
		refs := ImagePullSecrets("")
		if refs != nil {
			t.Errorf("expected nil, got %v", refs)
		}
	})

	t.Run("non-empty secret returns single reference", func(t *testing.T) {
		refs := ImagePullSecrets("my-secret")
		if len(refs) != 1 {
			t.Fatalf("expected 1 reference, got %d", len(refs))
		}
		if refs[0].Name != "my-secret" {
			t.Errorf("expected name 'my-secret', got '%s'", refs[0].Name)
		}
	})
}

func Test_generateDeployment_ImagePullSecret(t *testing.T) {
	d := &Deployer{}

	t.Run("with image pull secret", func(t *testing.T) {
		f := fn.Function{
			Name: "test-func",
			Deploy: fn.DeploySpec{
				Image:           "registry.example.com/test:latest",
				ImagePullSecret: "my-registry-secret",
			},
		}
		rs, rcm, rpvc := sets.New[string](), sets.New[string](), sets.New[string]()
		deployment, err := d.generateDeployment(f, "default", false, &rs, &rcm, &rpvc)
		if err != nil {
			t.Fatal(err)
		}
		secrets := deployment.Spec.Template.Spec.ImagePullSecrets
		if len(secrets) != 1 || secrets[0].Name != "my-registry-secret" {
			t.Errorf("expected ImagePullSecrets [{my-registry-secret}], got %v", secrets)
		}
	})

	t.Run("without image pull secret", func(t *testing.T) {
		f := fn.Function{
			Name: "test-func",
			Deploy: fn.DeploySpec{
				Image: "registry.example.com/test:latest",
			},
		}
		rs, rcm, rpvc := sets.New[string](), sets.New[string](), sets.New[string]()
		deployment, err := d.generateDeployment(f, "default", false, &rs, &rcm, &rpvc)
		if err != nil {
			t.Fatal(err)
		}
		secrets := deployment.Spec.Template.Spec.ImagePullSecrets
		if secrets != nil {
			t.Errorf("expected no ImagePullSecrets, got %v", secrets)
		}
	})
}

// Tests for generateTriggerName

func TestGenerateTriggerName_Deterministic(t *testing.T) {
	functionName := "order-processor"
	broker := "default"
	filters := map[string]string{
		"type":   "order.created",
		"source": "api",
	}

	// Call multiple times with same input
	name1 := generateTriggerName(functionName, broker, filters)
	name2 := generateTriggerName(functionName, broker, filters)
	name3 := generateTriggerName(functionName, broker, filters)

	// Should always produce the same result
	if name1 != name2 || name2 != name3 || name1 != name3 {
		t.Errorf("generateTriggerName() is not deterministic: got %v, %v, %v", name1, name2, name3)
	}
}

func TestGenerateTriggerName_FilterOrderIndependent(t *testing.T) {
	functionName := "order-processor"
	broker := "default"

	// Same filters, different order
	filters1 := map[string]string{
		"type":   "order.created",
		"status": "pending",
		"source": "api",
	}

	filters2 := map[string]string{
		"source": "api",
		"type":   "order.created",
		"status": "pending",
	}

	filters3 := map[string]string{
		"status": "pending",
		"source": "api",
		"type":   "order.created",
	}

	name1 := generateTriggerName(functionName, broker, filters1)
	name2 := generateTriggerName(functionName, broker, filters2)
	name3 := generateTriggerName(functionName, broker, filters3)

	// Should produce the same hash regardless of map iteration order
	if name1 != name2 || name2 != name3 || name1 != name3 {
		t.Errorf("generateTriggerName() is sensitive to filter order: got %v, %v, %v", name1, name2, name3)
	}
}

func TestGenerateTriggerName_DifferentInputsDifferentNames(t *testing.T) {
	functionName := "order-processor"
	broker := "default"

	// Different filters should produce different names
	name1 := generateTriggerName(functionName, broker, map[string]string{"type": "order.created"})
	name2 := generateTriggerName(functionName, broker, map[string]string{"type": "order.paid"})
	name3 := generateTriggerName(functionName, broker, map[string]string{"type": "order.shipped"})

	if name1 == name2 || name2 == name3 || name1 == name3 {
		t.Errorf("generateTriggerName() produced same name for different filters: %v, %v, %v", name1, name2, name3)
	}

	// Different brokers should produce different names
	name4 := generateTriggerName(functionName, "default", map[string]string{"type": "order.created"})
	name5 := generateTriggerName(functionName, "production", map[string]string{"type": "order.created"})

	if name4 == name5 {
		t.Errorf("generateTriggerName() produced same name for different brokers: %v, %v", name4, name5)
	}

	// Different function names should produce different names
	name6 := generateTriggerName("order-processor", broker, map[string]string{"type": "order.created"})
	name7 := generateTriggerName("payment-processor", broker, map[string]string{"type": "order.created"})

	if name6 == name7 {
		t.Errorf("generateTriggerName() produced same name for different functions: %v, %v", name6, name7)
	}
}

func TestGenerateTriggerName_ValidKubernetesName(t *testing.T) {
	tests := []struct {
		name         string
		functionName string
		broker       string
		filters      map[string]string
	}{
		{
			name:         "standard case",
			functionName: "order-processor",
			broker:       "default",
			filters:      map[string]string{"type": "order.created"},
		},
		{
			name:         "long function name",
			functionName: "very-long-function-name-that-might-cause-issues",
			broker:       "default",
			filters:      map[string]string{"type": "test"},
		},
		{
			name:         "many filters",
			functionName: "test-func",
			broker:       "default",
			filters: map[string]string{
				"type":     "order.created",
				"source":   "api",
				"status":   "pending",
				"priority": "high",
			},
		},
		{
			name:         "empty filters",
			functionName: "test-func",
			broker:       "default",
			filters:      map[string]string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := generateTriggerName(tt.functionName, tt.broker, tt.filters)

			// Kubernetes name requirements:
			// - Max 253 characters
			if len(got) > 253 {
				t.Errorf("generateTriggerName() = %v, length %d exceeds Kubernetes limit of 253", got, len(got))
			}

			// Check format matches expected pattern
			if got[:len(tt.functionName)] != tt.functionName {
				t.Errorf("generateTriggerName() = %v, doesn't start with function name %s", got, tt.functionName)
			}

			// Should contain "-trigger-"
			if len(got) < len(tt.functionName)+17 {
				t.Errorf("generateTriggerName() = %v, invalid format (too short)", got)
			}
		})
	}
}

func TestGenerateTriggerName_ReorderingScenario(t *testing.T) {
	// Simulate the reordering scenario from the bug report
	functionName := "order-processor"
	broker := "default"

	// Original order
	sub1 := map[string]string{"type": "order.created"}
	sub2 := map[string]string{"type": "order.paid"}
	sub3 := map[string]string{"type": "order.shipped"}

	name1_original := generateTriggerName(functionName, broker, sub1)
	name2_original := generateTriggerName(functionName, broker, sub2)
	name3_original := generateTriggerName(functionName, broker, sub3)

	// Reordered (sub2, sub1, sub3)
	name2_reordered := generateTriggerName(functionName, broker, sub2)
	name1_reordered := generateTriggerName(functionName, broker, sub1)
	name3_reordered := generateTriggerName(functionName, broker, sub3)

	// Names should be the same regardless of subscription order
	if name1_original != name1_reordered {
		t.Errorf("Reordering changed trigger name for sub1: %v != %v", name1_original, name1_reordered)
	}
	if name2_original != name2_reordered {
		t.Errorf("Reordering changed trigger name for sub2: %v != %v", name2_original, name2_reordered)
	}
	if name3_original != name3_reordered {
		t.Errorf("Reordering changed trigger name for sub3: %v != %v", name3_original, name3_reordered)
	}
}

// TestGenerateTriggerName_TriggerNamingConsistency verifies that the naming
// follows a consistent pattern across multiple subscriptions
func TestGenerateTriggerName_TriggerNamingConsistency(t *testing.T) {
	functionName := "order-processor"

	// Simulate subscriptions from func.yaml
	subscriptions := []struct {
		source  string
		filters map[string]string
	}{
		{
			source:  "default",
			filters: map[string]string{"type": "com.example.order.created"},
		},
		{
			source:  "default",
			filters: map[string]string{"type": "com.example.order.paid"},
		},
		{
			source:  "default",
			filters: map[string]string{"type": "com.example.order.shipped"},
		},
	}

	triggerNames := make(map[string]bool)

	for _, sub := range subscriptions {
		name := generateTriggerName(functionName, sub.source, sub.filters)

		// Each trigger should have a unique name
		if triggerNames[name] {
			t.Errorf("Duplicate trigger name generated: %s", name)
		}
		triggerNames[name] = true

		// Verify name format
		if len(name) < len(functionName)+17 { // functionName + "-trigger-" + 8 hex chars
			t.Errorf("Trigger name too short: %s", name)
		}
	}

	// Verify we generated 3 unique names
	if len(triggerNames) != 3 {
		t.Errorf("Expected 3 unique trigger names, got %d", len(triggerNames))
	}
}

// TestGenerateTriggerName_EmptyFilters verifies behavior with empty filters
func TestGenerateTriggerName_EmptyFilters(t *testing.T) {
	name := generateTriggerName("test-func", "default", map[string]string{})

	// Should still generate a valid name based on broker alone
	expectedPrefix := "test-func-trigger-"
	if len(name) <= len(expectedPrefix) {
		t.Errorf("Expected name to have hash suffix, got: %s", name)
	}

	if name[:len(expectedPrefix)] != expectedPrefix {
		t.Errorf("Expected prefix %s, got: %s", expectedPrefix, name)
	}
}

// TestGenerateTriggerName_SpecialCharacters verifies handling of special chars in filters
func TestGenerateTriggerName_SpecialCharacters(t *testing.T) {
	// Filters with special characters
	filters := map[string]string{
		"type":   "com.example.order/created",
		"source": "https://api.example.com",
	}

	name := generateTriggerName("test-func", "default", filters)

	// Name should be valid despite special chars in filters
	expectedPrefix := "test-func-trigger-"
	if name[:len(expectedPrefix)] != expectedPrefix {
		t.Errorf("Expected prefix %s, got: %s", expectedPrefix, name)
	}

	// Hash should be 8 hex characters
	hash := name[len(expectedPrefix):]
	if len(hash) != 8 {
		t.Errorf("Expected 8-char hash, got %d chars: %s", len(hash), hash)
	}
}

// TestGenerateTriggerName_DifferentBrokers verifies different brokers produce different names
func TestGenerateTriggerName_DifferentBrokers(t *testing.T) {
	filters := map[string]string{"type": "test.event"}

	name1 := generateTriggerName("test-func", "default", filters)
	name2 := generateTriggerName("test-func", "production", filters)
	name3 := generateTriggerName("test-func", "staging", filters)

	if name1 == name2 || name2 == name3 || name1 == name3 {
		t.Errorf("Different brokers should produce different names: %s, %s, %s", name1, name2, name3)
	}
}

func TestAppendKafkaEnvs_Nil(t *testing.T) {
	base := []corev1.EnvVar{
		{Name: "EXISTING", Value: "value"},
	}
	got := AppendKafkaEnvs(base, nil)
	if len(got) != 1 {
		t.Fatalf("expected 1 env var, got %d", len(got))
	}
	if got[0].Name != "EXISTING" || got[0].Value != "value" {
		t.Errorf("expected existing env var unchanged, got %v", got[0])
	}
}

func TestAppendKafkaEnvs_AllFields(t *testing.T) {
	base := []corev1.EnvVar{
		{Name: "EXISTING", Value: "value"},
	}
	kafka := &fn.KafkaConfig{
		Brokers:       "broker1:9092,broker2:9092",
		Topic:         "my-topic",
		ConsumerGroup: "my-group",
	}
	got := AppendKafkaEnvs(base, kafka)
	if len(got) != 5 {
		t.Fatalf("expected 5 env vars (1 existing + 4 kafka), got %d", len(got))
	}
	expected := map[string]string{
		"FUNC_TRANSPORT":       "kafka",
		"KAFKA_BROKERS":        "broker1:9092,broker2:9092",
		"KAFKA_TOPIC":          "my-topic",
		"KAFKA_CONSUMER_GROUP": "my-group",
	}
	for _, ev := range got[1:] {
		want, ok := expected[ev.Name]
		if !ok {
			t.Errorf("unexpected env var: %s", ev.Name)
			continue
		}
		if ev.Value != want {
			t.Errorf("env var %s: expected %q, got %q", ev.Name, want, ev.Value)
		}
	}
}

func TestAppendKafkaEnvs_MissingBrokers(t *testing.T) {
	base := []corev1.EnvVar{
		{Name: "EXISTING", Value: "value"},
	}
	kafka := &fn.KafkaConfig{
		Brokers:       "",
		Topic:         "my-topic",
		ConsumerGroup: "my-group",
	}
	got := AppendKafkaEnvs(base, kafka)
	if len(got) != 1 {
		t.Fatalf("expected 1 env var (unchanged), got %d", len(got))
	}
}

func TestAppendKafkaEnvs_MissingTopic(t *testing.T) {
	base := []corev1.EnvVar{
		{Name: "EXISTING", Value: "value"},
	}
	kafka := &fn.KafkaConfig{
		Brokers:       "broker1:9092",
		Topic:         "",
		ConsumerGroup: "my-group",
	}
	got := AppendKafkaEnvs(base, kafka)
	if len(got) != 1 {
		t.Fatalf("expected 1 env var (unchanged), got %d", len(got))
	}
}

func Test_ProcessVolumes_NilPath(t *testing.T) {
	secretName := "my-secret"
	referencedSecrets := sets.New[string]()
	referencedConfigMaps := sets.New[string]()
	referencedPVCs := sets.New[string]()

	volumes := []fn.Volume{
		{Secret: &secretName, Path: nil},
	}

	_, _, err := ProcessVolumes(volumes, &referencedSecrets, &referencedConfigMaps, &referencedPVCs)
	if err == nil {
		t.Fatal("expected error for volume with nil path, got nil")
	}
	if !strings.Contains(err.Error(), "missing required path") {
		t.Fatalf("unexpected error message: %s", err.Error())
	}
}

func Test_ProcessVolumes_NilPathConfigMap(t *testing.T) {
	configMapName := "my-config"
	referencedSecrets := sets.New[string]()
	referencedConfigMaps := sets.New[string]()
	referencedPVCs := sets.New[string]()

	volumes := []fn.Volume{
		{ConfigMap: &configMapName, Path: nil},
	}

	_, _, err := ProcessVolumes(volumes, &referencedSecrets, &referencedConfigMaps, &referencedPVCs)
	if err == nil {
		t.Fatal("expected error for volume with nil path, got nil")
	}
	if !strings.Contains(err.Error(), "missing required path") {
		t.Fatalf("unexpected error message: %s", err.Error())
	}
}

func Test_ProcessVolumes_ValidPath(t *testing.T) {
	secretName := "my-secret"
	path := "/etc/secret"
	referencedSecrets := sets.New[string]()
	referencedConfigMaps := sets.New[string]()
	referencedPVCs := sets.New[string]()

	volumes := []fn.Volume{
		{Secret: &secretName, Path: &path},
	}

	vols, mounts, err := ProcessVolumes(volumes, &referencedSecrets, &referencedConfigMaps, &referencedPVCs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(vols) != 1 {
		t.Fatalf("expected 1 volume, got %d", len(vols))
	}
	if len(mounts) != 1 {
		t.Fatalf("expected 1 mount, got %d", len(mounts))
	}
	if mounts[0].MountPath != "/etc/secret" {
		t.Errorf("expected mount path /etc/secret, got %s", mounts[0].MountPath)
	}
}

// Test_WithDeployerExposureDisabled: exposure is on by default (raw) and off
// with others
func Test_WithDeployerExposureDisabled(t *testing.T) {
	if NewDeployer().exposureDisabled {
		t.Error("expected exposure enabled on a default Deployer")
	}
	if !NewDeployer(WithDeployerExposureDisabled()).exposureDisabled {
		t.Error("expected exposure disabled with WithDeployerExposureDisabled")
	}
}

// Test_ResolveExposure_RouteGatedOnOpenShift: functions are exposed by
// default, so an explicit expose:route request hard-errors off OpenShift
// (the user asked for something impossible), and expose:none (explicit
// opt-out) never requires OpenShift or touches the Route API at all, on
// either platform - removeExposure's Get against an empty fake dynamic
// client returns NotFound immediately. The unset/empty value off OpenShift
// also stays cluster-local, but silently (no error): the default degrading
// gracefully rather than failing an ordinary deploy is exactly the point.
//
// The "route on OpenShift" and "empty on OpenShift" cases are NOT exercised
// here: both fall through to ensureExposure, which waits up to 30s
// (hardcoded) for a router to admit the Route - a real wait against a fake
// client with no controller to populate status would either hang the test
// for 30s or require simulating async status writes, disproportionate for
// this table. That deeper path (EnsureRoute, WaitForRouteAdmitted,
// GenerateRoute) is covered directly and fast in route_test.go instead,
// each with its own short timeout.
//
// Note: SetOpenShiftForTest mutates a package-level bool without a mutex -
// this test must not run with t.Parallel() (see openshift.go).
func Test_ResolveExposure_RouteGatedOnOpenShift(t *testing.T) {
	d := NewDeployer()
	f := fn.Function{Name: "f", Deploy: fn.DeploySpec{Namespace: "ns"}}
	ctx := t.Context()
	clientset := fake.NewClientset()
	dynClient := dynamicfakeclient.NewSimpleDynamicClient(runtime.NewScheme())

	tests := []struct {
		name       string
		expose     string
		openShift  bool
		wantErr    bool
		wantExpose bool
	}{
		{name: "route off OpenShift: hard error", expose: "route", openShift: false, wantErr: true},
		{name: "none off OpenShift: fine", expose: "none", openShift: false},
		{name: "none on OpenShift: fine", expose: "none", openShift: true},
		{name: "empty off OpenShift: fine, cluster-local, no error", expose: "", openShift: false},
		// "empty on OpenShift" is NOT in this table: functions are exposed
		// by default now, so unset+OpenShift takes the same real
		// Route-creation path as explicit expose:route does - excluded
		// here for the same reason "route on OpenShift" already is (see
		// the comment above this test).
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cleanup := SetOpenShiftForTest(tt.openShift)
			defer cleanup()

			f.Deploy.Expose = tt.expose
			url, exposed, err := d.resolveExposure(ctx, f, "ns", clientset, dynClient)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("resolveExposure(%q) on OpenShift=%v: expected an error, got nil", tt.expose, tt.openShift)
				}
				return
			}
			if err != nil {
				t.Fatalf("resolveExposure(%q) on OpenShift=%v: unexpected error: %v", tt.expose, tt.openShift, err)
			}
			if exposed != tt.wantExpose {
				t.Errorf("resolveExposure(%q) on OpenShift=%v: exposed = %v, want %v", tt.expose, tt.openShift, exposed, tt.wantExpose)
			}
			if url == "" {
				t.Errorf("resolveExposure(%q) on OpenShift=%v: expected a non-empty URL", tt.expose, tt.openShift)
			}
		})
	}
}

// Test_referenceCheckMessage asserts that, for every resource kind, a Forbidden
// error yields the access-denied wording and any other error the not-present
// wording, and that both name the kind, the resource and the namespace.
func Test_referenceCheckMessage(t *testing.T) {
	kinds := []struct {
		kind     string
		resource string
	}{
		{"Secret", "secrets"},
		{"ConfigMap", "configmaps"},
		{"PersistentVolumeClaim", "persistentvolumeclaims"},
		{"ServiceAccount", "serviceaccounts"},
		{"image pull Secret", "secrets"},
	}

	for _, k := range kinds {
		gr := schema.GroupResource{Resource: k.resource}

		t.Run(k.kind+"/forbidden", func(t *testing.T) {
			msg := referenceCheckMessage(k.kind, "my-res", "my-ns", apierrors.NewForbidden(gr, "my-res", nil))

			if strings.Contains(msg, "is not present") {
				t.Errorf("a forbidden GET must not claim the resource is absent, got %q", msg)
			}
			if !strings.Contains(msg, "denied") {
				t.Errorf("expected the message to say access was denied, got %q", msg)
			}
			if !strings.Contains(msg, k.kind) || !strings.Contains(msg, "my-res") || !strings.Contains(msg, "my-ns") {
				t.Errorf("expected the message to name kind, resource and namespace, got %q", msg)
			}
		})

		t.Run(k.kind+"/absent", func(t *testing.T) {
			msg := referenceCheckMessage(k.kind, "my-res", "my-ns", apierrors.NewNotFound(gr, "my-res"))

			if !strings.Contains(msg, "is not present") {
				t.Errorf("a genuinely absent resource must be reported as not present, got %q", msg)
			}
			if strings.Contains(msg, "denied") {
				t.Errorf("an absent resource must not be reported as a permissions problem, got %q", msg)
			}
			if !strings.Contains(msg, k.kind) || !strings.Contains(msg, "my-res") || !strings.Contains(msg, "my-ns") {
				t.Errorf("expected the message to name kind, resource and namespace, got %q", msg)
			}
		})
	}

	// A timeout or a conflict must not be reported as a permissions problem.
	t.Run("other errors read as absent", func(t *testing.T) {
		msg := referenceCheckMessage("Secret", "my-res", "my-ns",
			apierrors.NewTimeoutError("too slow", 1))
		if !strings.Contains(msg, "is not present") || strings.Contains(msg, "denied") {
			t.Errorf("expected the not-present wording for a non-forbidden error, got %q", msg)
		}
	})

}
