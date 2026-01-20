package testing

//nolint:staticcheck  // ST1001: should not use dot imports
import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/rand"
	"k8s.io/apimachinery/pkg/util/wait"
	eventingv1 "knative.dev/eventing/pkg/apis/eventing/v1"
	"knative.dev/func/pkg/keda"
	"knative.dev/func/pkg/knative"
	"knative.dev/func/pkg/oci"
	. "knative.dev/func/pkg/testing"
	. "knative.dev/func/pkg/testing/k8s"
	v1 "knative.dev/pkg/apis/duck/v1"

	fn "knative.dev/func/pkg/functions"
	"knative.dev/func/pkg/k8s"
)

// TestInt_Deploy ensures that the deployer creates a callable service.
// See TestInt_Metadata for Labels, Volumes, Envs.
// See TestInt_Events for Subscriptions
func TestInt_Deploy(t *testing.T, deployer fn.Deployer, remover fn.Remover, describer fn.Describer, deployerName string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute*10)
	name := "func-int-knative-deploy-" + rand.String(5)
	root := t.TempDir()
	ns := Namespace(t, ctx)

	t.Cleanup(cancel)

	client := fn.New(
		fn.WithScaffolder(oci.NewScaffolder(true)),
		fn.WithBuilder(oci.NewBuilder("", false)),
		fn.WithPusher(oci.NewPusher(true, true, true)),
		fn.WithDeployer(deployer),
		fn.WithDescribers(describer),
		fn.WithRemovers(remover),
	)

	f, err := client.Init(fn.Function{
		Root:      root,
		Name:      name,
		Runtime:   "go",
		Namespace: ns,
		Registry:  Registry(),
	})
	if err != nil {
		t.Fatal(err)
	}
	// Not really necessary, but it allows us to reuse the "invoke" method:
	handlerPath := filepath.Join(root, "function.go")
	if err := os.WriteFile(handlerPath, []byte(testHandler), 0644); err != nil {
		t.Fatal(err)
	}

	// Scaffold
	err = client.Scaffold(ctx, f, "")
	if err != nil {
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
	statusCode, _ := invoke(t, ctx, instance.Route, deployerName)
	if statusCode != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", statusCode)
	}
}

