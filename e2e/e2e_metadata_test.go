//go:build e2e
// +build e2e

package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	fn "knative.dev/func/pkg/functions"
	"knative.dev/func/pkg/k8s"
)

// ---------------------------------------------------------------------------
// METADATA TESTS
// Environment Variables, Labels, Volumes, and Subscriptions
// ---------------------------------------------------------------------------

// TestMetadata_Envs_Add ensures that environment variables configured to be
// passed to the Function are available at runtime.
// - Static Value
// - Local Environment Variable
// - Config Map (single key)
// - Config Map (all keys)
// - Secret (single key)
// - Secret (all keys)
//
//	func config envs add --name={name} --value={value}
func TestMetadata_Envs_Add(t *testing.T) {
	name := "func-e2e-test-metadata-envs-add"
	root := fromCleanEnv(t, name)

	// Create the test Function
	if err := newCmd(t, "init", "-l=go").Run(); err != nil {
		t.Fatal(err)
	}

	// Set Env: fixed value passed as an argument
	if err := newCmd(t, "config", "envs", "add",
		"--name=A", "--value=a").Run(); err != nil {
		t.Fatal(err)
	}

	// Set Env: from local ENV "B"
	os.Setenv("B", "b") // From a local ENV
	if err := newCmd(t, "config", "envs", "add",
		"--name=B", "--value={{env:B}}").Run(); err != nil {
		t.Fatal(err)
	}

	// Set Env: from cluster secret (single)
	setSecret(t, "test-secret-single", Namespace, map[string][]byte{
		"C": []byte("c"),
	})
	if err := newCmd(t, "config", "envs", "add",
		"--name=C", "--value={{secret:test-secret-single:C}}").Run(); err != nil {
		t.Fatal(err)
	}

	// Set Env: from all the keys in a secret (multi)
	setSecret(t, "test-secret-multi", Namespace, map[string][]byte{
		"D": []byte("d"),
		"E": []byte("e"),
	})
	if err := newCmd(t, "config", "envs", "add",
		"--value={{secret:test-secret-multi}}").Run(); err != nil {
		t.Fatal(err)
	}

	// Set Env: from cluster config map (single)
	setConfigMap(t, "test-config-map-single", Namespace, map[string]string{
		"F": "f",
	})
	if err := newCmd(t, "config", "envs", "add",
		"--name=F", "--value={{configMap:test-config-map-single:F}}").Run(); err != nil {
		t.Fatal(err)
	}

	// Set Env: from all keys in a configMap (multi)
	setConfigMap(t, "test-config-map-multi", Namespace, map[string]string{
		"G": "g",
		"H": "h",
	})
	if err := newCmd(t, "config", "envs", "add",
		"--value={{configMap:test-config-map-multi}}").Run(); err != nil {
		t.Fatal(err)
	}

	// The test function will respond HTTP 500 unless all defined environment
	// variables exist and are populated.
	impl := `
	package function
	import (
		"fmt"
		"net/http"
		"os"
	    "strings"
	)
	func Handle(w http.ResponseWriter, _ *http.Request) {
		for c := 'A'; c <= 'H'; c++ {
			envVar := string(c)
			value, exists := os.LookupEnv(envVar)
			if exists && strings.ToLower(envVar) == value {
				continue
			} else if exists {
				msg := fmt.Sprintf("Environment variable %s exists but does not have the expected value: %s\n", envVar, value)
				http.Error(w, msg, http.StatusInternalServerError)
	            return
			} else {
				msg := fmt.Sprintf("Environment variable %s does not exist\n", envVar)
				http.Error(w, msg, http.StatusInternalServerError)
	            return
			}
		}
		fmt.Fprintln(w, "OK")
	}
	`
	err := os.WriteFile(filepath.Join(root, "handle.go"), []byte(impl), 0644)
	if err != nil {
		t.Fatal(err)
	}
	if err := newCmd(t, "deploy").Run(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		clean(t, name, Namespace)
	}()
	if !waitFor(t, fmt.Sprintf("http://%v.%s.%s", name, Namespace, Domain),
		withContentMatch("OK")) {
		t.Fatal("handler failed")
	}
}

