//go:build integration_kafka

package keda_test

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/rand"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"

	fn "knative.dev/func/pkg/functions"
	"knative.dev/func/pkg/k8s"
	testingk8s "knative.dev/func/pkg/testing/k8s"
)

// TestInt_KafkaPythonDelivery proves the language-agnostic claim for Python: a
// plain Python cloudevents function, built and deployed through func on the keda
// deployer, receives Kafka records via the injected kafka-runtime sidecar as
// CloudEvents over HTTP -- the function itself contains no Kafka code.
//
// Unlike TestInt_KafkaScaling (which pins a prebuilt image and only asserts the
// KEDA objects), this test exercises the real build+deploy pipeline for Python
// and asserts end-to-end delivery:
//  1. func create -l python -t cloudevents, add run.kafka + a kafka KEDA trigger
//  2. func deploy (builds the Python image, pushes to the in-cluster registry,
//     injects the sidecar) against the real Strimzi broker
//  3. produce one record to test-topic
//  4. assert the Python user-container logged receipt ("Request Received")
//  5. assert the consumer group drains to lag 0 (the sidecar commits the offset
//     only on a 2xx from the function, so lag 0 confirms delivery succeeded)
//
// Environment: the Strimzi broker provisioned by hack/cluster.sh with
// FUNC_CLUSTER_KAFKA=true (TLS + SCRAM-SHA-512 on 9093, topic test-topic, user
// test-kafka-user) plus KEDA and the in-cluster registry mirror -- i.e. the slim
// cluster the Kafka integration-tests CI job already stands up. The CA cert and
// SASL password are copied from the operator-managed secrets into the per-test
// namespace so the sidecar can authenticate, exactly as TestInt_KafkaScaling
// does.
//
// Redelivery-on-failure is intentionally out of scope here (it needs the
// function to fail then recover deterministically); this test covers the
// delivery + lag-drain half of the acceptance criteria.
func TestInt_KafkaPythonDelivery(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute*25)
	t.Cleanup(cancel)

	name := "func-int-kafka-py-" + rand.String(5)
	group := name + "-group"
	ns := testingk8s.Namespace(t, ctx)

	cliSet, err := k8s.NewKubernetesClientset()
	if err != nil {
		t.Fatal(err)
	}

	// Copy the broker CA cert and the user's SASL password from the operator-
	// managed secrets in the kafka namespace into the per-test namespace so the
	// injected sidecar can authenticate against the real broker (SCRAM over TLS).
	caCert := readClusterSecretKey(t, ctx, cliSet, kafkaNamespace, kafkaCASecretName, "ca.crt")
	caSecretName := name + "-ca"
	createSecretForTest(t, ctx, cliSet, ns, caSecretName, map[string][]byte{"ca.crt": caCert})

	saslPassword := readClusterSecretKey(t, ctx, cliSet, kafkaNamespace, kafkaUserSecretName, "password")
	userSecretName := name + "-user"
	createSecretForTest(t, ctx, cliSet, ns, userSecretName, map[string][]byte{"password": saslPassword})

	// Build the func CLI from the repo under test, then scaffold a Python
	// cloudevents function with it.
	funcBin := buildFuncCLI(t)
	funcDir := t.TempDir()
	runFunc(t, ctx, funcBin, funcDir, nil, "create", "-l", "python", "-t", "cloudevents", funcDir)

	// Configure the generated function: deploy via keda with the sidecar, invoke
	// as cloudevent, keep one replica up so the consumer stays joined, and point
	// run.kafka at the real broker over SASL_SSL.
	f, err := fn.NewFunction(funcDir)
	if err != nil {
		t.Fatalf("loading created function: %v", err)
	}
	f.Name = name
	f.Namespace = ns
	f.Deployer = "keda"
	f.Invoke = "cloudevent"
	f.Expose = "none"
	minScale := int64(1)
	maxScale := int64(1)
	lagThreshold := int64(5)
	f.Scale = &fn.ScaleOptions{
		Min: &minScale,
		Max: &maxScale,
		KEDA: &fn.KEDAScaleOptions{
			Triggers: []fn.KEDATrigger{
				{Type: "kafka", LagThreshold: &lagThreshold},
			},
		},
	}
	f.Run.Kafka = &fn.KafkaConfig{
		Brokers:          kafkaBootstrap,
		Topic:            "test-topic",
		ConsumerGroup:    group,
		SecurityProtocol: "SASL_SSL",
		TLS: &fn.KafkaTLS{
			CACert: "/etc/kafka/ca/ca.crt",
		},
		SASL: &fn.KafkaSASL{
			Mechanism: "SCRAM-SHA-512",
			User:      "test-kafka-user",
			Password:  "{{ secret:" + userSecretName + ":password }}",
		},
	}
	f.Run.Volumes = []fn.Volume{
		{Secret: ptrTo(caSecretName), Path: ptrTo("/etc/kafka/ca")},
	}
	if err := f.Write(); err != nil {
		t.Fatalf("writing function config: %v", err)
	}

	// Build + push + deploy. Use the in-cluster registry mirror (insecure), and
	// honor a FUNC_KAFKA_RUNTIME_IMAGE override if the caller set one (otherwise
	// the deployer's built-in default sidecar image is used).
	deployEnv := []string{
		"FUNC_REGISTRY=registry.localtest.me/func",
		"FUNC_REGISTRY_INSECURE=true",
	}
	if img := os.Getenv("FUNC_KAFKA_RUNTIME_IMAGE"); img != "" {
		deployEnv = append(deployEnv, "FUNC_KAFKA_RUNTIME_IMAGE="+img)
	}
	runFunc(t, ctx, funcBin, funcDir, deployEnv, "deploy", "--namespace", ns, "--verbose")
	t.Cleanup(func() {
		// Best effort; the per-test namespace cleanup (testingk8s.Namespace) is
		// the real backstop.
		_ = runFuncErr(context.Background(), funcBin, funcDir, deployEnv, "delete", "--namespace", ns)
	})

	// A successful keda deploy only reports ready once the sidecar's readiness
	// probe passes, which requires the consumer to have joined the group and been
	// assigned partitions. So by now the consumer is live and -- because it reads
	// from the newest offset -- a record produced now will be seen.
	strimziImg := strimziKafkaImage(t, ctx, cliSet)
	produceRecord(t, ctx, cliSet, strimziImg, "hello from python e2e")

	// Assert the Python function received the CloudEvent.
	waitForFunctionLog(t, ctx, cliSet, ns, name, "Request Received")

	// Assert the consumer group drains to lag 0 (offset committed => 2xx
	// delivery confirmed).
	waitForZeroLag(t, ctx, cliSet, strimziImg, group)
}

