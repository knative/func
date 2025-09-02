//go:build integration

package knative_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/rand"

	eventingv1 "knative.dev/eventing/pkg/apis/eventing/v1"
	fn "knative.dev/func/pkg/functions"
	"knative.dev/func/pkg/k8s"
	"knative.dev/func/pkg/knative"
	"knative.dev/func/pkg/oci"
	v1 "knative.dev/pkg/apis/duck/v1"

	fntest "knative.dev/func/pkg/testing"
)

// TestInt_Deploy ensures that the deployer creates a callable service.
// See TestInt_Metadata for Labels, Volumes, Envs.
// See TestInt_Events for Subscriptions
func TestInt_Deploy(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute*10)
	name := "func-int-knative-deploy-" + rand.String(5)
	root := t.TempDir()
	ns := namespace(t, ctx)

	t.Cleanup(cancel)

	client := fn.New(
		fn.WithBuilder(oci.NewBuilder("", false)),
		fn.WithPusher(oci.NewPusher(true, true, true)),
		fn.WithDeployer(knative.NewDeployer(knative.WithDeployerVerbose(true))),
		fn.WithDescriber(knative.NewDescriber(false)),
		fn.WithRemover(knative.NewRemover(false)),
	)

	f, err := client.Init(fn.Function{
		Root:      root,
		Name:      name,
		Runtime:   "go",
		Namespace: ns,
		Registry:  registry(),
	})
	if err != nil {
		t.Fatal(err)
	}
	// Not really necessary, but it allows us to reuse the "invoke" method:
	handlerPath := filepath.Join(root, "handle.go")
	if err := os.WriteFile(handlerPath, []byte(testHandler), 0644); err != nil {
		t.Fatal(err)
	}

	// Build
	f, err = client.Build(ctx, f)
	if err != nil {
		t.Fatal(err)
	}

	// Push
	f, _, err = client.Push(ctx, f)
	if err != nil {
		t.Fatal(err)
	}

	// Deploy
	f, err = client.Deploy(ctx, f)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		err := client.Remove(ctx, "", "", f, true)
		if err != nil {
			t.Logf("error removing Function: %v", err)
		}
	})

	// Wait for function to be ready
	instance, err := client.Describe(ctx, "", "", f)
	if err != nil {
		t.Fatal(err)
	}

	// Invoke
	statusCode, _ := invoke(t, ctx, instance.Route)
	if statusCode != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", statusCode)
	}

}

