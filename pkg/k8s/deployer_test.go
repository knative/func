package k8s

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"os"
	"strings"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/dynamic"
	dynamicfakeclient "k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
	"knative.dev/func/pkg/deployer"
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

func testMeta(t *testing.T, f fn.Function) (map[string]string, map[string]string) {
	t.Helper()
	labels, err := deployer.GenerateCommonLabels(f, nil)
	if err != nil {
		t.Fatal(err)
	}
	return labels, deployer.GenerateCommonAnnotations(f, nil, false, KubernetesDeployerName)
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
		labels, anns := testMeta(t, f)
		deployment, err := d.generateDeployment(f, "default", labels, anns, &rs, &rcm, &rpvc)
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
		labels, anns := testMeta(t, f)
		deployment, err := d.generateDeployment(f, "default", labels, anns, &rs, &rcm, &rpvc)
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

// Test_ResolveExposure_NoExposer: a Deployer with no Exposer performs no
// exposure at all. Every valid intent, active or not, leaves the function
// cluster-local and reports applying nothing; only an unrecognized value is
// an error, since that cannot be applied or torn down by anyone. This is the
// state keda's embedded raw Deployer runs in on every deploy, so it must stay
// quiet - keda exposes its own Route afterwards (pkg/keda/exposure.go).
func Test_ResolveExposure_NoExposer(t *testing.T) {
	d := NewDeployer()
	f := fn.Function{Name: "f", Deploy: fn.DeploySpec{Namespace: "ns"}}
	ctx := t.Context()
	clientset := fake.NewClientset()
	dynClient := dynamicfakeclient.NewSimpleDynamicClient(runtime.NewScheme())

	tests := []struct {
		name   string
		expose string
	}{
		{name: "empty: cluster-local", expose: ""},
		{name: "none: cluster-local", expose: "none"},
		{name: "route without exposer: cluster-local, no error", expose: "route"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f.Expose = tt.expose
			url, applied, err := d.resolveExposure(ctx, f, "ns", testService(nil), clientset, dynClient, nil, nil)
			if err != nil {
				t.Fatalf("resolveExposure(%q): unexpected error: %v", tt.expose, err)
			}
			if applied != "" {
				t.Errorf("resolveExposure(%q): applied = %q, want empty", tt.expose, applied)
			}
			if url == "" {
				t.Errorf("resolveExposure(%q): expected a non-empty cluster-local URL", tt.expose)
			}
		})
	}
}

// stubExposer stands in for a real exposure mechanism, recording what it was
// asked to do so a test can assert on it without a cluster.
type stubExposer struct {
	host        string
	unexposeErr error
	exposed     []deployer.Exposure
	unexposed   []string
}

func (s *stubExposer) Expose(_ context.Context, _ dynamic.Interface, e deployer.Exposure) (string, error) {
	s.exposed = append(s.exposed, e)
	return s.host, nil
}

func (s *stubExposer) Unexpose(_ context.Context, _ dynamic.Interface, ref deployer.ExposureRef) error {
	s.unexposed = append(s.unexposed, ref.Namespace+"/"+ref.FunctionNamespace+"/"+ref.FunctionName)
	return s.unexposeErr
}

func testService(annotations map[string]string) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name: "f", Namespace: "ns", Annotations: annotations,
		},
	}
}