// TestMetadata_Envs_Remove ensures that environment variables can be removed.
//
//	func config envs remove --name={name}
func TestMetadata_Envs_Remove(t *testing.T) {
	name := "func-e2e-test-metadata-envs-remove"
	root := fromCleanEnv(t, name)

	// Create the test Function
	if err := newCmd(t, "init", "-l=go").Run(); err != nil {
		t.Fatal(err)
	}

	// Set Env: two fixed values passed as an argument
	if err := newCmd(t, "config", "envs", "add",
		"--name=A", "--value=a").Run(); err != nil {
		t.Fatal(err)
	}
	if err := newCmd(t, "config", "envs", "add",
		"--name=B", "--value=b").Run(); err != nil {
		t.Fatal(err)
	}

	// Test that the function received both A and B
	impl := `
	package function
	import (
		"fmt"
		"net/http"
		"os"
	)
	func Handle(w http.ResponseWriter, _ *http.Request) {
		if os.Getenv("A") != "a" {
			http.Error(w, "A not set", http.StatusInternalServerError)
			return
		}
		if os.Getenv("B") != "b" {
			http.Error(w, "A not set", http.StatusInternalServerError)
			return
		}
		fmt.Fprintln(w, "OK")
	}
	`
	if err := os.WriteFile(filepath.Join(root, "handle.go"), []byte(impl), 0644); err != nil {
		t.Fatal(err)
	}
	if err := newCmd(t, "deploy").Run(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		clean(t, name, Namespace)
	}()
	if !waitFor(t, fmt.Sprintf("http://%v.%s.%s", name, Namespace, Domain),
		withContentMatch("OK")) {
		t.Fatal("handler failed")
	}

	// Remove B
	if err := newCmd(t, "config", "envs", "remove", "--name=B").Run(); err != nil {
		t.Fatal(err)
	}

	// Test that the function now only receives A
	impl = `
	package function
	import (
		"fmt"
		"net/http"
		"os"
	)
	func Handle(w http.ResponseWriter, _ *http.Request) {
		if os.Getenv("A") != "a" {
			http.Error(w, "A not set", http.StatusInternalServerError)
			return
		}
		if _, exists := os.LookupEnv("B"); exists {
			http.Error(w, "B still exists after remove", http.StatusInternalServerError)
			return
		}
		fmt.Fprintln(w, "OK")
	}
	`
	if err := os.WriteFile(filepath.Join(root, "handle.go"), []byte(impl), 0644); err != nil {
		t.Fatal(err)
	}
	if err := newCmd(t, "deploy").Run(); err != nil {
		t.Fatal(err)
	}
	if !waitFor(t, fmt.Sprintf("http://%v.%s.%s", name, Namespace, Domain),
		withContentMatch("OK")) {
		t.Fatal("handler failed")
	}
}

// TestMetadata_Labels_Add ensures that labels added via the CLI are
// carried through to the final service
//
// func config labels add
func TestMetadata_Labels_Add(t *testing.T) {
	name := "func-e2e-test-metadata-labels-add"
	_ = fromCleanEnv(t, name)

	if err := newCmd(t, "init", "-l=go").Run(); err != nil {
		t.Fatal(err)
	}

	// Add a label with a simple value
	// func config labels add --name=foo --value=bar
	if err := newCmd(t, "config", "labels", "add", "--name=foo", "--value=bar").Run(); err != nil {
		t.Fatal(err)
	}

	// Add a label which pulls its value from an environment variable
	// func config labels add --name=foo --value={{env:TESTLABEL}}
	os.Setenv("TESTLABEL", "testvalue")
	if err := newCmd(t, "config", "labels", "add", "--name=envlabel", "--value={{ env:TESTLABEL }}").Run(); err != nil {
		t.Fatal(err)
	}

	// Deploy the function
	if err := newCmd(t, "deploy").Run(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		clean(t, name, Namespace)
	}()
	if !waitFor(t, fmt.Sprintf("http://%v.%s.%s", name, Namespace, Domain)) {
		t.Fatal("function did not deploy correctly")
	}

	// structured output of description should have expected labels
	cmd := newCmd(t, "describe", name, "--output=json", "--namespace", Namespace)
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}

	var instance fn.Instance
	if err := json.Unmarshal(out.Bytes(), &instance); err != nil {
		t.Fatalf("error unmarshaling describe output: %v", err)
	}
	if instance.Labels == nil {
		t.Fatal("No labels returned")
	}
	if instance.Labels["foo"] != "bar" {
		t.Errorf("Label 'foo' not found or has wrong value. Got: %v", instance.Labels["foo"])
	}
	if instance.Labels["envlabel"] != "testvalue" {
		t.Errorf("Label 'envlabel' not found or has wrong value. Got: %v", instance.Labels["envlabel"])
	}
}

