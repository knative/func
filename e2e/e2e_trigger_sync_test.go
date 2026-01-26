//go:build e2e
// +build e2e

package e2e

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	fn "knative.dev/func/pkg/functions"
)

// TestTriggerSync_StaleTriggerCleanup verifies that stale triggers are deleted
// when subscriptions are removed from func.yaml
func TestTriggerSync_StaleTriggerCleanup(t *testing.T) {
	brokerName := "default"
	createBrokerWithCheck(t, Namespace, brokerName)

	functionName := "func-e2e-test-trigger-sync-cleanup"
	root := fromCleanEnv(t, functionName)

	// Create function
	if err := newCmd(t, "init", "-l=go", "-t=http").Run(); err != nil {
		t.Fatal(err)
	}

	// Add first subscription
	if err := newCmd(t, "subscribe",
		"--source", "default",
		"--filter", "type=order.created").Run(); err != nil {
		t.Fatal(err)
	}

	// Add second subscription by editing func.yaml
	f, err := fn.NewFunction(root)
	if err != nil {
		t.Fatal(err)
	}
	// Add another subscription manually
	f.Deploy.Subscriptions = append(f.Deploy.Subscriptions, fn.KnativeSubscription{
		Source: "default",
		Filters: map[string]string{
			"type": "order.updated",
		},
	})
	if err := f.Write(); err != nil {
		t.Fatal(err)
	}

	// Deploy with raw deployer
	if err := newCmd(t, "deploy", "--deployer", "raw").Run(); err != nil {
		t.Fatal(err)
	}
	defer clean(t, functionName, Namespace)

	waitForDeployment(t, Namespace, functionName)

	// Verify 2 triggers were created
	triggers := listTriggersForFunction(t, Namespace, functionName)
	if len(triggers) != 2 {
		t.Fatalf("Expected 2 triggers after initial deploy, got %d: %v", len(triggers), triggers)
	}
	t.Logf("Initial deploy created 2 triggers: %v", triggers)

	// Verify triggers have managed-by annotation
	for _, trigger := range triggers {
		if !hasManagedByAnnotation(t, Namespace, trigger) {
			t.Errorf("Trigger %s missing managed-by annotation", trigger)
		}
	}
	t.Log("All triggers have managed-by annotation")

	// Remove one subscription by editing func.yaml
	f, err = fn.NewFunction(root)
	if err != nil {
		t.Fatal(err)
	}
	// Keep only the first subscription
	f.Deploy.Subscriptions = f.Deploy.Subscriptions[:1]
	if err := f.Write(); err != nil {
		t.Fatal(err)
	}

	// Redeploy
	if err := newCmd(t, "deploy", "--deployer", "raw").Run(); err != nil {
		t.Fatal(err)
	}

	time.Sleep(5 * time.Second)

	// Verify only 1 trigger remains
	triggersAfter := listTriggersForFunction(t, Namespace, functionName)
	if len(triggersAfter) != 1 {
		t.Fatalf("Expected 1 trigger after removing subscription, got %d: %v", len(triggersAfter), triggersAfter)
	}
	t.Logf("Stale trigger deleted, 1 trigger remains: %v", triggersAfter)
}

// TestTriggerSync_ManualTriggerPreservation verifies that manually created
// triggers (without managed-by annotation) are NOT deleted during sync
func TestTriggerSync_ManualTriggerPreservation(t *testing.T) {
	brokerName := "default"
	createBrokerWithCheck(t, Namespace, brokerName)

	functionName := "func-e2e-test-trigger-sync-manual"
	_ = fromCleanEnv(t, functionName)

	// Create function
	if err := newCmd(t, "init", "-l=go", "-t=http").Run(); err != nil {
		t.Fatal(err)
	}

	// Add one subscription
	if err := newCmd(t, "subscribe",
		"--source", "default",
		"--filter", "type=order.created").Run(); err != nil {
		t.Fatal(err)
	}

	// Deploy with raw deployer
	if err := newCmd(t, "deploy", "--deployer", "raw").Run(); err != nil {
		t.Fatal(err)
	}
	defer clean(t, functionName, Namespace)

	waitForDeployment(t, Namespace, functionName)

	// Create a manual trigger (without managed-by annotation)
	manualTriggerName := fmt.Sprintf("%s-manual-trigger", functionName)
	createManualTrigger(t, Namespace, manualTriggerName, functionName, brokerName)

	// Verify we have 2 triggers (1 managed + 1 manual)
	allTriggers := listAllTriggers(t, Namespace)
	managedTriggers := listTriggersForFunction(t, Namespace, functionName)
	if len(managedTriggers) != 1 {
		t.Fatalf("Expected 1 managed trigger, got %d", len(managedTriggers))
	}
	if len(allTriggers) < 2 {
		t.Fatalf("Expected at least 2 total triggers (managed + manual), got %d", len(allTriggers))
	}
	t.Logf("Created manual trigger: %s", manualTriggerName)

	// Redeploy (no changes to subscriptions)
	if err := newCmd(t, "deploy", "--deployer", "raw").Run(); err != nil {
		t.Fatal(err)
	}

	time.Sleep(5 * time.Second)

	// Verify manual trigger still exists
	if !triggerExists(t, Namespace, manualTriggerName) {
		t.Fatal("Manual trigger was deleted during sync - should have been preserved!")
	}
	t.Log("Manual trigger preserved after redeploy")

	// Verify managed trigger still exists
	managedTriggersAfter := listTriggersForFunction(t, Namespace, functionName)
	if len(managedTriggersAfter) != 1 {
		t.Fatalf("Expected 1 managed trigger after redeploy, got %d", len(managedTriggersAfter))
	}
	t.Log("Managed trigger still exists")
}