// Test_ResolveExposure_NoExposerLeavesHostnameAlone: a Deployer with no
// Exposer must leave the exposure record untouched, cluster-local intent
// included. Keda's embedded raw Deployer runs in exactly this state on every
// keda deploy, while keda's own exposing object and the record keda wrote
// for it are in place; clearing it here would blank the hostname Describe
// and List read back and lose the Route's location teardown reads.
func Test_ResolveExposure_NoExposerLeavesHostnameAlone(t *testing.T) {
	const (
		host    = "f-ns.apps.example.com"
		routeNS = "openshift-keda"
	)
	ctx := t.Context()
	svc := testService(map[string]string{
		RouteHostnameAnnotation:  host,
		RouteNamespaceAnnotation: routeNS,
	})
	clientset := fake.NewClientset(svc)
	dynClient := dynamicfakeclient.NewSimpleDynamicClient(runtime.NewScheme())

	d := NewDeployer()
	f := fn.Function{Name: "f", Expose: fn.ExposeNone, Deploy: fn.DeploySpec{Namespace: "ns"}}

	if _, applied, err := d.resolveExposure(ctx, f, "ns", svc, clientset, dynClient, nil, nil); err != nil {
		t.Fatal(err)
	} else if applied != "" {
		t.Errorf("applied = %q, want empty", applied)
	}

	svc, err := clientset.CoreV1().Services("ns").Get(ctx, "f", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if got := svc.Annotations[RouteHostnameAnnotation]; got != host {
		t.Errorf("expected hostname %q to survive, got %q", host, got)
	}
	if got := svc.Annotations[RouteNamespaceAnnotation]; got != routeNS {
		t.Errorf("expected route namespace %q to survive, got %q", routeNS, got)
	}
}

// Test_generateService_CarriesExposureRecordAcrossRedeploy pins that a
// redeploy does not erase the exposure record.
//
// Every other annotation here is regenerated from func.yaml, and the update
// replaces the Service's whole annotation map, so anything cluster-derived
// survives only by being copied off the live Service. The record is
// cluster-derived: the hostname is the one the router admitted, whether
// minted by it or given with --domain, and the exposing step chooses the
// namespace for Route. Both are known only after this write.
//
// The hostname is what Describe and List report; the location is what keda's
// delete and its unexpose toggle use to find the Route. Carrying only the
// hostname would leave a function advertising a URL whose Route has no
// recorded home, putting the delete back to guessing at a namespace.
func Test_generateService_CarriesExposureRecordAcrossRedeploy(t *testing.T) {
	const (
		host    = "f-ns.apps.example.com"
		routeNS = "openshift-keda"
	)
	d := NewDeployer()
	// A function already deployed to "ns". Each subtest regenerates its
	// Service as a redeploy would, against a different live Service.
	f := fn.Function{Name: "f", Deploy: fn.DeploySpec{Namespace: "ns"}}
	deployment := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "f", Namespace: "ns"}}

	generate := func(t *testing.T, existing *corev1.Service) *corev1.Service {
		t.Helper()
		labels, anns := testMeta(t, f)
		svc, err := d.generateService(f, "ns", labels, anns, deployment, existing)
		if err != nil {
			t.Fatal(err)
		}
		return svc
	}

	// An exposed function: the live Service records both the hostname and
	// where the Route is. A redeploy must keep both.
	t.Run("hostname and route namespace both survive", func(t *testing.T) {
		svc := generate(t, testService(map[string]string{
			RouteHostnameAnnotation:  host,
			RouteNamespaceAnnotation: routeNS,
		}))
		if got := svc.Annotations[RouteHostnameAnnotation]; got != host {
			t.Errorf("hostname = %q, want %q", got, host)
		}
		if got := svc.Annotations[RouteNamespaceAnnotation]; got != routeNS {
			t.Errorf("route namespace = %q, want %q", got, routeNS)
		}
	})

	t.Run("record does not leak onto the shared annotation map", func(t *testing.T) {
		labels, anns := testMeta(t, f)
		existing := testService(map[string]string{
			RouteHostnameAnnotation:  host,
			RouteNamespaceAnnotation: routeNS,
		})
		if _, err := d.generateService(f, "ns", labels, anns, deployment, existing); err != nil {
			t.Fatal(err)
		}
		if _, ok := anns[RouteHostnameAnnotation]; ok {
			t.Error("generateService must clone before writing the record; the Deployment's map must stay clean")
		}
	})

	// Never exposed: no record appears, and a create (no live Service at
	// all) is the same case. The copy is conditional on the key being
	// present, so an unexposed Service does not grow empty annotations.
	t.Run("nothing recorded leaves both off", func(t *testing.T) {
		for name, existing := range map[string]*corev1.Service{
			"live Service with no record": testService(nil),
			"create, no live Service":     nil,
		} {
			t.Run(name, func(t *testing.T) {
				svc := generate(t, existing)
				if got, ok := svc.Annotations[RouteHostnameAnnotation]; ok {
					t.Errorf("expected no hostname annotation, got %q", got)
				}
				if got, ok := svc.Annotations[RouteNamespaceAnnotation]; ok {
					t.Errorf("expected no route-namespace annotation, got %q", got)
				}
			})
		}
	})
}