// TestMetadata_Labels_Remove ensures that labels can be removed.
//
// func config labels remove
func TestMetadata_Labels_Remove(t *testing.T) {
	name := "func-e2e-test-metadata-labels-remove"
	_ = fromCleanEnv(t, name)

	// Create the test Function with a couple simple labels
	if err := newCmd(t, "init", "-l=go").Run(); err != nil {
		t.Fatal(err)
	}
	if err := newCmd(t, "config", "labels", "add", "--name=foo", "--value=bar").Run(); err != nil {
		t.Fatal(err)
	}
	if err := newCmd(t, "config", "labels", "add", "--name=foo2", "--value=bar2").Run(); err != nil {
		t.Fatal(err)
	}
	if err := newCmd(t, "deploy").Run(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		clean(t, name, Namespace)
	}()
	if !waitFor(t, fmt.Sprintf("http://%v.%s.%s", name, Namespace, Domain)) {
		t.Fatal("function did not deploy correctly")
	}

	// Verify the labels were applied
	cmd := newCmd(t, "describe", name, "--output=json", "--namespace", Namespace)
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}
	var desc fn.Instance
	if err := json.Unmarshal(out.Bytes(), &desc); err != nil {
		t.Fatalf("error unmarshaling describe output: %v", err)
	}
	if desc.Labels == nil {
		t.Fatal("No labels returned")
	}
	if desc.Labels["foo"] != "bar" {
		t.Errorf("Label 'foo' not found or has wrong value. Got: %v", desc.Labels["foo"])
	}
	if desc.Labels["foo2"] != "bar2" {
		t.Errorf("Label 'foo2' not found or has wrong value. Got: %v", desc.Labels["foo2"])
	}

	// Remove one label and redeploy
	if err := newCmd(t, "config", "labels", "remove", "--name=foo2").Run(); err != nil {
		t.Fatal(err)
	}
	if err := newCmd(t, "deploy").Run(); err != nil {
		t.Fatal(err)
	}
	if !waitFor(t, fmt.Sprintf("http://%v.%s.%s", name, Namespace, Domain)) {
		t.Fatal("function did not redeploy correctly")
	}

	// Verify the function no longer includes the removed label.
	cmd = newCmd(t, "describe", "--output=json")
	out = bytes.Buffer{}
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}

	var desc2 fn.Instance
	if err := json.Unmarshal(out.Bytes(), &desc2); err != nil {
		t.Fatalf("error unmarshaling describe output: %v", err)
	}
	if _, ok := desc2.Labels["foo"]; !ok {
		t.Error("Label 'foo' should still exist")
	}
	if _, ok := desc2.Labels["foo2"]; ok {
		t.Error("Label 'foo' was not removed")
	}
}