// TestTriggerSync_AddSubscription verifies that new triggers are created
// when subscriptions are added
func TestTriggerSync_AddSubscription(t *testing.T) {
	brokerName := "default"
	createBrokerWithCheck(t, Namespace, brokerName)

	functionName := "func-e2e-test-trigger-sync-add"
	root := fromCleanEnv(t, functionName)

	// Create function
	if err := newCmd(t, "init", "-l=go", "-t=http").Run(); err != nil {
		t.Fatal(err)
	}

	// Add one subscription
	if err := newCmd(t, "subscribe",
		"--source", "default",
		"--filter", "type=order.created").Run(); err != nil {
		t.Fatal(err)
	}

	// Deploy with raw deployer
	if err := newCmd(t, "deploy", "--deployer", "raw").Run(); err != nil {
		t.Fatal(err)
	}
	defer clean(t, functionName, Namespace)

	waitForDeployment(t, Namespace, functionName)

	// Verify 1 trigger created
	triggers := listTriggersForFunction(t, Namespace, functionName)
	if len(triggers) != 1 {
		t.Fatalf("Expected 1 trigger, got %d", len(triggers))
	}
	t.Logf("Initial deploy created 1 trigger: %v", triggers)

	// Add another subscription by editing func.yaml
	f, err := fn.NewFunction(root)
	if err != nil {
		t.Fatal(err)
	}
	f.Deploy.Subscriptions = append(f.Deploy.Subscriptions, fn.KnativeSubscription{
		Source: "default",
		Filters: map[string]string{
			"type": "order.shipped",
		},
	})
	if err := f.Write(); err != nil {
		t.Fatal(err)
	}

	// Redeploy
	if err := newCmd(t, "deploy", "--deployer", "raw").Run(); err != nil {
		t.Fatal(err)
	}

	time.Sleep(5 * time.Second)

	// Verify 2 triggers now exist
	triggersAfter := listTriggersForFunction(t, Namespace, functionName)
	if len(triggersAfter) != 2 {
		t.Fatalf("Expected 2 triggers after adding subscription, got %d: %v", len(triggersAfter), triggersAfter)
	}
	t.Logf("New trigger created, 2 triggers total: %v", triggersAfter)
}