// Test_ResolveExposure_WithExposer covers both directions of the reconcile a
// wired Exposer performs: active intent creates and records the hostname,
// cluster-local intent removes and clears it. The removal half is what makes
// toggling --expose=route off across redeploys work.
func Test_ResolveExposure_WithExposer(t *testing.T) {
	t.Run("active intent exposes and records the host", func(t *testing.T) {
		const host = "f-ns.apps.example.com"
		ctx := t.Context()
		// No Deployment staged: exposure must not depend on one existing.
		clientset := fake.NewClientset(testService(nil))
		dynClient := dynamicfakeclient.NewSimpleDynamicClient(runtime.NewScheme())

		exposer := &stubExposer{host: host}
		d := NewDeployer(WithExposer(exposer))
		f := fn.Function{Name: "f", Expose: fn.ExposeRoute, Deploy: fn.DeploySpec{Namespace: "ns"}}

		svc := testService(nil)
		svc.UID = "uid-svc"
		url, applied, err := d.resolveExposure(ctx, f, "ns", svc, clientset, dynClient, nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		if applied != fn.ExposeRoute {
			t.Errorf("applied = %q, want %q", applied, fn.ExposeRoute)
		}
		if url != "https://"+host {
			t.Errorf("url = %q, want %q", url, "https://"+host)
		}
		if len(exposer.exposed) != 1 {
			t.Fatalf("expected exactly one Expose call, got %d", len(exposer.exposed))
		}
		// The raw deployer exposes the function's own Service, which also owns
		// the Route so it never outlives its traffic target.
		e := exposer.exposed[0]
		if e.Name != "f" || e.Namespace != "ns" || e.TargetService != "f" || e.TargetPort != "http" {
			t.Errorf("unexpected Exposure target: %+v", e)
		}
		if e.Owner == nil || e.Owner.Kind != "Service" || e.Owner.Name != "f" || e.Owner.UID != "uid-svc" {
			t.Errorf("expected the Service as owner, got %+v", e.Owner)
		}

		updated, err := clientset.CoreV1().Services("ns").Get(ctx, "f", metav1.GetOptions{})
		if err != nil {
			t.Fatal(err)
		}
		if got := updated.Annotations[RouteHostnameAnnotation]; got != host {
			t.Errorf("expected hostname %q recorded on the Service, got %q", host, got)
		}
		// The record is written both-or-neither; the raw deployer's Route
		// sits in the function's own namespace.
		if got := updated.Annotations[RouteNamespaceAnnotation]; got != "ns" {
			t.Errorf("expected route namespace %q recorded on the Service, got %q", "ns", got)
		}
	})

	t.Run("cluster-local intent with a record unexposes and clears it", func(t *testing.T) {
		ctx := t.Context()
		svc := testService(map[string]string{
			RouteHostnameAnnotation:  "stale.apps.example.com",
			RouteNamespaceAnnotation: "ns",
		})
		clientset := fake.NewClientset(svc)
		dynClient := dynamicfakeclient.NewSimpleDynamicClient(runtime.NewScheme())

		exposer := &stubExposer{}
		d := NewDeployer(WithExposer(exposer))
		f := fn.Function{Name: "f", Expose: fn.ExposeNone, Deploy: fn.DeploySpec{Namespace: "ns"}}

		_, applied, err := d.resolveExposure(ctx, f, "ns", svc, clientset, dynClient, nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		if applied != "" {
			t.Errorf("applied = %q, want empty", applied)
		}
		// The raw deployer's Route sits beside its function, so the recorded
		// namespace and the function's namespace are the same.
		if len(exposer.unexposed) != 1 || exposer.unexposed[0] != "ns/ns/f" {
			t.Errorf("expected one Unexpose of ns/ns/f, got %v", exposer.unexposed)
		}

		updated, err := clientset.CoreV1().Services("ns").Get(ctx, "f", metav1.GetOptions{})
		if err != nil {
			t.Fatal(err)
		}
		if got, ok := updated.Annotations[RouteHostnameAnnotation]; ok {
			t.Errorf("expected the stale hostname cleared, still %q", got)
		}
		if got, ok := updated.Annotations[RouteNamespaceAnnotation]; ok {
			t.Errorf("expected the recorded namespace cleared, still %q", got)
		}
	})
}

// Test_ResolveExposure_RecordFailureRollsBack: the Route is created before
// the record is written, and the no-record paths never look for one. A
// recording failure must therefore take the just-created Route back down;
// otherwise a live, unrecorded exposure survives that --expose=none cannot
// remove.
func Test_ResolveExposure_RecordFailureRollsBack(t *testing.T) {
	const host = "f-ns.apps.example.com"
	newClientset := func() *fake.Clientset {
		clientset := fake.NewClientset(testService(nil))
		clientset.PrependReactor("update", "services", func(k8stesting.Action) (bool, runtime.Object, error) {
			return true, nil, fmt.Errorf("boom")
		})
		return clientset
	}
	f := fn.Function{Name: "f", Expose: fn.ExposeRoute, Deploy: fn.DeploySpec{Namespace: "ns"}}
	dynClient := dynamicfakeclient.NewSimpleDynamicClient(runtime.NewScheme())

	t.Run("rollback happens and both facts are reported", func(t *testing.T) {
		exposer := &stubExposer{host: host}
		d := NewDeployer(WithExposer(exposer))

		_, _, err := d.resolveExposure(t.Context(), f, "ns", testService(nil), newClientset(), dynClient, nil, nil)
		if err == nil {
			t.Fatal("expected the deploy to fail when the record cannot be written")
		}
		if len(exposer.unexposed) != 1 || exposer.unexposed[0] != "ns/ns/f" {
			t.Errorf("expected exactly one rollback Unexpose of ns/ns/f, got %v", exposer.unexposed)
		}
		if !strings.Contains(err.Error(), "rolled back") {
			t.Errorf("expected the error to say the Route was rolled back, got: %v", err)
		}
	})

	t.Run("a failed rollback reports both failures", func(t *testing.T) {
		exposer := &stubExposer{host: host, unexposeErr: fmt.Errorf("also boom")}
		d := NewDeployer(WithExposer(exposer))

		_, _, err := d.resolveExposure(t.Context(), f, "ns", testService(nil), newClientset(), dynClient, nil, nil)
		if err == nil {
			t.Fatal("expected the deploy to fail")
		}
		if !strings.Contains(err.Error(), "recording the exposure failed") ||
			!strings.Contains(err.Error(), "rolling the Route back failed") {
			t.Errorf("expected both failures reported, got: %v", err)
		}
	})
}

// Test_ResolveExposure_RecordOrSilence: teardown acts on the record the Service
// carries, and on nothing else.
//
// No record means this deployer created no Route, so the Route API is not
// reached at all and the intent behind the opt-out makes no difference. A
// record means a Route was made and is owed removal, so any failure to remove
// it is fatal: reporting cluster-local while an address nothing owns keeps
// serving is the outcome this refuses.
func Test_ResolveExposure_RecordOrSilence(t *testing.T) {
	// unexposeErr on a no-record case is staged to fail loudly if reached.
	notCalled := errors.New("must not be called")
	denied := fmt.Errorf("%w: forbidden", deployer.ErrExposureNotVisible)

	tests := []struct {
		name        string
		expose      string
		record      bool
		unexposeErr error
		wantErr     bool
		wantCalls   int
	}{
		{name: "no record, unset intent: silent", expose: "", unexposeErr: notCalled},
		{name: "no record, explicit none: silent", expose: fn.ExposeNone, unexposeErr: notCalled},
		{name: "record, denied removal: fatal", expose: fn.ExposeNone, record: true,
			unexposeErr: denied, wantErr: true, wantCalls: 1},
		{name: "record, any other failure: fatal", expose: fn.ExposeNone, record: true,
			unexposeErr: errors.New("boom"), wantErr: true, wantCalls: 1},
		{name: "record, removal succeeds: proceeds", expose: fn.ExposeNone, record: true, wantCalls: 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := t.Context()
			var ann map[string]string
			if tt.record {
				ann = map[string]string{
					RouteHostnameAnnotation:  "f-ns.apps.example.com",
					RouteNamespaceAnnotation: "ns",
				}
			}
			svc := testService(ann)
			clientset := fake.NewClientset(svc)
			dynClient := dynamicfakeclient.NewSimpleDynamicClient(runtime.NewScheme())

			exposer := &stubExposer{unexposeErr: tt.unexposeErr}
			d := NewDeployer(WithExposer(exposer))
			f := fn.Function{Name: "f", Expose: tt.expose, Deploy: fn.DeploySpec{Namespace: "ns"}}

			_, applied, err := d.resolveExposure(ctx, f, "ns", svc, clientset, dynClient, nil, nil)
			switch {
			case tt.wantErr && err == nil:
				t.Fatal("expected a removal failure to be fatal where a record exists")
			case !tt.wantErr && err != nil:
				t.Fatalf("expected the deploy to proceed, got %v", err)
			case !tt.wantErr && applied != "":
				t.Errorf("applied = %q, want empty", applied)
			}
			if len(exposer.unexposed) != tt.wantCalls {
				t.Errorf("Unexpose calls = %d, want %d", len(exposer.unexposed), tt.wantCalls)
			}
		})
	}
}

// Test_DeployNamespace pins the rule two packages now share. pkg/keda checks
// the name its Route would take BEFORE the embedded raw deploy runs, so it
// needs this answer early; a copy of the rule there could drift and validate a
// name this deployer would not use.
func Test_DeployNamespace(t *testing.T) {
	tests := []struct {
		name      string
		requested string
		deployed  string
		want      string
		wantErr   bool
	}{
		{
			name: "requested wins", requested: "want-this", deployed: "already-here",
			want: "want-this",
		},
		{
			// A redeploy with no --namespace stays where it is.
			name: "falls back to where it is deployed", requested: "", deployed: "already-here",
			want: "already-here",
		},
		{
			name: "requested only", requested: "want-this", deployed: "",
			want: "want-this",
		},
		{
			// Neither known: the caller has to be told, not given a guess.
			name: "neither: an error, never a default", requested: "", deployed: "",
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := fn.Function{Name: "f", Namespace: tt.requested}
			f.Deploy.Namespace = tt.deployed

			got, err := DeployNamespace(f)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected an error, got namespace %q", got)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Errorf("DeployNamespace() = %q, want %q", got, tt.want)
			}
		})
	}
}