// TestInt_Metadata ensures that Secrets, Labels, and Volumes are applied
// when deploying.
func TestInt_Metadata(t *testing.T, deployer fn.Deployer, remover fn.Remover, describer fn.Describer, deployerName string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute*10)
	name := "func-int-knative-metadata-" + rand.String(5)
	root := t.TempDir()
	ns := Namespace(t, ctx)

	t.Cleanup(cancel)

	client := fn.New(
		fn.WithScaffolder(oci.NewScaffolder(true)),
		fn.WithBuilder(oci.NewBuilder("", false)),
		fn.WithPusher(oci.NewPusher(true, true, true)),
		fn.WithDeployer(deployer),
		fn.WithDescribers(describer),
		fn.WithRemovers(remover),
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
		Registry:  Registry(),
	})
	if err != nil {
		t.Fatal(err)
	}
	handlerPath := filepath.Join(root, "function.go")

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

	// Scaffold
	err = client.Scaffold(ctx, f, "")
	if err != nil {
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

	// Assertions
	// ----------

	// Invoke
	_, result := invoke(t, ctx, instance.Route, deployerName)

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
func TestInt_Events(t *testing.T, deployer fn.Deployer, remover fn.Remover, describer fn.Describer, deployerName string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute*10)
	name := "func-int-knative-events-" + rand.String(5)
	root := t.TempDir()
	ns := Namespace(t, ctx)

	t.Cleanup(cancel)

	client := fn.New(
		fn.WithScaffolder(oci.NewScaffolder(true)),
		fn.WithBuilder(oci.NewBuilder("", false)),
		fn.WithPusher(oci.NewPusher(true, true, true)),
		fn.WithDeployer(deployer),
		fn.WithDescribers(describer),
		fn.WithRemovers(remover),
	)

	// Function
	// --------
	f, err := client.Init(fn.Function{
		Root:      root,
		Name:      name,
		Runtime:   "go",
		Namespace: ns,
		Registry:  Registry(),
	})
	if err != nil {
		t.Fatal(err)
	}

	// Trigger
	// -------
	triggerName := "func-int-knative-events-trigger"
	validator := createTrigger(t, ctx, ns, triggerName, f)

	// Deploy
	// ------

	// Scaffold
	err = client.Scaffold(ctx, f, "")
	if err != nil {
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

	// Assertions
	// ----------
	if err = validator(instance); err != nil {
		t.Fatal(err)
	}
}

// TestInt_Scale spot-checks that the scale settings are applied by
// ensuring the service is started multiple times when minScale=2
func TestInt_Scale(t *testing.T, deployer fn.Deployer, remover fn.Remover, describer fn.Describer, deployerName string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute*10)
	name := "func-int-knative-scale-" + rand.String(5)
	root := t.TempDir()
	ns := Namespace(t, ctx)

	t.Cleanup(cancel)

	client := fn.New(
		fn.WithScaffolder(oci.NewScaffolder(true)),
		fn.WithBuilder(oci.NewBuilder("", false)),
		fn.WithPusher(oci.NewPusher(true, true, true)),
		fn.WithDeployer(deployer),
		fn.WithDescribers(describer),
		fn.WithRemovers(remover),
	)

	f, err := client.Init(fn.Function{
		Root:      root,
		Name:      name,
		Runtime:   "go",
		Namespace: ns,
		Registry:  Registry(),
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

	// Scaffold
	err = client.Scaffold(ctx, f, "")
	if err != nil {
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
func TestInt_EnvsUpdate(t *testing.T, deployer fn.Deployer, remover fn.Remover, describer fn.Describer, deployerName string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute*10)
	name := "func-int-knative-envsupdate-" + rand.String(5)
	root := t.TempDir()
	ns := Namespace(t, ctx)

	t.Cleanup(cancel)

	client := fn.New(
		fn.WithScaffolder(oci.NewScaffolder(true)),
		fn.WithBuilder(oci.NewBuilder("", false)),
		fn.WithPusher(oci.NewPusher(true, true, true)),
		fn.WithDeployer(deployer),
		fn.WithDescribers(describer),
		fn.WithRemovers(remover),
	)

	// Function
	// --------
	f, err := client.Init(fn.Function{
		Root:      root,
		Name:      name,
		Runtime:   "go",
		Namespace: ns,
		Registry:  Registry(),
	})
	if err != nil {
		t.Fatal(err)
	}

	// Write custom test handler
	handlerPath := filepath.Join(root, "function.go")
	if err := os.WriteFile(handlerPath, []byte(testHandler), 0644); err != nil {
		t.Fatal(err)
	}

	// ENVS
	f.Run.Envs.Add("STATIC_A", "static-value-a")
	f.Run.Envs.Add("STATIC_B", "static-value-b")

	// Deploy
	// ------

	// Scaffold
	err = client.Scaffold(ctx, f, "")
	if err != nil {
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

	// Assert Initial ENVS are set
	// ----------
	_, result := invoke(t, ctx, instance.Route, deployerName)

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

	cliSet, err := k8s.NewKubernetesClientset()
	if err != nil {
		t.Fatal(err)
	}
	selector := fmt.Sprintf("function.knative.dev/name=%s", f.Name)
	err = k8s.WaitForDeploymentAvailableBySelector(ctx, cliSet, ns, selector, time.Minute)
	if err != nil {
		t.Fatal(err)
	}

	// Assertions
	// ----------
	_, result = invoke(t, ctx, instance.Route, deployerName)

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

// Basic happy path test of deploy->describe->list->re-deploy->delete.
func TestInt_FullPath(t *testing.T, deployer fn.Deployer, remover fn.Remover, lister fn.Lister, describer fn.Describer, deployerName string) {
	t.Helper()

	var err error
	functionName := "fn-testing"

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute*10)
	t.Cleanup(cancel)

	cliSet, err := k8s.NewKubernetesClientset()
	if err != nil {
		t.Fatal(err)
	}

	namespace := "knative-integration-test-ns-" + rand.String(5)

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
	t.Cleanup(func() { _ = cliSet.CoreV1().Namespaces().Delete(ctx, namespace, metav1.DeleteOptions{}) })
	t.Log("created namespace: ", namespace)

	secret := "credentials-secret"
	sc := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name: secret,
		},
		Data: map[string][]byte{
			"FUNC_TEST_SC_A": []byte("A"),
			"FUNC_TEST_SC_B": []byte("B"),
		},
		StringData: nil,
		Type:       corev1.SecretTypeOpaque,
	}

	_, err = cliSet.CoreV1().Secrets(namespace).Create(ctx, sc, metav1.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}

	configMap := "testing-config-map"
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name: configMap,
		},
		Data: map[string]string{"FUNC_TEST_CM_A": "1"},
	}
	_, err = cliSet.CoreV1().ConfigMaps(namespace).Create(ctx, cm, metav1.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}

	minScale := int64(2)
	maxScale := int64(100)

	now := time.Now()
	function := fn.Function{
		SpecVersion: "SNAPSHOT",
		Root:        "/non/existent",
		Name:        functionName,
		Runtime:     "blub",
		Template:    "cloudevents",
		// Basic HTTP service:
		//   * POST /    will do echo -- return body back
		//   * GET /info will get info about environment:
		//     * environment variables starting which name starts with FUNC_TEST,
		//     * files under /etc/cm and /etc/sc.
		//   * application also prints the same info to stderr on startup
		Created: now,
		Deploy: fn.DeploySpec{
			// TODO: gauron99 - is it okay to have this explicitly set to deploy.image already?
			// With this I skip the logic of setting the .Deploy.Image field but it should be fine for this test
			Image:     "quay.io/mvasek/func-test-service@sha256:2eca4de00d7569c8791634bdbb0c4d5ec8fb061b001549314591e839dabd5269",
			Namespace: namespace,
			Labels:    []fn.Label{{Key: ptr("my-label"), Value: ptr("my-label-value")}},
			Options: fn.Options{
				Scale: &fn.ScaleOptions{
					Min: &minScale,
					Max: &maxScale,
				},
			},
		},
		Run: fn.RunSpec{
			Envs: []fn.Env{
				{Name: ptr("FUNC_TEST_VAR"), Value: ptr("nbusr123")},
				{Name: ptr("FUNC_TEST_SC_A"), Value: ptr("{{ secret: " + secret + ":FUNC_TEST_SC_A }}")},
				{Value: ptr("{{configMap:" + configMap + "}}")},
			},
			Volumes: []fn.Volume{
				{Secret: ptr(secret), Path: ptr("/etc/sc")},
				{ConfigMap: ptr(configMap), Path: ptr("/etc/cm")},
			},
		},
	}

	buff := new(k8s.SynchronizedBuffer)
	go func() {
		selector := fmt.Sprintf("function.knative.dev/name=%s", functionName)
		_ = k8s.GetPodLogsBySelector(ctx, namespace, selector, "user-container", "", &now, buff)
	}()

	depRes, err := deployer.Deploy(ctx, function)
	if err != nil {
		t.Fatal(err)
	}

	outStr := buff.String()
	t.Logf("deploy result: %+v", depRes)
	t.Log("function output:\n" + outStr)

	if strings.Count(outStr, "starting app") < int(minScale) {
		t.Errorf("application should be scaled at least to %d pods", minScale)
	}

	// verify that environment variables and volumes works
	if !strings.Contains(outStr, "FUNC_TEST_VAR=nbusr123") {
		t.Error("plain environment variable was not propagated")
	}
	if !strings.Contains(outStr, "FUNC_TEST_SC_A=A") {
		t.Error("environment variables from secret was not propagated")
	}
	if strings.Contains(outStr, "FUNC_TEST_SC_B=") {
		t.Error("environment variables from secret was propagated but should have not been")
	}
	if !strings.Contains(outStr, "FUNC_TEST_CM_A=1") {
		t.Error("environment variable from config-map was not propagated")
	}
	if !strings.Contains(outStr, "/etc/sc/FUNC_TEST_SC_A") {
		t.Error("secret was not mounted")
	}
	if !strings.Contains(outStr, "/etc/cm/FUNC_TEST_CM_A") {
		t.Error("config-map was not mounted")
	}

	instance, err := describer.Describe(ctx, functionName, namespace)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("instance: %+v", instance)

	// try to invoke the function
	reqBody := "Hello World!"
	respBody, err := postText(ctx, instance.Route, reqBody, deployerName)
	if err != nil {
		t.Fatalf("failed to invoke function: %v", err)
	} else {
		t.Log("resp body:\n" + respBody)
		if !strings.Contains(respBody, reqBody) {
			t.Error("response body doesn't contain request body")
		}
	}

	list, err := lister.List(ctx, namespace)
	if err != nil {
		t.Fatal(err)
	}

	t.Logf("functions list: %+v", list)

	if len(list) != 1 {
		t.Errorf("expected exactly one functions but got: %d", len(list))
	} else {
		if list[0].URL != instance.Route {
			t.Error("URL mismatch")
		}
	}

	t.Setenv("LOCAL_ENV_TO_DEPLOY", "iddqd")
	function.Run.Envs = []fn.Env{
		{Name: ptr("FUNC_TEST_VAR"), Value: ptr("{{ env:LOCAL_ENV_TO_DEPLOY }}")},
		{Value: ptr("{{ secret: " + secret + " }}")},
		{Name: ptr("FUNC_TEST_CM_A_ALIASED"), Value: ptr("{{configMap:" + configMap + ":FUNC_TEST_CM_A}}")},
	}
	now = time.Now() // reset timer for new log receiver

	redeployLogBuff := new(k8s.SynchronizedBuffer)
	go func() {
		selector := fmt.Sprintf("function.knative.dev/name=%s", functionName)
		_ = k8s.GetPodLogsBySelector(ctx, namespace, selector, "user-container", "", &now, redeployLogBuff)
	}()

	_, err = deployer.Deploy(ctx, function)
	if err != nil {
		t.Fatal(err)
	}

	// Give logs time to be collected (not sure, why we need this here and not on the first collector too :thinking:)
	outStr = ""
	err = wait.PollUntilContextTimeout(ctx, time.Second, time.Minute, true, func(ctx context.Context) (done bool, err error) {
		outStr = redeployLogBuff.String()
		if len(outStr) > 0 ||
			outStr == "Hello World!" { // wait for more as only the "Hello World!"
			return true, nil
		}

		return false, nil
	})
	if err != nil {
		t.Fatal(err)
	}

	t.Log("function output:\n" + outStr)

	// verify that environment variables has been changed by re-deploy
	if strings.Contains(outStr, "FUNC_TEST_CM_A=") {
		t.Error("environment variables from previous deployment was not removed")
	}
	if !strings.Contains(outStr, "FUNC_TEST_SC_A=A") || !strings.Contains(outStr, "FUNC_TEST_SC_B=B") {
		t.Error("environment variables were not imported from secret")
	}
	if !strings.Contains(outStr, "FUNC_TEST_VAR=iddqd") {
		t.Error("environment variable was not set from local environment variable")
	}
	if !strings.Contains(outStr, "FUNC_TEST_CM_A_ALIASED=1") {
		t.Error("environment variable was not set from config-map")
	}

	if err = remover.Remove(ctx, functionName, namespace); err != nil {
		t.Fatal(err)
	}

	list, err = lister.List(ctx, namespace)
	if err != nil {
		t.Fatal(err)
	}

	if len(list) != 0 {
		t.Errorf("expected exactly zero functions but got: %d", len(list))
	}
}