// TestMetadata_Volumes ensures that adding volumes of various types are
// made available to the running function, and can be removed.
//
// func config volumes add
// func config volumes remove
func TestMetadata_Volumes(t *testing.T) {
	name := "func-e2e-test-metadata-volumes"
	root := fromCleanEnv(t, name)

	// Create the test Function
	if err := newCmd(t, "init", "-l=go").Run(); err != nil {
		t.Fatal(err)
	}

	// Cluster Test Configuration
	// --------------------------
	// Create test resources that will be mounted as volumes

	// Create a ConfigMap with test data
	configMapName := fmt.Sprintf("%s-configmap", name)
	setConfigMap(t, configMapName, Namespace, map[string]string{
		"config.txt": "configmap-data",
	})

	// Create a Secret with test data
	secretName := fmt.Sprintf("%s-secret", name)
	setSecret(t, secretName, Namespace, map[string][]byte{
		"secret.txt": []byte("secret-data"),
	})

	// Add volumes using the new CLI commands
	// Add ConfigMap volume
	if err := newCmd(t, "config", "volumes", "add",
		"--type=configmap",
		"--source="+configMapName,
		"--mount-path=/etc/config").Run(); err != nil {
		t.Fatal(err)
	}

	// Add Secret volume
	if err := newCmd(t, "config", "volumes", "add",
		"--type=secret",
		"--source="+secretName,
		"--mount-path=/etc/secret").Run(); err != nil {
		t.Fatal(err)
	}

	// Add EmptyDir volume (for testing write capabilities)
	if err := newCmd(t, "config", "volumes", "add",
		"--type=emptydir",
		"--mount-path=/tmp/scratch").Run(); err != nil {
		t.Fatal(err)
	}

	// Create a Function implementation which validates the volumes.
	impl := `package function

import (
	"fmt"
	"net/http"
	"os"
	"strings"
)

func Handle(w http.ResponseWriter, _ *http.Request) {
	errors := []string{}

	// Check ConfigMap volume
	configData, err := os.ReadFile("/etc/config/config.txt")
	if err != nil {
		errors = append(errors, fmt.Sprintf("ConfigMap read error: %v", err))
	} else if string(configData) != "configmap-data" {
		errors = append(errors, fmt.Sprintf("ConfigMap data mismatch: got %q", string(configData)))
	}

	// Check Secret volume
	secretData, err := os.ReadFile("/etc/secret/secret.txt")
	if err != nil {
		errors = append(errors, fmt.Sprintf("Secret read error: %v", err))
	} else if string(secretData) != "secret-data" {
		errors = append(errors, fmt.Sprintf("Secret data mismatch: got %q", string(secretData)))
	}

	// Check EmptyDir volume (test write capability)
	testFile := "/tmp/scratch/test.txt"
	testData := "emptydir-test"
	if err := os.WriteFile(testFile, []byte(testData), 0644); err != nil {
		errors = append(errors, fmt.Sprintf("EmptyDir write error: %v", err))
	} else {
		// Read it back to verify
		readData, err := os.ReadFile(testFile)
		if err != nil {
			errors = append(errors, fmt.Sprintf("EmptyDir read error: %v", err))
		} else if string(readData) != testData {
			errors = append(errors, fmt.Sprintf("EmptyDir data mismatch: got %q", string(readData)))
		}
	}

	if len(errors) > 0 {
		http.Error(w, strings.Join(errors, "\n"), http.StatusInternalServerError)
		return
	}
	fmt.Fprintln(w, "OK")
}

`
	err := os.WriteFile(filepath.Join(root, "handle.go"), []byte(impl), 0644)
	if err != nil {
		t.Fatal(err)
	}

	// Deploy the function
	if err := newCmd(t, "deploy").Run(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		clean(t, name, Namespace)
	}()

	// Verify the function has access to all volumes
	if !waitFor(t, fmt.Sprintf("http://%s.%s.%s", name, Namespace, Domain),
		withContentMatch("OK")) {
		t.Fatal("function failed to access volumes correctly")
	}

	// Test volume removal
	// Remove the ConfigMap volume
	if err := newCmd(t, "config", "volumes", "remove",
		"--mount-path=/etc/config").Run(); err != nil {
		t.Fatal(err)
	}

	// Update implementation to verify ConfigMap is no longer accessible
	impl = `package function

import (
	"fmt"
	"net/http"
	"os"
	"strings"
)

func Handle(w http.ResponseWriter, _ *http.Request) {
	errors := []string{}

	// Check ConfigMap volume should NOT exist
	if _, err := os.Stat("/etc/config"); !os.IsNotExist(err) {
		errors = append(errors, "ConfigMap volume still exists after removal")
	}

	// Check Secret volume should still exist
	secretData, err := os.ReadFile("/etc/secret/secret.txt")
	if err != nil {
		errors = append(errors, fmt.Sprintf("Secret read error: %v", err))
	} else if string(secretData) != "secret-data" {
		errors = append(errors, fmt.Sprintf("Secret data mismatch: got %q", string(secretData)))
	}

	// Check EmptyDir volume should still exist
	testFile := "/tmp/scratch/test2.txt"
	if err := os.WriteFile(testFile, []byte("test2"), 0644); err != nil {
		errors = append(errors, fmt.Sprintf("EmptyDir write error: %v", err))
	}

	if len(errors) > 0 {
		http.Error(w, strings.Join(errors, "\n"), http.StatusInternalServerError)
		return
	}
	fmt.Fprintln(w, "OK")
}
`
	err = os.WriteFile(filepath.Join(root, "handle.go"), []byte(impl), 0644)
	if err != nil {
		t.Fatal(err)
	}

	// Redeploy and verify removal worked
	if err := newCmd(t, "deploy").Run(); err != nil {
		t.Fatal(err)
	}

	if !waitFor(t, fmt.Sprintf("http://%s.%s.%s", name, Namespace, Domain),
		withContentMatch("OK")) {
		t.Fatal("function failed after volume removal")
	}
}