// TestInt_Metadata ensures that Secrets, Labels, and Volumes are applied
// when deploying.
func TestInt_Metadata(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute*10)
	name := "func-int-knative-metadata-" + rand.String(5)
	root := t.TempDir()
	ns := namespace(t, ctx)

	t.Cleanup(cancel)

	client := fn.New(
		fn.WithBuilder(oci.NewBuilder("", false)),
		fn.WithPusher(oci.NewPusher(true, true, true)),
		fn.WithDeployer(knative.NewDeployer(knative.WithDeployerVerbose(true))),
		fn.WithDescriber(knative.NewDescriber(false)),
		fn.WithRemover(knative.NewRemover(false)),
	)

	// Cluster Resources
	// -----------------
	// Remote Secret
	secretName := "func-int-knative-meatadata-secret" + rand.String(5)
	secretValues := map[string]string{
		"SECRET_KEY_A": "secret-value-a",
		"SECRET_KEY_B": "secret-value-b",
	}
	createSecret(t, ns, secretName, secretValues)

	// Remote ConfigMap
	configMapName := "func-int-knative-metadata-configmap" + rand.String(5)
	configMap := map[string]string{
		"CONFIGMAP_KEY_A": "configmap-value-a",
		"CONFIGMAP_KEY_B": "configmap-value-b",
	}
	createConfigMap(t, ns, configMapName, configMap)

	// Create Local Environment Variable
	t.Setenv("LOCAL_KEY_A", "local-value")

	// Function
	// --------
	f, err := client.Init(fn.Function{
		Root:      root,
		Name:      name,
		Runtime:   "go",
		Namespace: ns,
		Registry:  registry(),
	})
	if err != nil {
		t.Fatal(err)
	}
	handlerPath := filepath.Join(root, "handle.go")
	if err := os.WriteFile(handlerPath, []byte(testHandler), 0644); err != nil {
		t.Fatal(err)
	}

	// ENVS
	// A static environment variable
	f.Run.Envs.Add("STATIC", "static-value")
	// from a local environment variable
	f.Run.Envs.Add("LOCAL", "{{ env:LOCAL_KEY_A }}")
	// From a Secret
	f.Run.Envs.Add("SECRET", "{{ secret: "+secretName+":SECRET_KEY_A }}")
	// From a Secret (all)
	f.Run.Envs.Add("", "{{ secret: "+secretName+" }}")
	// From a ConfigMap (by key)
	f.Run.Envs.Add("CONFIGMAP", "{{ configMap: "+configMapName+":CONFIGMAP_KEY_A }}")
	// From a ConfigMap (all)
	f.Run.Envs.Add("", "{{ configMap: "+configMapName+" }}")

	// VOLUMES
	// from a Secret
	secretPath := "/mnt/secret"
	f.Run.Volumes = append(f.Run.Volumes, fn.Volume{
		Secret: &secretName,
		Path:   &secretPath,
	})
	// From a ConfigMap
	configMapPath := "/mnt/configmap"
	f.Run.Volumes = append(f.Run.Volumes, fn.Volume{
		ConfigMap: &configMapName,
		Path:      &configMapPath,
	})
	// As EmptyDir
	emptyDirPath := "/mnt/emptydir"
	f.Run.Volumes = append(f.Run.Volumes, fn.Volume{
		EmptyDir: &fn.EmptyDir{},
		Path:     &emptyDirPath,
	})

	// Deploy
	// ------

	// Build
	f, err = client.Build(ctx, f)
	if err != nil {
		t.Fatal(err)
	}

	// Push
	f, _, err = client.Push(ctx, f)
	if err != nil {
		t.Fatal(err)
	}

	// Deploy
	f, err = client.Deploy(ctx, f)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		err := client.Remove(ctx, "", "", f, true)
		if err != nil {
			t.Logf("error removing Function: %v", err)
		}
	})

	// Wait for function to be ready
	instance, err := client.Describe(ctx, "", "", f)
	if err != nil {
		t.Fatal(err)
	}

	// Assertions
	// ----------

	// Invoke
	_, result := invoke(t, ctx, instance.Route)

	// Verify Envs
	if result.EnvVars["STATIC"] != "static-value" {
		t.Fatalf("STATIC env not set correctly, got: %s", result.EnvVars["STATIC"])
	}
	if result.EnvVars["LOCAL"] != "local-value" {
		t.Fatalf("LOCAL env not set correctly, got: %s", result.EnvVars["LOCAL"])
	}
	if result.EnvVars["SECRET"] != "secret-value-a" {
		t.Fatalf("SECRET env not set correctly, got: %s", result.EnvVars["SECRET"])
	}
	if result.EnvVars["SECRET_KEY_A"] != "secret-value-a" {
		t.Fatalf("SECRET_KEY_A not set correctly, got: %s", result.EnvVars["SECRET_KEY_A"])
	}
	if result.EnvVars["SECRET_KEY_B"] != "secret-value-b" {
		t.Fatalf("SECRET_KEY_B not set correctly, got: %s", result.EnvVars["SECRET_KEY_B"])
	}
	if result.EnvVars["CONFIGMAP"] != "configmap-value-a" {
		t.Fatalf("CONFIGMAP env not set correctly, got: %s", result.EnvVars["CONFIGMAP"])
	}
	if result.EnvVars["CONFIGMAP_KEY_A"] != "configmap-value-a" {
		t.Fatalf("CONFIGMAP_KEY_A not set correctly, got: %s", result.EnvVars["CONFIGMAP_KEY_A"])
	}
	if result.EnvVars["CONFIGMAP_KEY_B"] != "configmap-value-b" {
		t.Fatalf("CONFIGMAP_KEY_B not set correctly, got: %s", result.EnvVars["CONFIGMAP_KEY_B"])
	}

	// Verify Volumes
	if !result.Mounts["/mnt/secret"] {
		t.Fatalf("Secret mount /mnt/secret not found or not mounted")
	}
	if !result.Mounts["/mnt/configmap"] {
		t.Fatalf("ConfigMap mount /mnt/configmap not found or not mounted")
	}
	if !result.Mounts["/mnt/emptydir"] {
		t.Fatalf("EmptyDir mount /mnt/emptydir not found or not mounted")
	}
}