// Test_DomainStaysOutOfSelectors guards that Deployment and Service
// selectors are identity-only. Deployment.spec.selector is immutable, so
// domain or user labels there make later changes require recreation. Those
// labels stay on object metadata and the pod template.
func Test_DomainStaysOutOfSelectors(t *testing.T) {
	d := NewDeployer()
	team := "red"
	teamKey := "team"
	f := fn.Function{
		Name:   "f",
		Domain: "f.example.test",
		Deploy: fn.DeploySpec{
			Image:  "registry.example.com/f:latest",
			Labels: []fn.Label{{Key: &teamKey, Value: &team}},
		},
	}
	rs, rcm, rpvc := sets.New[string](), sets.New[string](), sets.New[string]()
	labels, anns := testMeta(t, f)
	deployment, err := d.generateDeployment(f, "ns", labels, anns, &rs, &rcm, &rpvc)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := deployment.Spec.Selector.MatchLabels[deployer.DomainLabel]; ok {
		t.Error("expected the immutable Deployment selector to exclude the domain label")
	}
	if _, ok := deployment.Spec.Selector.MatchLabels["team"]; ok {
		t.Error("expected the immutable Deployment selector to exclude user labels")
	}
	if got := deployment.Spec.Template.Labels[deployer.DomainLabel]; got != f.Domain {
		t.Errorf("expected the pod template labels to carry the domain, got %q", got)
	}
	if got := deployment.Spec.Template.Labels["team"]; got != "red" {
		t.Errorf("expected the pod template labels to carry the user label, got %q", got)
	}
	if got := deployment.Labels[deployer.DomainLabel]; got != f.Domain {
		t.Errorf("expected the Deployment labels to carry the domain, got %q", got)
	}
	// The API server requires the selector to be satisfied by the template
	// labels; assert the subset relation holds after the filtering.
	for k, v := range deployment.Spec.Selector.MatchLabels {
		if deployment.Spec.Template.Labels[k] != v {
			t.Errorf("selector entry %s=%s not satisfied by template labels", k, v)
		}
	}

	svc, err := d.generateService(f, "ns", labels, anns, deployment, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := svc.Spec.Selector[deployer.DomainLabel]; ok {
		t.Error("expected the Service selector to exclude the domain label")
	}
	if _, ok := svc.Spec.Selector["team"]; ok {
		t.Error("expected the Service selector to exclude user labels")
	}
	if got := svc.Labels[deployer.DomainLabel]; got != f.Domain {
		t.Errorf("expected the Service labels to carry the domain, got %q", got)
	}
}

// Test_generateService_OwnerReferenceOmitsBlockOwnerDeletion guards the
// hand-built owner reference. metav1.NewControllerRef would also set
// BlockOwnerDeletion. Under OpenShift's OwnerReferencesPermissionEnforcement
// admission plugin, writing that flag requires update on the owner's
// finalizers subresource; ordinary users and the pipeline ServiceAccount
// hold no such grant, so the Service create fails outright. KinD does not
// run the plugin, so CI cannot catch a regression; this test can.
func Test_generateService_OwnerReferenceOmitsBlockOwnerDeletion(t *testing.T) {
	d := NewDeployer()
	f := fn.Function{Name: "f", Deploy: fn.DeploySpec{Image: "registry.example.com/f:latest"}}
	deployment := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "f", Namespace: "ns", UID: "uid-1"}}

	labels, anns := testMeta(t, f)
	svc, err := d.generateService(f, "ns", labels, anns, deployment, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(svc.OwnerReferences) != 1 {
		t.Fatalf("expected exactly 1 owner reference, got %d", len(svc.OwnerReferences))
	}
	ref := svc.OwnerReferences[0]
	if ref.Kind != "Deployment" || ref.Name != "f" || ref.UID != "uid-1" {
		t.Errorf("expected the Deployment as owner, got %+v", ref)
	}
	if ref.Controller == nil || !*ref.Controller {
		t.Error("expected Controller set on the owner reference")
	}
	if ref.BlockOwnerDeletion != nil {
		t.Error("expected BlockOwnerDeletion left unset: setting it makes OpenShift's " +
			"OwnerReferencesPermissionEnforcement reject the Service create")
	}
}