// buildFuncCLI compiles the func binary from the repo under test and returns its
// path.
func buildFuncCLI(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot determine source file path")
	}
	repoRoot := filepath.Join(filepath.Dir(thisFile), "..", "..")
	bin := filepath.Join(t.TempDir(), "func")

	cmd := exec.Command("go", "build", "-o", bin, "./cmd/func")
	cmd.Dir = repoRoot
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("building func CLI: %v\n%s", err, out)
	}
	return bin
}

// runFunc runs the func CLI and fails the test on error. extraEnv entries
// (KEY=VALUE) are appended to the inherited environment.
func runFunc(t *testing.T, ctx context.Context, bin, dir string, extraEnv []string, args ...string) {
	t.Helper()
	if err := runFuncErr(ctx, bin, dir, extraEnv, args...); err != nil {
		t.Fatalf("func %s failed: %v", strings.Join(args, " "), err)
	}
}

func runFuncErr(ctx context.Context, bin, dir string, extraEnv []string, args ...string) error {
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), extraEnv...)
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%w\n%s", err, buf.String())
	}
	return nil
}

// strimziKafkaImage returns the image the running broker uses, so the client
// tool pods below run the exact same Kafka version as the broker.
func strimziKafkaImage(t *testing.T, ctx context.Context, cliSet *kubernetes.Clientset) string {
	t.Helper()
	pods, err := cliSet.CoreV1().Pods(kafkaNamespace).List(ctx, metav1.ListOptions{
		LabelSelector: "strimzi.io/cluster=" + kafkaClusterName + ",strimzi.io/broker-role=true",
	})
	if err != nil || len(pods.Items) == 0 {
		// Fall back to the generic kafka-pool label if the role label is absent.
		pods, err = cliSet.CoreV1().Pods(kafkaNamespace).List(ctx, metav1.ListOptions{
			LabelSelector: "strimzi.io/cluster=" + kafkaClusterName + ",strimzi.io/kind=Kafka",
		})
	}
	if err != nil {
		t.Fatalf("listing broker pods: %v", err)
	}
	if len(pods.Items) == 0 {
		t.Fatal("no Strimzi broker pod found (is Kafka installed? FUNC_CLUSTER_KAFKA=true)")
	}
	for _, c := range pods.Items[0].Spec.Containers {
		if c.Name == "kafka" {
			return c.Image
		}
	}
	// Single-container broker pod: use the first container's image.
	return pods.Items[0].Spec.Containers[0].Image
}