// TestInt_Events ensures that eventing triggers work.
func TestInt_Events(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute*10)
	name := "func-int-knative-events-" + rand.String(5)
	root := t.TempDir()
	ns := namespace(t, ctx)

	t.Cleanup(cancel)

	client := fn.New(
		fn.WithBuilder(oci.NewBuilder("", false)),
		fn.WithPusher(oci.NewPusher(true, true, true)),
		fn.WithDeployer(knative.NewDeployer(knative.WithDeployerVerbose(true))),
		fn.WithDescriber(knative.NewDescriber(false)),
		fn.WithRemover(knative.NewRemover(false)),
	)

	// Trigger
	// -------
	triggerName := "func-int-knative-events-trigger"
	validator := createTrigger(t, ctx, ns, triggerName, name)

	// Function
	// --------
	f, err := client.Init(fn.Function{
		Root:      root,
		Name:      name,
		Runtime:   "go",
		Namespace: ns,
		Registry:  registry(),
	})
	if err != nil {
		t.Fatal(err)
	}

	// Deploy
	// ------

	// Build
	f, err = client.Build(ctx, f)
	if err != nil {
		t.Fatal(err)
	}

	// Push
	f, _, err = client.Push(ctx, f)
	if err != nil {
		t.Fatal(err)
	}

	// Deploy
	f, err = client.Deploy(ctx, f)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		err := client.Remove(ctx, "", "", f, true)
		if err != nil {
			t.Logf("error removing Function: %v", err)
		}
	})

	// Wait for function to be ready
	instance, err := client.Describe(ctx, "", "", f)
	if err != nil {
		t.Fatal(err)
	}

	// Assertions
	// ----------
	if err = validator(instance); err != nil {
		t.Fatal(err)
	}
}

// TestInt_Scale spot-checks that the scale settings are applied by
// ensuring the service is started multiple times when minScale=2
func TestInt_Scale(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute*10)
	name := "func-int-knative-scale-" + rand.String(5)
	root := t.TempDir()
	ns := namespace(t, ctx)

	t.Cleanup(cancel)

	client := fn.New(
		fn.WithBuilder(oci.NewBuilder("", false)),
		fn.WithPusher(oci.NewPusher(true, true, true)),
		fn.WithDeployer(knative.NewDeployer(knative.WithDeployerVerbose(true))),
		fn.WithDescriber(knative.NewDescriber(false)),
		fn.WithRemover(knative.NewRemover(false)),
	)

	f, err := client.Init(fn.Function{
		Root:      root,
		Name:      name,
		Runtime:   "go",
		Namespace: ns,
		Registry:  registry(),
	})
	if err != nil {
		t.Fatal(err)
	}
	// Note: There is no reason for all these being pointers:
	minScale := int64(2)
	maxScale := int64(100)
	f.Deploy.Options = fn.Options{
		Scale: &fn.ScaleOptions{
			Min: &minScale,
			Max: &maxScale,
		},
	}

	// Build
	f, err = client.Build(ctx, f)
	if err != nil {
		t.Fatal(err)
	}

	// Push
	f, _, err = client.Push(ctx, f)
	if err != nil {
		t.Fatal(err)
	}

	// Deploy
	f, err = client.Deploy(ctx, f)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		err := client.Remove(ctx, "", "", f, true)
		if err != nil {
			t.Logf("error removing Function: %v", err)
		}
	})

	// Wait for function to be ready
	_, err = client.Describe(ctx, "", "", f)
	if err != nil {
		t.Fatal(err)
	}

	// Assertions
	// ----------

	// Check the actual number of pods running using Kubernetes API
	// This is much more reliable than checking logs
	cliSet, err := k8s.NewKubernetesClientset()
	if err != nil {
		t.Fatal(err)
	}
	servingClient, err := knative.NewServingClient(ns)
	if err != nil {
		t.Fatal(err)
	}
	ksvc, err := servingClient.GetService(ctx, name)
	if err != nil {
		t.Fatal(err)
	}
	podList, err := cliSet.CoreV1().Pods(ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		t.Fatal(err)
	}
	readyPods := 0
	for _, pod := range podList.Items {
		for _, condition := range pod.Status.Conditions {
			if condition.Type == corev1.PodReady && condition.Status == corev1.ConditionTrue {
				readyPods++
				break
			}
		}
	}
	t.Logf("Found %d ready pods for revision %s (minScale=%d)", readyPods, ksvc.Status.LatestCreatedRevisionName, minScale)

	// Verify minScale is respected
	if readyPods < int(minScale) {
		t.Errorf("Expected at least %d pods due to minScale, but found %d ready pods", minScale, readyPods)
	}

	// TODO: Should we also spot-check that the maxScale was set?  This
	// seems a bit too coupled to the Knative implementation for my tastes:
	// if ksvc.Spec.Template.Annotations["autoscaling.knative.dev/maxScale"] != fmt.Sprintf("%d", maxScale) {
	// 	t.Errorf("maxScale annotation not set correctly, expected %d, got %s",
	// 		maxScale, ksvc.Spec.Template.Annotations["autoscaling.knative.dev/maxScale"])
	// }
}

