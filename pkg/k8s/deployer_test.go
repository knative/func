package k8s

import (
	"os"
	"testing"

	corev1 "k8s.io/api/core/v1"
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