// clientPropsScript emits the shell that writes an SASL_SSL client.properties to
// /tmp/client.properties, authenticating as test-kafka-user with the password in
// $PASSWORD and trusting the broker CA mounted (PEM) at /opt/ca/ca.crt.
const clientPropsScript = `cat > /tmp/client.properties <<EOF
security.protocol=SASL_SSL
sasl.mechanism=SCRAM-SHA-512
sasl.jaas.config=org.apache.kafka.common.security.scram.ScramLoginModule required username="test-kafka-user" password="$PASSWORD";
ssl.truststore.type=PEM
ssl.truststore.location=/opt/ca/ca.crt
EOF
`

// produceRecord produces a single record to test-topic over SASL_SSL by running
// a short-lived pod with the broker's own Kafka image.
func produceRecord(t *testing.T, ctx context.Context, cliSet *kubernetes.Clientset, image, message string) {
	t.Helper()
	script := clientPropsScript +
		fmt.Sprintf("echo %q | /opt/kafka/bin/kafka-console-producer.sh "+
			"--bootstrap-server %s --topic test-topic "+
			"--producer.config /tmp/client.properties\n", message, kafkaBootstrap)
	out := runKafkaToolPod(t, ctx, cliSet, "kprod-"+rand.String(5), image, script)
	t.Logf("producer output:\n%s", out)
}

// waitForZeroLag polls the consumer group until every partition reports lag 0.
func waitForZeroLag(t *testing.T, ctx context.Context, cliSet *kubernetes.Clientset, image, group string) {
	t.Helper()
	script := clientPropsScript +
		fmt.Sprintf("/opt/kafka/bin/kafka-consumer-groups.sh --bootstrap-server %s "+
			"--command-config /tmp/client.properties --describe --group %s\n", kafkaBootstrap, group)

	var last string
	err := wait.PollUntilContextTimeout(ctx, time.Second*5, time.Minute*3, true, func(ctx context.Context) (bool, error) {
		last = runKafkaToolPod(t, ctx, cliSet, "klag-"+rand.String(5), image, script)
		return consumerGroupDrained(last), nil
	})
	if err != nil {
		t.Errorf("consumer group %s did not drain to lag 0:\n%s", group, last)
	}
}

// consumerGroupDrained reports whether the kafka-consumer-groups --describe
// output shows a row for test-topic with a committed offset and lag 0. A missing
// or "-" lag (no committed offset yet, or rebalancing) is treated as not-drained.
func consumerGroupDrained(describe string) bool {
	sawTopicRow := false
	for _, line := range strings.Split(describe, "\n") {
		fields := strings.Fields(line)
		// Columns: GROUP TOPIC PARTITION CURRENT-OFFSET LOG-END-OFFSET LAG ...
		if len(fields) < 6 || fields[1] != "test-topic" {
			continue
		}
		sawTopicRow = true
		if fields[5] != "0" {
			return false
		}
	}
	return sawTopicRow
}