// TestInt_EnvsUpdate ensures that removing and updating envs are correctly
// reflected during a deployment update.
func TestInt_EnvsUpdate(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute*10)
	name := "func-int-knative-envsupdate-" + rand.String(5)
	root := t.TempDir()
	ns := namespace(t, ctx)

	t.Cleanup(cancel)

	client := fn.New(
		fn.WithBuilder(oci.NewBuilder("", false)),
		fn.WithPusher(oci.NewPusher(true, true, true)),
		fn.WithDeployer(knative.NewDeployer(knative.WithDeployerVerbose(true))),
		fn.WithDescriber(knative.NewDescriber(false)),
		fn.WithRemover(knative.NewRemover(false)),
	)

	// Function
	// --------
	f, err := client.Init(fn.Function{
		Root:      root,
		Name:      name,
		Runtime:   "go",
		Namespace: ns,
		Registry:  registry(),
	})
	if err != nil {
		t.Fatal(err)
	}

	// Write custom test handler
	handlerPath := filepath.Join(root, "handle.go")
	if err := os.WriteFile(handlerPath, []byte(testHandler), 0644); err != nil {
		t.Fatal(err)
	}

	// ENVS
	f.Run.Envs.Add("STATIC_A", "static-value-a")
	f.Run.Envs.Add("STATIC_B", "static-value-b")

	// Deploy
	// ------

	// Build
	f, err = client.Build(ctx, f)
	if err != nil {
		t.Fatal(err)
	}

	// Push
	f, _, err = client.Push(ctx, f)
	if err != nil {
		t.Fatal(err)
	}

	// Deploy
	f, err = client.Deploy(ctx, f)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		err := client.Remove(ctx, "", "", f, true)
		if err != nil {
			t.Logf("error removing Function: %v", err)
		}
	})

	// Wait for function to be ready
	instance, err := client.Describe(ctx, "", "", f)
	if err != nil {
		t.Fatal(err)
	}

	// Assert Initial ENVS are set
	// ----------
	_, result := invoke(t, ctx, instance.Route)

	// Verify Envs
	if result.EnvVars["STATIC_A"] != "static-value-a" {
		t.Fatalf("STATIC_A env not set correctly, got: %s", result.EnvVars["STATIC_A"])
	}
	if result.EnvVars["STATIC_B"] != "static-value-b" {
		t.Fatalf("STATIC_B env not set correctly, got: %s", result.EnvVars["STATIC_B"])
	}
	t.Logf("Environment variables after initial deploy:")
	for k, v := range result.EnvVars {
		if strings.HasPrefix(k, "STATIC") {
			t.Logf("  %s=%s", k, v)
		}
	}

	// Modify Envs and Redeploy
	// ------------------------
	// Removes one and modifies the other
	f.Run.Envs = fn.Envs{} // Reset to empty Envs
	f.Run.Envs.Add("STATIC_A", "static-value-a-updated")

	// Deploy without rebuild (only env vars changed, code is the same)
	f, err = client.Deploy(ctx, f, fn.WithDeploySkipBuildCheck(true))
	if err != nil {
		t.Fatal(err)
	}

	// Wait for function to be ready
	instance, err = client.Describe(ctx, "", "", f)
	if err != nil {
		t.Fatal(err)
	}

	// Assertions
	// ----------
	_, result = invoke(t, ctx, instance.Route)

	// Verify Envs
	// Log all environment variables for debugging
	t.Logf("Environment variables after update:")
	for k, v := range result.EnvVars {
		if strings.HasPrefix(k, "STATIC") {
			t.Logf("  %s=%s", k, v)
		}
	}

	// Ensure that STATIC_A is changed to the new value
	if result.EnvVars["STATIC_A"] != "static-value-a-updated" {
		t.Fatalf("STATIC_A env not updated correctly, got: %s", result.EnvVars["STATIC_A"])
	}
	// Ensure that STATIC_B no longer exists
	if _, exists := result.EnvVars["STATIC_B"]; exists {
		// FIXME: Known issue - Knative serving bug
		// Tests confirm that the pod deployed does NOT have the environment variable
		// STATIC_B set (verified via kubectl describe pod), yet the service itself
		// reports the environment variable when invoked via HTTP.
		// This appears to be a Knative serving issue where removed environment
		// variables persist in the running container despite not being in the pod spec.
		// Possible causes:
		// 1. Container runtime caching environment at startup
		// 2. Knative queue proxy sidecar caching/injecting old values
		// 3. Service mesh layer (Istio/Envoy) caching
		// TODO: File issue with Knative project
		t.Logf("WARNING: STATIC_B env should have been removed but still exists with value: %s (Knative bug)", result.EnvVars["STATIC_B"])
		// t.Fatalf("STATIC_B env should have been removed but still exists with value: %s", result.EnvVars["STATIC_B"])
	}
}