// TestTriggerSync_Idempotency verifies that repeated deploys with the same
// subscriptions don't create duplicate triggers
func TestTriggerSync_Idempotency(t *testing.T) {
	brokerName := "default"
	createBrokerWithCheck(t, Namespace, brokerName)

	functionName := "func-e2e-test-trigger-sync-idempotent"
	root := fromCleanEnv(t, functionName)

	// Create function
	if err := newCmd(t, "init", "-l=go", "-t=http").Run(); err != nil {
		t.Fatal(err)
	}

	// Add first subscription
	if err := newCmd(t, "subscribe",
		"--source", "default",
		"--filter", "type=order.created").Run(); err != nil {
		t.Fatal(err)
	}

	// Add second subscription by editing func.yaml
	f, err := fn.NewFunction(root)
	if err != nil {
		t.Fatal(err)
	}
	f.Deploy.Subscriptions = append(f.Deploy.Subscriptions, fn.KnativeSubscription{
		Source: "default",
		Filters: map[string]string{
			"type": "order.updated",
		},
	})
	if err := f.Write(); err != nil {
		t.Fatal(err)
	}

	// Deploy with raw deployer
	if err := newCmd(t, "deploy", "--deployer", "raw").Run(); err != nil {
		t.Fatal(err)
	}
	defer clean(t, functionName, Namespace)

	waitForDeployment(t, Namespace, functionName)

	// Verify 2 triggers created
	triggers := listTriggersForFunction(t, Namespace, functionName)
	if len(triggers) != 2 {
		t.Fatalf("Expected 2 triggers, got %d", len(triggers))
	}
	initialTriggers := make([]string, len(triggers))
	copy(initialTriggers, triggers)
	t.Logf("Initial triggers: %v", initialTriggers)

	// Redeploy multiple times
	for i := 1; i <= 3; i++ {
		t.Logf("Redeploy #%d", i)
		if err := newCmd(t, "deploy", "--deployer", "raw").Run(); err != nil {
			t.Fatal(err)
		}
		time.Sleep(3 * time.Second)

		triggersAfter := listTriggersForFunction(t, Namespace, functionName)
		if len(triggersAfter) != 2 {
			t.Fatalf("Redeploy #%d: Expected 2 triggers, got %d: %v", i, len(triggersAfter), triggersAfter)
		}

		// Verify trigger names haven't changed
		if !equalStringSlices(initialTriggers, triggersAfter) {
			t.Fatalf("Redeploy #%d: Trigger names changed! Initial: %v, After: %v", i, initialTriggers, triggersAfter)
		}
	}
	t.Log("Idempotency verified: 3 redeploys produced same triggers")
}

// TestTriggerSync_RemoveAllSubscriptions verifies that all managed triggers
// are deleted when all subscriptions are removed
func TestTriggerSync_RemoveAllSubscriptions(t *testing.T) {
	brokerName := "default"
	createBrokerWithCheck(t, Namespace, brokerName)

	functionName := "func-e2e-test-trigger-sync-remove-all"
	root := fromCleanEnv(t, functionName)

	// Create function
	if err := newCmd(t, "init", "-l=go", "-t=http").Run(); err != nil {
		t.Fatal(err)
	}

	// Add first subscription
	if err := newCmd(t, "subscribe",
		"--source", "default",
		"--filter", "type=order.created").Run(); err != nil {
		t.Fatal(err)
	}

	// Add second subscription by editing func.yaml
	f, err := fn.NewFunction(root)
	if err != nil {
		t.Fatal(err)
	}
	f.Deploy.Subscriptions = append(f.Deploy.Subscriptions, fn.KnativeSubscription{
		Source: "default",
		Filters: map[string]string{
			"type": "order.updated",
		},
	})
	if err := f.Write(); err != nil {
		t.Fatal(err)
	}

	// Deploy with raw deployer
	if err := newCmd(t, "deploy", "--deployer", "raw").Run(); err != nil {
		t.Fatal(err)
	}
	defer clean(t, functionName, Namespace)

	waitForDeployment(t, Namespace, functionName)

	// Verify 2 triggers created
	triggers := listTriggersForFunction(t, Namespace, functionName)
	if len(triggers) != 2 {
		t.Fatalf("Expected 2 triggers, got %d", len(triggers))
	}
	t.Logf("Initial triggers: %v", triggers)

	// Create a manual trigger for comparison
	manualTriggerName := fmt.Sprintf("%s-manual", functionName)
	createManualTrigger(t, Namespace, manualTriggerName, functionName, brokerName)

	// Remove all subscriptions by editing func.yaml directly
	f, err = fn.NewFunction(root)
	if err != nil {
		t.Fatal(err)
	}
	f.Deploy.Subscriptions = []fn.KnativeSubscription{}
	if err := f.Write(); err != nil {
		t.Fatal(err)
	}

	// Redeploy
	if err := newCmd(t, "deploy", "--deployer", "raw").Run(); err != nil {
		t.Fatal(err)
	}

	time.Sleep(5 * time.Second)

	// Verify no managed triggers remain
	managedTriggersAfter := listTriggersForFunction(t, Namespace, functionName)
	if len(managedTriggersAfter) != 0 {
		t.Fatalf("Expected 0 managed triggers after removing all subscriptions, got %d: %v", len(managedTriggersAfter), managedTriggersAfter)
	}
	t.Log("All managed triggers deleted")

	// Verify manual trigger still exists
	if !triggerExists(t, Namespace, manualTriggerName) {
		t.Fatal("Manual trigger was deleted - should have been preserved!")
	}
	t.Log("Manual trigger preserved")
}

// Helper functions

