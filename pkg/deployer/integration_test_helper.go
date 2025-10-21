//go:build integration
// +build integration

package deployer

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/rand"
	eventingv1 "knative.dev/eventing/pkg/apis/eventing/v1"
	v1 "knative.dev/pkg/apis/duck/v1"

	fn "knative.dev/func/pkg/functions"
	"knative.dev/func/pkg/k8s"
	"knative.dev/func/pkg/knative"
)

// Basic happy path test of deploy->describe->list->re-deploy->delete.
func IntegrationTest(t *testing.T, deployer fn.Deployer, remover fn.Remover, lister fn.Lister, describer fn.Describer, deployType string) {
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

	subscriberRef := v1.KReference{
		Kind:      "Service",
		Namespace: namespace,
		Name:      functionName,
	}

	switch deployType {
	case KnativeDeployerName:
		subscriberRef.APIVersion = "serving.knative.dev"
	case KubernetesDeployerName:
		subscriberRef.APIVersion = "v1"
	}

	trigger := "testing-trigger"
	tr := &eventingv1.Trigger{
		ObjectMeta: metav1.ObjectMeta{
			Name: trigger,
		},
		Spec: eventingv1.TriggerSpec{
			Broker:     "testing-broker",
			Subscriber: v1.Destination{Ref: &subscriberRef},
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
			DeployType: deployType,
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

	buff := new(knative.SynchronizedBuffer)
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
	respBody, err := postText(ctx, instance.Route, reqBody, deployType)
	if err != nil {
		t.Fatalf("failed to invoke function: %v", err)
	} else {
		t.Log("resp body:\n" + respBody)
		if !strings.Contains(respBody, reqBody) {
			t.Error("response body doesn't contain request body")
		}
	}

	// verify that trigger info is included in describe output
	if len(instance.Subscriptions) != 1 {
		t.Error("exactly one subscription is expected")
	} else {
		if instance.Subscriptions[0].Broker != "testing-broker" {
			t.Error("bad broker")
		}
		if instance.Subscriptions[0].Source != "test-event-source" {
			t.Error("bad source")
		}
		if instance.Subscriptions[0].Type != "test-event-type" {
			t.Error("bad type")
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

	redeployLogBuff := new(knative.SynchronizedBuffer)
	go func() {
		selector := fmt.Sprintf("function.knative.dev/name=%s", functionName)
		_ = k8s.GetPodLogsBySelector(ctx, namespace, selector, "user-container", "", &now, redeployLogBuff)
	}()

	_, err = deployer.Deploy(ctx, function)
	if err != nil {
		t.Fatal(err)
	}

	// Give logs time to be collected (not sure, why we need this here and not on the first collector too :thinking:)
	time.Sleep(5 * time.Second)

	outStr = redeployLogBuff.String()
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

	err = remover.Remove(ctx, functionName, namespace)
	if err != nil {
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

func postText(ctx context.Context, url, reqBody, deployType string) (respBody string, err error) {
	req, err := http.NewRequestWithContext(ctx, "POST", url, strings.NewReader(reqBody))
	if err != nil {
		return "", err
	}
	req.Header.Add("Content-Type", "text/plain")

	var client *http.Client

	// For Kubernetes deployments, use in-cluster dialer to access ClusterIP services
	if deployType == KubernetesDeployerName {
		clientConfig := k8s.GetClientConfig()
		dialer, err := k8s.NewInClusterDialer(ctx, clientConfig)
		if err != nil {
			return "", fmt.Errorf("failed to create in-cluster dialer: %w", err)
		}
		defer func() {
			_ = dialer.Close()
		}()

		transport := &http.Transport{
			DialContext: dialer.DialContext,
		}
		client = &http.Client{
			Transport: transport,
			Timeout:   time.Minute,
		}
	} else {
		// For Knative deployments, use default client (service is externally accessible)
		client = http.DefaultClient
	}

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