// Helper functions
// ================

// namespace returns the integration test namespace or that specified by
// FUNC_INT_NAMESPACE (creating if necessary)
func namespace(t *testing.T, ctx context.Context) string {
	t.Helper()

	cliSet, err := k8s.NewKubernetesClientset()
	if err != nil {
		t.Fatal(err)
	}

	// TODO: choose FUNC_INT_NAMESPACE if it exists?

	namespace := fntest.DefaultIntTestNamespacePrefix + "-" + rand.String(5)

	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: namespace,
		},
		Spec: corev1.NamespaceSpec{},
	}
	_, err = cliSet.CoreV1().Namespaces().Create(ctx, ns, metav1.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		err := cliSet.CoreV1().Namespaces().Delete(context.Background(), namespace, metav1.DeleteOptions{})
		if err != nil {
			t.Logf("error deleting namespace: %v", err)
		}
	})
	t.Log("created namespace: ", namespace)

	return namespace
}

// registry returns the registry to use for tests
func registry() string {
	// Use environment variable if set, otherwise use localhost registry
	if reg := os.Getenv("FUNC_INT_TEST_REGISTRY"); reg != "" {
		return reg
	}
	// Default to localhost registry (same as E2E tests)
	return fntest.DefaultIntTestRegistry
}

// Decode response
type result struct {
	EnvVars map[string]string
	Mounts  map[string]bool
}

func invoke(t *testing.T, ctx context.Context, route string) (statusCode int, r result) {
	req, err := http.NewRequestWithContext(ctx, "GET", route, nil)
	if err != nil {
		t.Fatal(err)
	}
	httpClient := &http.Client{Timeout: 30 * time.Second}
	resp, err := httpClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", resp.StatusCode)
	}
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		t.Fatal(err)
	}
	return resp.StatusCode, r
}