// runKafkaToolPod runs script in a one-shot pod using the given Kafka image, with
// the broker CA mounted at /opt/ca and the SASL password in $PASSWORD, waits for
// it to finish, returns its logs, and deletes it.
func runKafkaToolPod(t *testing.T, ctx context.Context, cliSet *kubernetes.Clientset, podName, image, script string) string {
	t.Helper()
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: podName, Namespace: kafkaNamespace},
		Spec: corev1.PodSpec{
			RestartPolicy: corev1.RestartPolicyNever,
			Containers: []corev1.Container{{
				Name:    "kafka",
				Image:   image,
				Command: []string{"sh", "-c", script},
				Env: []corev1.EnvVar{{
					Name: "PASSWORD",
					ValueFrom: &corev1.EnvVarSource{
						SecretKeyRef: &corev1.SecretKeySelector{
							LocalObjectReference: corev1.LocalObjectReference{Name: kafkaUserSecretName},
							Key:                  "password",
						},
					},
				}},
				VolumeMounts: []corev1.VolumeMount{{
					Name:      "ca",
					MountPath: "/opt/ca",
					ReadOnly:  true,
				}},
			}},
			Volumes: []corev1.Volume{{
				Name: "ca",
				VolumeSource: corev1.VolumeSource{
					Secret: &corev1.SecretVolumeSource{SecretName: kafkaCASecretName},
				},
			}},
		},
	}
	if _, err := cliSet.CoreV1().Pods(kafkaNamespace).Create(ctx, pod, metav1.CreateOptions{}); err != nil {
		t.Fatalf("creating tool pod %s: %v", podName, err)
	}
	defer func() {
		_ = cliSet.CoreV1().Pods(kafkaNamespace).Delete(context.Background(), podName, metav1.DeleteOptions{})
	}()

	err := wait.PollUntilContextTimeout(ctx, time.Second*2, time.Minute*2, true, func(ctx context.Context) (bool, error) {
		p, err := cliSet.CoreV1().Pods(kafkaNamespace).Get(ctx, podName, metav1.GetOptions{})
		if err != nil {
			return false, err
		}
		return p.Status.Phase == corev1.PodSucceeded || p.Status.Phase == corev1.PodFailed, nil
	})
	logs := podLogs(ctx, cliSet, kafkaNamespace, podName, "")
	if err != nil {
		t.Fatalf("tool pod %s did not complete: %v\n%s", podName, err, logs)
	}
	return logs
}

// waitForFunctionLog polls the function's user-container logs until substr
// appears.
func waitForFunctionLog(t *testing.T, ctx context.Context, cliSet *kubernetes.Clientset, ns, deployment, substr string) {
	t.Helper()
	var last string
	err := wait.PollUntilContextTimeout(ctx, time.Second*3, time.Minute*3, true, func(ctx context.Context) (bool, error) {
		pods, err := cliSet.CoreV1().Pods(ns).List(ctx, metav1.ListOptions{})
		if err != nil {
			return false, err
		}
		for _, p := range pods.Items {
			if !strings.HasPrefix(p.Name, deployment) {
				continue
			}
			last = podLogs(ctx, cliSet, ns, p.Name, "user-container")
			if strings.Contains(last, substr) {
				return true, nil
			}
		}
		return false, nil
	})
	if err != nil {
		t.Fatalf("function did not log %q (delivery not observed):\n%s", substr, last)
	}
}

func podLogs(ctx context.Context, cliSet *kubernetes.Clientset, ns, pod, container string) string {
	opts := &corev1.PodLogOptions{}
	if container != "" {
		opts.Container = container
	}
	b, err := cliSet.CoreV1().Pods(ns).GetLogs(pod, opts).DoRaw(ctx)
	if err != nil {
		return fmt.Sprintf("<error reading logs for %s/%s: %v>", ns, pod, err)
	}
	return string(b)
}