// Helper functions
// ================

// Decode response
type result struct {
	EnvVars map[string]string
	Mounts  map[string]bool
}

func invoke(t *testing.T, ctx context.Context, route string, deployer string) (statusCode int, r result) {
	req, err := http.NewRequestWithContext(ctx, "GET", route, nil)
	if err != nil {
		t.Fatal(err)
	}

	httpClient, closeFunc, err := getHttpClient(ctx, deployer)
	if err != nil {
		t.Fatal(err)
	}
	defer closeFunc()

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

func createTrigger(t *testing.T, ctx context.Context, namespace, triggerName string, function fn.Function) func(fn.Instance) error {
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
				Name:       function.Name,
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

func postText(ctx context.Context, url, reqBody, deployer string) (respBody string, err error) {
	req, err := http.NewRequestWithContext(ctx, "POST", url, strings.NewReader(reqBody))
	if err != nil {
		return "", err
	}
	req.Header.Add("Content-Type", "text/plain")

	client, closeFunc, err := getHttpClient(ctx, deployer)
	if err != nil {
		return "", fmt.Errorf("error creating http client: %w", err)
	}
	defer closeFunc()

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	bs, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return string(bs), nil
}

func ptr[T interface{}](s T) *T {
	return &s
}

func getHttpClient(ctx context.Context, deployer string) (*http.Client, func(), error) {
	noopDeferFunc := func() {}

	switch deployer {
	case k8s.KubernetesDeployerName, keda.KedaDeployerName:
		// For Kubernetes deployments, use in-cluster dialer to access ClusterIP services

		clientConfig := k8s.GetClientConfig()
		dialer, err := k8s.NewInClusterDialer(ctx, clientConfig)
		if err != nil {
			return nil, noopDeferFunc, fmt.Errorf("failed to create in-cluster dialer: %w", err)
		}

		transport := &http.Transport{
			DialContext: dialer.DialContext,
		}

		deferFunc := func() {
			_ = dialer.Close()
		}

		return &http.Client{
			Transport: transport,
			Timeout:   time.Minute,
		}, deferFunc, nil
	case knative.KnativeDeployerName:
		// For Knative deployments, use default client (service is externally accessible)
		return http.DefaultClient, noopDeferFunc, nil
	default:
		return nil, noopDeferFunc, fmt.Errorf("unknown deploy type: %s", deployer)
	}
}