// Older funcs pinned the whole label map in the immutable selector; keep the
// live selector on update and refuse when a pinned label changes.
func Test_preserveDeploymentSelector(t *testing.T) {
	stable := map[string]string{
		"boson.dev/function":        "true",
		"function.knative.dev/name": "f",
	}
	withDomain := func(domain string) map[string]string {
		m := map[string]string{}
		maps.Copy(m, stable)
		if domain != "" {
			m[deployer.DomainLabel] = domain
		}
		return m
	}
	newDesired := func(templateLabels map[string]string) *appsv1.Deployment {
		return &appsv1.Deployment{Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{MatchLabels: stable},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: templateLabels},
			},
		}}
	}
	legacy := &appsv1.Deployment{Spec: appsv1.DeploymentSpec{
		Selector: &metav1.LabelSelector{MatchLabels: withDomain("old.example.com")},
	}}

	t.Run("unchanged domain keeps the live selector", func(t *testing.T) {
		desired := newDesired(withDomain("old.example.com"))
		if err := preserveDeploymentSelector(legacy, desired, "f"); err != nil {
			t.Fatalf("unexpected refusal: %v", err)
		}
		if got := desired.Spec.Selector.MatchLabels[deployer.DomainLabel]; got != "old.example.com" {
			t.Errorf("expected the live selector preserved, pinned domain = %q", got)
		}
	})

	t.Run("changed domain refuses with recreation instructions", func(t *testing.T) {
		desired := newDesired(withDomain("new.example.com"))
		err := preserveDeploymentSelector(legacy, desired, "f")
		if err == nil {
			t.Fatal("expected a refusal for a changed pinned domain")
		}
		if !strings.Contains(err.Error(), "func delete") {
			t.Errorf("expected the refusal to point at recreation, got: %v", err)
		}
	})

	t.Run("removed domain refuses too", func(t *testing.T) {
		if err := preserveDeploymentSelector(legacy, newDesired(withDomain("")), "f"); err == nil {
			t.Fatal("expected a refusal for a removed pinned domain")
		}
	})

	t.Run("pinned user label refuses without domain wording", func(t *testing.T) {
		pinned := withDomain("")
		pinned["team"] = "a"
		legacyUser := &appsv1.Deployment{Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{MatchLabels: pinned},
		}}
		err := preserveDeploymentSelector(legacyUser, newDesired(stable), "f")
		if err == nil {
			t.Fatal("expected a refusal for a dropped pinned user label")
		}
		if !strings.Contains(err.Error(), "team") {
			t.Errorf("expected the pinned key named, got: %v", err)
		}
		if strings.Contains(err.Error(), "domain") {
			t.Errorf("expected no domain-specific advice for a user label, got: %v", err)
		}
	})

	t.Run("current-era selector accepts a new domain without pinning it", func(t *testing.T) {
		current := &appsv1.Deployment{Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{MatchLabels: stable},
		}}
		desired := newDesired(withDomain("new.example.com"))
		if err := preserveDeploymentSelector(current, desired, "f"); err != nil {
			t.Fatalf("unexpected refusal: %v", err)
		}
		if _, pinned := desired.Spec.Selector.MatchLabels[deployer.DomainLabel]; pinned {
			t.Error("expected the domain to stay out of the selector")
		}
	})
}