// TestMetadata_Subscriptions verifies the full event flow using Knative Eventing:
// Producer function -> Broker -> Trigger -> Subscriber function
func TestMetadata_Subscriptions(t *testing.T) {
	// Verify Knative Eventing is installed
	checkCmd := exec.Command("kubectl", "get", "svc", "-n", "knative-eventing", "broker-ingress")
	checkCmd.Env = append(os.Environ(), "KUBECONFIG="+Kubeconfig)
	if err := checkCmd.Run(); err != nil {
		t.Skip("Skipping test: Knative Eventing is not installed (broker-ingress service not found)")
	}

	brokerName := "default"
	createBroker(t, Namespace, brokerName)
	defer deleteBroker(t, Namespace, brokerName)

	// Create subscriber function that receives CloudEvents
	subscriberName := "func-e2e-test-subscriber"
	subscriberRoot := fromCleanEnv(t, subscriberName)

	if err := newCmd(t, "init", "-l=go", "-t=cloudevents").Run(); err != nil {
		t.Fatal(err)
	}

	subscriberImpl := `package function

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/cloudevents/sdk-go/v2/event"
)

func Handle(ctx context.Context, e event.Event) (*event.Event, error) {
	os.WriteFile("/tmp/received_event", []byte(e.Type()), 0644)
	fmt.Printf("Received event: type=%s, source=%s, id=%s\n", e.Type(), e.Source(), e.ID())

	response := event.New()
	response.SetID(fmt.Sprintf("response-%d", time.Now().UnixNano()))
	response.SetSource("subscriber")
	response.SetType("test.response")
	response.SetData("application/json", map[string]string{
		"received_type": e.Type(),
		"status":        "received",
	})
	return &response, nil
}
`
	if err := os.WriteFile(filepath.Join(subscriberRoot, "handle.go"), []byte(subscriberImpl), 0644); err != nil {
		t.Fatal(err)
	}

	// Run func subscribe (without -v flag which subscribe doesn't support)
	subscribeCmd := exec.Command(Bin, "subscribe", "--filter", "type=test.event")
	subscribeCmd.Stdout = os.Stdout
	subscribeCmd.Stderr = os.Stderr
	t.Log("$ func subscribe --filter type=test.event")
	if err := subscribeCmd.Run(); err != nil {
		t.Fatal(err)
	}

	// Verify subscription config
	f, err := fn.NewFunction(subscriberRoot)
	if err != nil {
		t.Fatal(err)
	}
	if len(f.Deploy.Subscriptions) != 1 {
		t.Fatalf("expected 1 subscription, got %d", len(f.Deploy.Subscriptions))
	}

	if err := newCmd(t, "deploy").Run(); err != nil {
		t.Fatal(err)
	}
	defer clean(t, subscriberName, Namespace)

	subscriberURL := fmt.Sprintf("http://%s.%s.%s", subscriberName, Namespace, Domain)
	if !waitFor(t, subscriberURL, withTemplate("cloudevents")) {
		t.Fatal("subscriber did not become ready")
	}
	t.Log("Subscriber deployed and ready")
	waitForTrigger(t, Namespace, subscriberName)

	// Create producer function that sends CloudEvents to the broker
	producerName := "func-e2e-test-producer"
	_ = fromCleanEnv(t, producerName)

	if err := newCmd(t, "init", "-l=go", "-t=http").Run(); err != nil {
		t.Fatal(err)
	}

	producerImpl := fmt.Sprintf(`package function

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	brokerNamespace = "%s"
	brokerName      = "%s"
)

func Handle(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
		w.WriteHeader(200)
		fmt.Fprintf(w, "Producer is ready")
		return
	}

	brokerURL := fmt.Sprintf("http://broker-ingress.knative-eventing.svc.cluster.local/%%s/%%s", brokerNamespace, brokerName)
	eventBody := `+"`"+`{"message": "hello from producer"}`+"`"+`
	httpReq, err := http.NewRequest("POST", brokerURL, strings.NewReader(eventBody))
	if err != nil {
		w.WriteHeader(500)
		fmt.Fprintf(w, "Failed to create request: %%v", err)
		return
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("ce-specversion", "1.0")
	httpReq.Header.Set("ce-type", "test.event")
	httpReq.Header.Set("ce-source", "producer-function")
	httpReq.Header.Set("ce-id", fmt.Sprintf("evt-%%d", time.Now().UnixNano()))

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		w.WriteHeader(500)
		fmt.Fprintf(w, "Failed to send to broker at %%s: %%v", brokerURL, err)
		return
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		w.WriteHeader(200)
		fmt.Fprintf(w, "Event sent successfully to %%s (status %%d)", brokerURL, resp.StatusCode)
	} else {
		w.WriteHeader(500)
		fmt.Fprintf(w, "Broker %%s returned status %%d. Body: %%s", brokerURL, resp.StatusCode, string(body))
	}
}

`, Namespace, brokerName)

	producerRoot, _ := os.Getwd()
	if err := os.WriteFile(filepath.Join(producerRoot, "handle.go"), []byte(producerImpl), 0644); err != nil {
		t.Fatal(err)
	}

	if err := newCmd(t, "deploy").Run(); err != nil {
		t.Fatal(err)
	}
	defer clean(t, producerName, Namespace)

	producerURL := fmt.Sprintf("http://%s.%s.%s", producerName, Namespace, Domain)
	if !waitFor(t, producerURL, withContentMatch("Producer is ready")) {
		t.Fatal("producer did not become ready")
	}
	t.Log("Producer deployed and ready")

	// Invoke producer to trigger event flow
	t.Log("Invoking producer to send event to broker...")
	client := http.Client{Timeout: 30 * time.Second}
	resp, err := client.Post(producerURL, "application/json", strings.NewReader("{}"))
	if err != nil {
		t.Fatalf("Failed to invoke producer: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	t.Logf("Producer response: %s (Status: %d)", string(body), resp.StatusCode)

	if resp.StatusCode != 200 {
		t.Fatalf("Broker failed to accept event: Status %d. Body: %s", resp.StatusCode, string(body))
	}

	t.Log("Event sent to broker successfully. Waiting for subscriber...")
	if !waitFor(t, subscriberURL, withTemplate("cloudevents")) {
		t.Fatal("subscriber did not respond after event was sent")
	}
	t.Log("Event flow verified: Producer -> Broker -> Subscriber")
}

// createBroker creates a Knative Broker in the given namespace.
func createBroker(t *testing.T, namespace, name string) {
	t.Helper()

	brokerYAML := fmt.Sprintf(`apiVersion: eventing.knative.dev/v1
kind: Broker
metadata:
  name: %s
  namespace: %s
`, name, namespace)

	cmd := exec.Command("kubectl", "apply", "-f", "-")
	cmd.Stdin = strings.NewReader(brokerYAML)
	cmd.Env = append(os.Environ(), "KUBECONFIG="+Kubeconfig)

	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Logf("Warning: could not create broker: %v, output: %s", err, string(output))
		return
	}
	t.Logf("Created broker %s in namespace %s", name, namespace)

	waitCmd := exec.Command("kubectl", "wait", "--for=condition=Ready",
		fmt.Sprintf("broker/%s", name), "-n", namespace, "--timeout=60s")
	waitCmd.Env = append(os.Environ(), "KUBECONFIG="+Kubeconfig)
	waitOutput, err := waitCmd.CombinedOutput()
	if err != nil {
		t.Logf("Warning: broker may not be ready: %v, output: %s", err, string(waitOutput))
	} else {
		t.Logf("Broker %s is ready", name)
	}
}

// deleteBroker removes a Knative Broker from the given namespace.
func deleteBroker(t *testing.T, namespace, name string) {
	t.Helper()

	cmd := exec.Command("kubectl", "delete", "broker", name, "-n", namespace, "--ignore-not-found")
	cmd.Env = append(os.Environ(), "KUBECONFIG="+Kubeconfig)

	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Logf("Warning: could not delete broker: %v, output: %s", err, string(output))
		return
	}
	t.Logf("Deleted broker %s from namespace %s", name, namespace)
}

