//go:build e2e
// +build e2e

package e2e

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	fn "knative.dev/func/pkg/functions"
	fnhttp "knative.dev/func/pkg/http"
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
	type Function struct{}
	func New() *Function { return &Function{} }
	func (f *Function) Handle(w http.ResponseWriter, _ *http.Request) {
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
	err := os.WriteFile(filepath.Join(root, "function.go"), []byte(impl), 0644)
	if err != nil {
		t.Fatal(err)
	}
	if err := newCmd(t, "deploy").Run(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		clean(t, name, Namespace)
	}()
	if !waitFor(t, ksvcUrl(name)) {
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
	type Function struct{}
	func New() *Function { return &Function{} }
	func (f *Function) Handle(w http.ResponseWriter, _ *http.Request) {
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
	if err := os.WriteFile(filepath.Join(root, "function.go"), []byte(impl), 0644); err != nil {
		t.Fatal(err)
	}
	if err := newCmd(t, "deploy").Run(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		clean(t, name, Namespace)
	}()
	if !waitFor(t, ksvcUrl(name)) {
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
	type Function struct{}
	func New() *Function { return &Function{} }
	func (f *Function) Handle(w http.ResponseWriter, _ *http.Request) {
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
	if err := os.WriteFile(filepath.Join(root, "function.go"), []byte(impl), 0644); err != nil {
		t.Fatal(err)
	}
	if err := newCmd(t, "deploy").Run(); err != nil {
		t.Fatal(err)
	}
	if !waitFor(t, ksvcUrl(name)) {
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
	if !waitFor(t, ksvcUrl(name)) {
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
	if !waitFor(t, ksvcUrl(name)) {
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
	if !waitFor(t, ksvcUrl(name)) {
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

type Function struct{}
func New() *Function { return &Function{} }
func (f *Function) Handle(w http.ResponseWriter, _ *http.Request) {
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
	err := os.WriteFile(filepath.Join(root, "function.go"), []byte(impl), 0644)
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
	if !waitFor(t, ksvcUrl(name)) {
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

type Function struct{}
func New() *Function { return &Function{} }
func (f *Function) Handle(w http.ResponseWriter, _ *http.Request) {
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
	err = os.WriteFile(filepath.Join(root, "function.go"), []byte(impl), 0644)
	if err != nil {
		t.Fatal(err)
	}

	// Redeploy and verify removal worked
	if err := newCmd(t, "deploy").Run(); err != nil {
		t.Fatal(err)
	}

	if !waitFor(t, ksvcUrl(name)) {
		t.Fatal("function failed after volume removal")
	}
}

// Tests the complete event flow using func subscribe
func TestMetadata_Subscriptions(t *testing.T) {
	brokerName := "default"

	createBrokerWithCheck(t, Namespace, brokerName)

	uniqueEventID := fmt.Sprintf("e2e-test-%d", time.Now().UnixNano())

	subscriberName := "func-e2e-test-subscriber-knative"
	eventReceived := waitForEvent(t, subscriberName, uniqueEventID)

	subscriberRoot := fromCleanEnv(t, subscriberName)
	if err := newCmd(t, "init", "-l=go", "-t=cloudevents").Run(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(subscriberRoot, "function.go"),
		[]byte(subscriberCode()), 0644); err != nil {
		t.Fatal(err)
	}

	subscribeCmd := exec.Command(Bin, "subscribe", "--filter", "type=test.event")
	subscribeCmd.Stdout, subscribeCmd.Stderr = os.Stdout, os.Stderr
	if err := subscribeCmd.Run(); err != nil {
		t.Fatal(err)
	}

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

	subscriberURL := ksvcUrl(subscriberName)
	if !waitFor(t, subscriberURL, withTemplate("cloudevents")) {
		t.Fatal("subscriber not ready")
	}
	waitForTriggerKnative(t, Namespace, subscriberName)

	transport := fnhttp.NewRoundTripper()
	defer transport.Close()
	client := http.Client{
		Transport: transport,
		Timeout:   30 * time.Second,
	}
	url := fmt.Sprintf("http://broker-ingress.knative-eventing.svc/%s/%s", Namespace, brokerName)
	req, _ := http.NewRequestWithContext(t.Context(), "POST", url, strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("ce-specversion", "1.0")
	req.Header.Set("ce-type", "test.event")
	req.Header.Set("ce-source", "producer")
	req.Header.Set("ce-id", uniqueEventID)

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Failed to invoke producer: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 202 {
		t.Fatalf("Broker rejected event: code: %d, body: %q", resp.StatusCode, body)
	}
	t.Logf("Broker accepted event %s", uniqueEventID)

	select {
	case receivedID := <-eventReceived:
		t.Logf("Event flow verified (received: %s)", receivedID)
	case <-time.After(60 * time.Second):
		t.Fatal("Timeout: No callback from subscriber")
	}
}

func TestMetadata_Subscriptions_Raw(t *testing.T) {
	brokerName := "default"

	createBrokerWithCheck(t, Namespace, brokerName)

	uniqueEventID := fmt.Sprintf("e2e-test-%d", time.Now().UnixNano())

	subscriberName := "func-e2e-test-subscriber-raw"
	eventReceived := waitForEvent(t, subscriberName, uniqueEventID)

	subscriberRoot := fromCleanEnv(t, subscriberName)
	if err := newCmd(t, "init", "-l=go", "-t=cloudevents").Run(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(subscriberRoot, "function.go"),
		[]byte(subscriberCode()), 0644); err != nil {
		t.Fatal(err)
	}

	subscribeCmd := exec.Command(Bin, "subscribe", "--filter", "type=test.event")
	subscribeCmd.Stdout, subscribeCmd.Stderr = os.Stdout, os.Stderr
	if err := subscribeCmd.Run(); err != nil {
		t.Fatal(err)
	}

	f, err := fn.NewFunction(subscriberRoot)
	if err != nil {
		t.Fatal(err)
	}
	if len(f.Deploy.Subscriptions) != 1 {
		t.Fatalf("expected 1 subscription, got %d", len(f.Deploy.Subscriptions))
	}

	// Deploy with raw deployer to test trigger creation
	if err := newCmd(t, "deploy", "--deployer", "raw").Run(); err != nil {
		t.Fatal(err)
	}
	defer clean(t, subscriberName, Namespace)

	// Note: Raw deployer creates cluster-internal services without external routes,
	// so we can't use waitFor with domain-based URLs. Instead, we verify the
	// deployment is ready and then test event delivery directly.
	t.Log("Waiting for deployment to be ready...")
	waitForDeployment(t, Namespace, subscriberName)

	// Wait for trigger to be created and ready
	waitForTriggerRaw(t, Namespace, subscriberName)

	transport := fnhttp.NewRoundTripper()
	defer transport.Close()
	client := http.Client{
		Transport: transport,
		Timeout:   30 * time.Second,
	}
	url := fmt.Sprintf("http://broker-ingress.knative-eventing.svc/%s/%s", Namespace, brokerName)
	req, _ := http.NewRequestWithContext(t.Context(), "POST", url, strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("ce-specversion", "1.0")
	req.Header.Set("ce-type", "test.event")
	req.Header.Set("ce-source", "producer")
	req.Header.Set("ce-id", uniqueEventID)

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Failed to invoke producer: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 202 {
		t.Fatalf("Broker rejected event: code: %d, body: %q", resp.StatusCode, body)
	}
	t.Logf("Broker accepted event %s", uniqueEventID)

	select {
	case receivedID := <-eventReceived:
		t.Logf("Event flow verified (received: %s)", receivedID)
	case <-time.After(60 * time.Second):
		t.Fatal("Timeout: No callback from subscriber")
	}
}

func waitForEvent(t *testing.T, functionName, eventId string) <-chan string {
	t.Helper()

	eventReceived := make(chan string, 10)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	pr, pw := io.Pipe()
	cmd := exec.CommandContext(ctx, "stern", functionName+"-.*", "-n", Namespace)
	cmd.Stderr = io.Discard
	cmd.Stdout = pw
	cmd.Env = append(os.Environ(), "KUBECONFIG="+Kubeconfig)
	err := cmd.Start()
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		r := bufio.NewReader(pr)
		m, e := regexp.MatchReader(`EVENT_RECEIVED: id=`+eventId, r)
		if e != nil {
			panic(e)
		}
		if m {
			eventReceived <- "OK"
			close(eventReceived)
			cancel()
		}
		_, _ = io.Copy(io.Discard, r)
	}()

	return eventReceived
}

// CloudEvents handler that logs events
func subscriberCode() string {
	return `package function

import (
	"fmt"
	"github.com/cloudevents/sdk-go/v2/event"
)

type Function struct{}
func New() *Function { return &Function{} }
func (f *Function) Handle(e event.Event) (*event.Event, error) {
	fmt.Printf("EVENT_RECEIVED: id=%s type=%s source=%s\n", e.ID(), e.Type(), e.Source())
	r := event.New()
	r.SetID("response-" + e.ID())
	r.SetSource("subscriber")
	r.SetType("test.response")
	r.SetData("application/json", map[string]string{"status": "received"})
	return &r, nil
}
`
}

// createBrokerWithCheck creates a Knative Broker
func createBrokerWithCheck(t *testing.T, namespace, name string) {
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
		t.Fatalf("Failed to create broker: %v, output: %s", err, string(output))
	}
	t.Cleanup(func() {
		deleteBroker(t, namespace, name)
	})
	t.Logf("Created broker %s in namespace %s", name, namespace)

	waitCmd := exec.Command("kubectl", "wait", "--for=condition=Ready",
		fmt.Sprintf("broker/%s", name), "-n", namespace, "--timeout=60s")
	waitCmd.Env = append(os.Environ(), "KUBECONFIG="+Kubeconfig)
	waitOutput, err := waitCmd.CombinedOutput()
	if err != nil {
		t.Logf("Broker not ready: %v, output: %s", err, string(waitOutput))
	}
	t.Logf("Broker %s is ready", name)

	// Wait for broker-ingress service to be available
	t.Log("Waiting for broker-ingress service to be available...")
	for range 30 {
		checkCmd := exec.Command("kubectl", "get", "svc", "-n", "knative-eventing", "broker-ingress")
		checkCmd.Env = append(os.Environ(), "KUBECONFIG="+Kubeconfig)
		if err := checkCmd.Run(); err == nil {
			t.Log("broker-ingress service is available")
			return
		}
		time.Sleep(2 * time.Second)
	}
	t.Fatal("broker-ingress service check timed out")
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

// waitForTriggerKnative waits for the function's trigger to become ready.
// For Knative deployer, triggers are named with pattern: {functionName}-function-trigger-{index}
func waitForTriggerKnative(t *testing.T, namespace, functionName string) {
	t.Helper()

	// Knative deployer uses predictable sequential naming
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

// waitForTriggerRaw waits for the function's trigger to become ready.
// For raw deployer, triggers are named with pattern: {functionName}-trigger-{hash}
func waitForTriggerRaw(t *testing.T, namespace, functionName string) {
	t.Helper()

	// List triggers matching the pattern using kubectl
	// Raw deployer creates triggers with pattern: {functionName}-trigger-{hash}
	listCmd := exec.Command("kubectl", "get", "triggers", "-n", namespace,
		"-o", "name", "--field-selector", "metadata.name!=")
	listCmd.Env = append(os.Environ(), "KUBECONFIG="+Kubeconfig)
	output, err := listCmd.CombinedOutput()
	if err != nil {
		t.Logf("Warning: could not list triggers: %v", err)
		return
	}

	// Find trigger matching our function name pattern
	prefix := "trigger.eventing.knative.dev/" + functionName + "-trigger-"
	var triggerName string
	for _, line := range strings.Split(string(output), "\n") {
		if strings.HasPrefix(line, prefix) {
			// Extract just the trigger name
			triggerName = strings.TrimPrefix(line, "trigger.eventing.knative.dev/")
			break
		}
	}

	if triggerName == "" {
		t.Logf("Warning: no trigger found for function %s", functionName)
		return
	}

	t.Logf("Found trigger: %s", triggerName)

	cmd := exec.Command("kubectl", "wait", "--for=condition=Ready",
		fmt.Sprintf("trigger/%s", triggerName), "-n", namespace, "--timeout=60s")
	cmd.Env = append(os.Environ(), "KUBECONFIG="+Kubeconfig)

	waitOutput, err := cmd.CombinedOutput()
	if err != nil {
		t.Logf("Warning: trigger may not be ready: %v, output: %s", err, string(waitOutput))
	} else {
		t.Logf("Trigger %s is ready", triggerName)
	}
}

// waitForDeployment waits for a Kubernetes Deployment to become ready.
// This is used for raw deployer which creates Deployments instead of Knative Services.
func waitForDeployment(t *testing.T, namespace, name string) {
	t.Helper()

	cmd := exec.Command("kubectl", "wait", "--for=condition=Available",
		fmt.Sprintf("deployment/%s", name), "-n", namespace, "--timeout=120s")
	cmd.Env = append(os.Environ(), "KUBECONFIG="+Kubeconfig)

	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Deployment %s not ready: %v, output: %s", name, err, string(output))
	}
	t.Logf("Deployment %s is ready", name)
}