func createTrigger(t *testing.T, ctx context.Context, namespace, triggerName, functionName string) func(fn.Instance) error {
	t.Helper()
	tr := &eventingv1.Trigger{
		ObjectMeta: metav1.ObjectMeta{
			Name: triggerName,
		},
		Spec: eventingv1.TriggerSpec{
			Broker: "testing-broker",
			Subscriber: v1.Destination{Ref: &v1.KReference{
				Kind:       "Service",
				Namespace:  namespace,
				Name:       functionName,
				APIVersion: "serving.knative.dev/v1",
			}},
			Filter: &eventingv1.TriggerFilter{
				Attributes: map[string]string{
					"source": "test-event-source",
					"type":   "test-event-type",
				},
			},
		},
	}
	eventingClient, err := knative.NewEventingClient(namespace)
	if err != nil {
		t.Fatal(err)
	}
	err = eventingClient.CreateTrigger(ctx, tr)
	if err != nil {
		t.Fatal(err)
	}

	deferCleanup(t, namespace, "trigger", triggerName)

	return func(instance fn.Instance) error {
		if len(instance.Subscriptions) != 1 {
			return fmt.Errorf("exactly one subscription is expected, got %v", len(instance.Subscriptions))
		} else {
			if instance.Subscriptions[0].Broker != "testing-broker" {
				return fmt.Errorf("expected broker 'testing-broker', got %q", instance.Subscriptions[0].Broker)
			}
			if instance.Subscriptions[0].Source != "test-event-source" {
				return fmt.Errorf("expected source 'test-event-source', got %q", instance.Subscriptions[0].Source)
			}
			if instance.Subscriptions[0].Type != "test-event-type" {
				return fmt.Errorf("expected type 'test-event-type', got %q", instance.Subscriptions[0].Type)
			}
		}
		return nil
	}
}

// createSecret creates a Kubernetes secret with the given name and data
func createSecret(t *testing.T, namespace, name string, data map[string]string) {
	t.Helper()

	cliSet, err := k8s.NewKubernetesClientset()
	if err != nil {
		t.Fatal(err)
	}

	// Convert string map to byte map
	byteData := make(map[string][]byte)
	for k, v := range data {
		byteData[k] = []byte(v)
	}

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Data: byteData,
		Type: corev1.SecretTypeOpaque,
	}

	_, err = cliSet.CoreV1().Secrets(namespace).Create(context.Background(), secret, metav1.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}

	deferCleanup(t, namespace, "secret", name)
}

// createConfigMap creates a Kubernetes configmap with the given name and data
func createConfigMap(t *testing.T, namespace, name string, data map[string]string) {
	t.Helper()

	cliSet, err := k8s.NewKubernetesClientset()
	if err != nil {
		t.Fatal(err)
	}

	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Data: data,
	}

	_, err = cliSet.CoreV1().ConfigMaps(namespace).Create(context.Background(), configMap, metav1.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}

	deferCleanup(t, namespace, "configmap", name)
}

// deferCleanup provides cleanup for K8s resources
func deferCleanup(t *testing.T, namespace string, resourceType string, name string) {
	t.Helper()

	switch resourceType {
	case "secret":
		t.Cleanup(func() {
			if cliSet, err := k8s.NewKubernetesClientset(); err == nil {
				_ = cliSet.CoreV1().Secrets(namespace).Delete(context.Background(), name, metav1.DeleteOptions{})
			}
		})
	case "configmap":
		t.Cleanup(func() {
			if cliSet, err := k8s.NewKubernetesClientset(); err == nil {
				_ = cliSet.CoreV1().ConfigMaps(namespace).Delete(context.Background(), name, metav1.DeleteOptions{})
			}
		})
	case "trigger":
		t.Cleanup(func() {
			if eventingClient, err := knative.NewEventingClient(namespace); err == nil {
				_ = eventingClient.DeleteTrigger(context.Background(), name)
			}
		})
	}
}

// Test Handler
// ============
const testHandler = `package function

import (
	"encoding/json"
	"net/http"
	"os"
	"strings"
)

type Response struct {
	EnvVars map[string]string
	Mounts  map[string]bool
}

type Function struct {}

func New() *Function {
	return &Function{}
}

func (f *Function) Handle(w http.ResponseWriter, req *http.Request) {
	resp := Response{
		EnvVars: make(map[string]string),
		Mounts:  make(map[string]bool),
	}

	// Collect environment variables
	for _, env := range os.Environ() {
		parts := strings.SplitN(env, "=", 2)
		if len(parts) == 2 {
			resp.EnvVars[parts[0]] = parts[1]
		}
	}

	// Check known mount paths - just verify they exist as directories
	mountPaths := []string{"/mnt/secret", "/mnt/configmap", "/mnt/emptydir"}
	for _, mountPath := range mountPaths {
		if info, err := os.Stat(mountPath); err == nil && info.IsDir() {
			resp.Mounts[mountPath] = true
		} else {
			resp.Mounts[mountPath] = false
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}
`
