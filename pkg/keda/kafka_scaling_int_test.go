//go:build integration_kafka

package keda_test

import (
	"context"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/rand"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"

	fn "knative.dev/func/pkg/functions"
	"knative.dev/func/pkg/k8s"
	"knative.dev/func/pkg/keda"
	testingk8s "knative.dev/func/pkg/testing/k8s"
)

// Kafka cluster provisioned by hack/cluster.sh when FUNC_CLUSTER_KAFKA=true:
// a single-node Strimzi broker named func-kafka in the kafka namespace, with a
// TLS + SCRAM-SHA-512 listener on 9093, a test-topic topic, and a test-kafka-user
// user. The operator publishes the cluster CA cert and the user's SASL password
// as secrets, which this test copies into the per-test namespace.
const (
	kafkaNamespace      = "kafka"
	kafkaClusterName    = "func-kafka"
	kafkaBootstrap      = kafkaClusterName + "-kafka-bootstrap." + kafkaNamespace + ".svc:9093"
	kafkaCASecretName   = kafkaClusterName + "-cluster-ca-cert"
	kafkaUserSecretName = "test-kafka-user"
)

var (
	scaledObjectGVR = schema.GroupVersionResource{
		Group:    "keda.sh",
		Version:  "v1alpha1",
		Resource: "scaledobjects",
	}
	triggerAuthGVR = schema.GroupVersionResource{
		Group:    "keda.sh",
		Version:  "v1alpha1",
		Resource: "triggerauthentications",
	}
)