// waitForTrigger waits for the function's trigger to become ready.
func waitForTrigger(t *testing.T, namespace, functionName string) {
	t.Helper()

	triggerName := fmt.Sprintf("%s-function-trigger-0", functionName)

	cmd := exec.Command("kubectl", "wait", "--for=condition=Ready",
		fmt.Sprintf("trigger/%s", triggerName), "-n", namespace, "--timeout=60s")
	cmd.Env = append(os.Environ(), "KUBECONFIG="+Kubeconfig)

	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Logf("Warning: trigger may not be ready: %v, output: %s", err, string(output))
	} else {
		t.Logf("Trigger %s is ready", triggerName)
	}
}

// sendEventToBrokerWithRetry attempts to send a CloudEvent to the broker with retries.
func sendEventToBrokerWithRetry(t *testing.T, brokerURL, eventType, eventSource string, maxRetries int) bool {
	t.Helper()

	for i := 0; i < maxRetries; i++ {
		if i > 0 {
			t.Logf("Retry %d/%d for sending event to broker", i+1, maxRetries)
			time.Sleep(5 * time.Second)
		}

		success := sendEventToBrokerInternal(t, brokerURL, eventType, eventSource)
		if success {
			return true
		}
	}
	return false
}

// sendEventToBrokerInternal sends a CloudEvent to the broker using in-cluster networking.
func sendEventToBrokerInternal(t *testing.T, brokerURL, eventType, eventSource string) bool {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	clientConfig := k8s.GetClientConfig()
	dialer, err := k8s.NewInClusterDialer(ctx, clientConfig)
	if err != nil {
		t.Logf("Warning: could not create in-cluster dialer: %v", err)
		return false
	}
	defer dialer.Close()

	transport := &http.Transport{
		DialContext: dialer.DialContext,
	}
	client := &http.Client{
		Transport: transport,
		Timeout:   30 * time.Second,
	}

	eventID := fmt.Sprintf("test-event-%d", time.Now().UnixNano())
	eventJSON := fmt.Sprintf(`{"specversion":"1.0","type":"%s","source":"%s","id":"%s","datacontenttype":"application/json","data":{"message":"test subscription event"}}`, eventType, eventSource, eventID)

	req, err := http.NewRequestWithContext(ctx, "POST", brokerURL, strings.NewReader(eventJSON))
	if err != nil {
		t.Logf("Warning: could not create request to broker: %v", err)
		return false
	}

	req.Header.Set("Content-Type", "application/cloudevents+json")

	resp, err := client.Do(req)
	if err != nil {
		t.Logf("Warning: could not send event to broker: %v", err)
		return false
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		t.Logf("Event sent to broker successfully (status: %d)", resp.StatusCode)
		return true
	}
	t.Logf("Broker responded with status: %d", resp.StatusCode)
	return false
}