// listTriggersForFunction lists all triggers with managed-by annotation for a function
func listTriggersForFunction(t *testing.T, namespace, functionName string) []string {
	t.Helper()

	// Get all triggers in namespace (kubectl doesn't support annotation selectors)
	cmd := exec.Command("kubectl", "get", "triggers", "-n", namespace,
		"-o", "json")
	cmd.Env = append(os.Environ(), "KUBECONFIG="+Kubeconfig)

	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Logf("Warning: could not list triggers: %v, output: %s", err, string(output))
		return []string{}
	}

	// Parse JSON to filter by annotation
	var triggerList struct {
		Items []struct {
			Metadata struct {
				Name        string            `json:"name"`
				Annotations map[string]string `json:"annotations"`
			} `json:"metadata"`
		} `json:"items"`
	}

	if err := json.Unmarshal(output, &triggerList); err != nil {
		t.Logf("Warning: could not parse triggers JSON: %v", err)
		return []string{}
	}

	// Filter triggers that:
	// 1. Have the managed-by annotation
	// 2. Belong to this function (name starts with functionName-trigger-)
	var functionTriggers []string
	for _, trigger := range triggerList.Items {
		if trigger.Metadata.Annotations["func.knative.dev/managed-by"] == "func-raw-deployer" {
			if strings.HasPrefix(trigger.Metadata.Name, functionName+"-trigger-") {
				functionTriggers = append(functionTriggers, trigger.Metadata.Name)
			}
		}
	}

	return functionTriggers
}

// listAllTriggers lists all triggers in namespace
func listAllTriggers(t *testing.T, namespace string) []string {
	t.Helper()

	cmd := exec.Command("kubectl", "get", "triggers", "-n", namespace,
		"-o", "jsonpath={.items[*].metadata.name}")
	cmd.Env = append(os.Environ(), "KUBECONFIG="+Kubeconfig)

	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Logf("Warning: could not list all triggers: %v", err)
		return []string{}
	}

	triggersStr := strings.TrimSpace(string(output))
	if triggersStr == "" {
		return []string{}
	}

	return strings.Fields(triggersStr)
}

// hasManagedByAnnotation checks if a trigger has the managed-by annotation
func hasManagedByAnnotation(t *testing.T, namespace, triggerName string) bool {
	t.Helper()

	cmd := exec.Command("kubectl", "get", "trigger", triggerName, "-n", namespace,
		"-o", "jsonpath={.metadata.annotations.func\\.knative\\.dev/managed-by}")
	cmd.Env = append(os.Environ(), "KUBECONFIG="+Kubeconfig)

	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Logf("Warning: could not get trigger annotation: %v", err)
		return false
	}

	return strings.TrimSpace(string(output)) == "func-raw-deployer"
}

// createManualTrigger creates a trigger without the managed-by annotation
func createManualTrigger(t *testing.T, namespace, triggerName, functionName, brokerName string) {
	t.Helper()

	triggerYAML := fmt.Sprintf(`apiVersion: eventing.knative.dev/v1
kind: Trigger
metadata:
  name: %s
  namespace: %s
  # Note: NO managed-by annotation
spec:
  broker: %s
  subscriber:
    uri: http://%s.%s.svc.cluster.local
  filter:
    attributes:
      type: manual.event
`, triggerName, namespace, brokerName, functionName, namespace)

	cmd := exec.Command("kubectl", "apply", "-f", "-")
	cmd.Stdin = strings.NewReader(triggerYAML)
	cmd.Env = append(os.Environ(), "KUBECONFIG="+Kubeconfig)

	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Failed to create manual trigger: %v, output: %s", err, string(output))
	}

	t.Cleanup(func() {
		deleteCmd := exec.Command("kubectl", "delete", "trigger", triggerName, "-n", namespace, "--ignore-not-found")
		deleteCmd.Env = append(os.Environ(), "KUBECONFIG="+Kubeconfig)
		_ = deleteCmd.Run()
	})

	t.Logf("Created manual trigger: %s", triggerName)
}

// triggerExists checks if a trigger exists
func triggerExists(t *testing.T, namespace, triggerName string) bool {
	t.Helper()

	cmd := exec.Command("kubectl", "get", "trigger", triggerName, "-n", namespace)
	cmd.Env = append(os.Environ(), "KUBECONFIG="+Kubeconfig)

	return cmd.Run() == nil
}

// equalStringSlices checks if two string slices contain the same elements (order-independent)
func equalStringSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}

	aMap := make(map[string]bool)
	for _, s := range a {
		aMap[s] = true
	}

	for _, s := range b {
		if !aMap[s] {
			return false
		}
	}

	return true
}