// TestInt_KafkaScaling deploys a function with a Kafka-only KEDA trigger
// (no HTTP trigger, since a Deployment can only be owned by one ScaledObject
// and the http trigger's HTTPScaledObject creates its own -- see #4043) and
// verifies that the deployer creates a ScaledObject and TriggerAuthentication
// with the expected spec, and that both are cleaned up on removal.
//
// This requires a reachable Kafka broker. Deploy readiness is gated on the
// injected Kafka runtime sidecar connecting to the broker and getting partitions
// assigned (its readiness probe only reports ready once the consumer joins the
// group), so a function pointed at an unreachable broker never becomes ready and
// the deploy times out. The test therefore points the function at the real
// Strimzi broker and copies its CA cert and SASL password into the test
// namespace so the sidecar can authenticate (SCRAM-SHA-512 over TLS). No messages
// are produced: partition assignment alone satisfies readiness.
func TestInt_KafkaScaling(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute*10)
	t.Cleanup(cancel)

	name := "func-int-keda-kafka-" + rand.String(5)
	ns := testingk8s.Namespace(t, ctx)

	cliSet, err := k8s.NewKubernetesClientset()
	if err != nil {
		t.Fatal(err)
	}

	// Copy the broker's CA cert and the user's SASL password from the operator-
	// managed secrets in the kafka namespace into the per-test namespace, so the
	// sidecar can actually authenticate against the real broker.
	caCert := readClusterSecretKey(t, ctx, cliSet, kafkaNamespace, kafkaCASecretName, "ca.crt")
	caSecretName := name + "-ca"
	createSecretForTest(t, ctx, cliSet, ns, caSecretName, map[string][]byte{"ca.crt": caCert})

	saslPassword := readClusterSecretKey(t, ctx, cliSet, kafkaNamespace, kafkaUserSecretName, "password")
	userSecretName := name + "-user"
	createSecretForTest(t, ctx, cliSet, ns, userSecretName, map[string][]byte{"password": saslPassword})

	minScale := int64(0)
	maxScale := int64(10)
	lagThreshold := int64(7)

	function := fn.Function{
		SpecVersion: "SNAPSHOT",
		Root:        "/non/existent",
		Name:        name,
		Runtime:     "blub",
		Template:    "cloudevents",
		Created:     time.Now(),
		Expose:      "none",
		Deploy: fn.DeploySpec{
			// pinned prebuilt image: this test exercises the deployer's
			// Kafka-scaling object creation, not the build/image flow
			Image:     "quay.io/mvasek/func-test-service@sha256:2eca4de00d7569c8791634bdbb0c4d5ec8fb061b001549314591e839dabd5269",
			Namespace: ns,
		},
		Scale: &fn.ScaleOptions{
			Min: &minScale,
			Max: &maxScale,
			KEDA: &fn.KEDAScaleOptions{
				Triggers: []fn.KEDATrigger{
					{Type: "kafka", LagThreshold: &lagThreshold},
				},
			},
		},
		Run: fn.RunSpec{
			Kafka: &fn.KafkaConfig{
				Brokers:          kafkaBootstrap,
				Topic:            "test-topic",
				ConsumerGroup:    name + "-group",
				SecurityProtocol: "SASL_SSL",
				TLS: &fn.KafkaTLS{
					CACert: "/etc/kafka/ca/ca.crt",
				},
				SASL: &fn.KafkaSASL{
					Mechanism: "SCRAM-SHA-512",
					User:      "test-kafka-user",
					Password:  "{{ secret:" + userSecretName + ":password }}",
				},
			},
			Volumes: []fn.Volume{
				{Secret: ptrTo(caSecretName), Path: ptrTo("/etc/kafka/ca")},
			},
		},
	}

	deployer := keda.NewDeployer(keda.WithDeployerVerbose(false))
	remover := keda.NewRemover(false)

	_, err = deployer.Deploy(ctx, function)
	if err != nil {
		t.Fatalf("deploy failed: %v", err)
	}
	t.Cleanup(func() {
		if err := remover.Remove(context.Background(), name, ns); err != nil {
			t.Logf("error removing function: %v", err)
		}
	})

	dynClient, err := k8s.NewDynamicClient()
	if err != nil {
		t.Fatal(err)
	}

	so, err := dynClient.Resource(scaledObjectGVR).Namespace(ns).Get(ctx, name+"-kafka", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("expected ScaledObject to exist: %v", err)
	}

	spec, _ := so.Object["spec"].(map[string]interface{})
	if spec["minReplicaCount"] != int64(0) {
		t.Errorf("minReplicaCount = %v, want 0", spec["minReplicaCount"])
	}
	if spec["maxReplicaCount"] != int64(10) {
		t.Errorf("maxReplicaCount = %v, want 10", spec["maxReplicaCount"])
	}
	triggersList, _ := spec["triggers"].([]interface{})
	if len(triggersList) != 1 {
		t.Fatalf("expected 1 trigger, got %d", len(triggersList))
	}
	trigger, _ := triggersList[0].(map[string]interface{})
	if trigger["type"] != "kafka" {
		t.Errorf("trigger type = %v, want kafka", trigger["type"])
	}
	meta, _ := trigger["metadata"].(map[string]interface{})
	if meta["lagThreshold"] != "7" {
		t.Errorf("lagThreshold = %v, want 7", meta["lagThreshold"])
	}
	if meta["consumerGroup"] != name+"-group" {
		t.Errorf("consumerGroup = %v, want %s", meta["consumerGroup"], name+"-group")
	}
	if meta["sasl"] != "scram_sha512" {
		t.Errorf("sasl = %v, want scram_sha512", meta["sasl"])
	}
	if meta["tls"] != "enable" {
		t.Errorf("tls = %v, want enable", meta["tls"])
	}
	authRef, _ := trigger["authenticationRef"].(map[string]interface{})
	if authRef["name"] != name+"-kafka-auth" {
		t.Errorf("authenticationRef.name = %v, want %s", authRef["name"], name+"-kafka-auth")
	}

	ownerRefs, _ := so.Object["metadata"].(map[string]interface{})["ownerReferences"].([]interface{})
	if len(ownerRefs) != 1 {
		t.Fatalf("expected 1 owner reference, got %d", len(ownerRefs))
	}
	owner, _ := ownerRefs[0].(map[string]interface{})
	if owner["name"] != name || owner["kind"] != "Deployment" {
		t.Errorf("unexpected owner reference: %v", owner)
	}

	ta, err := dynClient.Resource(triggerAuthGVR).Namespace(ns).Get(ctx, name+"-kafka-auth", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("expected TriggerAuthentication to exist: %v", err)
	}

	taSpec, _ := ta.Object["spec"].(map[string]interface{})
	secretRefs, _ := taSpec["secretTargetRef"].([]interface{})
	if len(secretRefs) != 2 {
		t.Fatalf("expected 2 secretTargetRef entries (password, ca), got %d: %v", len(secretRefs), secretRefs)
	}
	envRefs, _ := taSpec["env"].([]interface{})
	if len(envRefs) != 1 {
		t.Fatalf("expected 1 env entry (username), got %d: %v", len(envRefs), envRefs)
	}
	envRef, _ := envRefs[0].(map[string]interface{})
	if envRef["parameter"] != "username" || envRef["name"] != "KAFKA_SASL_USER" {
		t.Errorf("unexpected env ref: %v", envRef)
	}

	// Removal: TriggerAuthentication/ScaledObject carry KEDA's own finalizer,
	// so deletion is asynchronous even after remover.Remove returns.
	if err := remover.Remove(ctx, name, ns); err != nil {
		t.Fatalf("remove failed: %v", err)
	}

	err = wait.PollUntilContextTimeout(ctx, time.Second, time.Minute, true, func(ctx context.Context) (bool, error) {
		_, soErr := dynClient.Resource(scaledObjectGVR).Namespace(ns).Get(ctx, name+"-kafka", metav1.GetOptions{})
		_, taErr := dynClient.Resource(triggerAuthGVR).Namespace(ns).Get(ctx, name+"-kafka-auth", metav1.GetOptions{})
		return apierrors.IsNotFound(soErr) && apierrors.IsNotFound(taErr), nil
	})
	if err != nil {
		t.Fatalf("expected ScaledObject and TriggerAuthentication to be removed: %v", err)
	}
}

func createSecretForTest(t *testing.T, ctx context.Context, cliSet *kubernetes.Clientset, ns, name string, data map[string][]byte) {
	t.Helper()
	_, err := cliSet.CoreV1().Secrets(ns).Create(ctx, &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Data:       data,
		Type:       corev1.SecretTypeOpaque,
	}, metav1.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}
}

// readClusterSecretKey reads a single key from a secret the Strimzi operator
// publishes in the kafka namespace. A missing secret means the Kafka broker was
// not provisioned (hack/cluster.sh with FUNC_CLUSTER_KAFKA=true), which is a
// setup error for this test rather than a product failure.
func readClusterSecretKey(t *testing.T, ctx context.Context, cliSet *kubernetes.Clientset, ns, name, key string) []byte {
	t.Helper()
	s, err := cliSet.CoreV1().Secrets(ns).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("reading secret %s/%s (is Kafka installed? set FUNC_CLUSTER_KAFKA=true): %v", ns, name, err)
	}
	v, ok := s.Data[key]
	if !ok {
		t.Fatalf("secret %s/%s has no key %q", ns, name, key)
	}
	return v
}

func ptrTo[T any](v T) *T {
	return &v
}
