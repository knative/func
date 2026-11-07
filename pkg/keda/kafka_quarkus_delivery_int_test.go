//go:build integration_kafka

package keda_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/util/rand"
	"k8s.io/client-go/kubernetes"

	fn "knative.dev/func/pkg/functions"
	"knative.dev/func/pkg/k8s"
	testingk8s "knative.dev/func/pkg/testing/k8s"
)

// TestInt_KafkaQuarkusDelivery proves the language-agnostic claim for
// Quarkus/Java: a plain Quarkus Funqy cloudevents function, built and deployed
// through func on the keda deployer, receives Kafka records via the injected
// kafka-runtime sidecar as CloudEvents over HTTP -- the function itself contains
// no Kafka code.
//
// It mirrors TestInt_KafkaPythonDelivery but additionally covers both delivery
// shapes the sidecar can emit, since Funqy binds data by type (unlike Python's
// untyped get_data()):
//  1. func create -l quarkus -t cloudevents, replace the handler with a fixture
//     that declares two @Funq methods routed by CloudEvent type, add run.kafka +
//     a kafka KEDA trigger
//  2. func deploy with the pack builder (the host builder does not support
//     Quarkus) -- builds the JVM image, pushes to the in-cluster registry,
//     injects the sidecar -- against the real Strimzi broker
//  3. produce two records to test-topic, each carrying a unique token:
//     - a plain record -> sidecar wraps it as type dev.knative.kafka.event
//     with application/octet-stream data -> handler's raw(byte[]) echoes it
//     - a record with ce_* headers + JSON body -> sidecar passes it through ->
//     handler's typed(Input) binds the POJO and echoes the message
//  4. assert the Quarkus user-container logged both exact tokens (the fixture
//     echoes the received payloads, so this confirms our specific records were
//     delivered and decoded intact -- not merely that requests arrived)
//  5. assert the consumer group drains to lag 0 (the sidecar commits the offset
//     only on a 2xx from the function, so lag 0 independently confirms the
//     function acked both deliveries)
//
// Environment is identical to TestInt_KafkaPythonDelivery (the slim Kafka
// integration-tests cluster: Strimzi broker with TLS + SCRAM-SHA-512 on 9093,
// KEDA, the in-cluster registry mirror). Quarkus builds take noticeably longer
// than Python, hence the wider timeout.
//
// First-run risk: Funqy binding of raw application/octet-stream data to byte[]
// (the raw() path) is the part not verifiable offline; if the broker/sidecar/
// Funqy versions handle octet-stream differently, this is where it surfaces.
// Redelivery-on-failure is intentionally out of scope (see the Python test).
func TestInt_KafkaQuarkusDelivery(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute*35)
	t.Cleanup(cancel)

	name := "func-int-kafka-qk-" + rand.String(5)
	group := name + "-group"
	// Unique per-run tokens, one per delivery shape, carried in the record
	// payloads. The handler fixture echoes what it received, so finding each
	// token in the function's logs proves it received *our* specific record
	// (payload intact) over that path, not merely that some request arrived.
	rawToken := "raw-" + rand.String(10)
	typedToken := "typed-" + rand.String(10)
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

	// Build the func CLI from the repo under test, then scaffold a Quarkus
	// cloudevents function with it.
	funcBin := buildFuncCLI(t)
	// func create derives the function name from the directory's basename, and
	// t.TempDir()'s leaf is numeric (e.g. "002"), which is not a valid function
	// name. Nest the function in a validly-named subdirectory.
	funcDir := filepath.Join(t.TempDir(), name)
	if err := os.MkdirAll(funcDir, 0o755); err != nil {
		t.Fatalf("creating function dir: %v", err)
	}
	runFunc(t, ctx, funcBin, funcDir, nil, "create", "-l", "quarkus", "-t", "cloudevents", funcDir)

	// Replace the scaffolded handler (and the unit test that referenced its old
	// signature) with the echo fixture, so delivery of our specific records can
	// be verified. The stock handler only prints the whole event and returns OK.
	installQuarkusHandlerFixture(t, funcDir)

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

	// Build + push + deploy. Force the pack builder (the host builder does not
	// support Quarkus). Push to the in-cluster registry via localhost:5001, not
	// registry.localtest.me: the latter is only reachable through the Contour
	// ingress, which the slim Kafka cluster skips (no serving). hack/cluster.sh
	// maps host port 5001 to the node's registry for serving-less clusters (5001
	// rather than 5000 to avoid the macOS AirPlay receiver on :5000), and a
	// containerd mirror resolves localhost:5001 to the same registry so the node
	// can pull. Honor a FUNC_KAFKA_RUNTIME_IMAGE override if the caller set one.
	deployEnv := []string{
		"FUNC_REGISTRY=localhost:5001/func",
		"FUNC_REGISTRY_INSECURE=true",
	}
	if img := os.Getenv("FUNC_KAFKA_RUNTIME_IMAGE"); img != "" {
		deployEnv = append(deployEnv, "FUNC_KAFKA_RUNTIME_IMAGE="+img)
	}
	runFunc(t, ctx, funcBin, funcDir, deployEnv, "deploy", "--namespace", ns, "--builder", "pack", "--verbose")
	t.Cleanup(func() {
		// Best effort; the per-test namespace cleanup (testingk8s.Namespace) is
		// the real backstop.
		_ = runFuncErr(context.Background(), funcBin, funcDir, deployEnv, "delete", "--namespace", ns)
	})

	// A successful keda deploy only reports ready once the sidecar's readiness
	// probe passes, which requires the consumer to have joined the group and been
	// assigned partitions. So by now the consumer is live and -- because it reads
	// from the newest offset -- records produced now will be seen.
	strimziImg := strimziKafkaImage(t, ctx, cliSet)

	// Path 1: a plain record. The sidecar wraps it as type dev.knative.kafka.event
	// with octet-stream data, routed to the handler's raw(byte[]) method.
	produceRecord(t, ctx, cliSet, strimziImg, "payload-"+rawToken)

	// Path 2: a CloudEvent-formatted record (ce_* headers + JSON body). The
	// sidecar passes it through with its own type, routed to typed(Input).
	typedHeaders := "ce_specversion:1.0,ce_id:" + typedToken +
		",ce_source:func-e2e,ce_type:e2e.kafka.typed,content-type:application/json"
	typedBody := `{"message":"payload-` + typedToken + `"}`
	produceRecordWithHeaders(t, ctx, cliSet, strimziImg, typedHeaders, typedBody)

	// Assert the Quarkus function received both specific records: the fixture
	// echoes each payload, so the unique tokens appear in its logs only if the
	// records were delivered and decoded intact over their respective paths.
	waitForFunctionLog(t, ctx, cliSet, ns, name, rawToken)
	waitForFunctionLog(t, ctx, cliSet, ns, name, typedToken)

	// Assert the consumer group drains to lag 0 (offsets committed => both 2xx
	// deliveries confirmed).
	waitForZeroLag(t, ctx, cliSet, strimziImg, group)
}

// installQuarkusHandlerFixture overwrites the scaffolded Quarkus handler, its
// application.properties, and its unit test with the committed fixtures under
// testdata/kafka_quarkus_delivery. The handler echoes received payloads (so the
// test can assert exact delivery); the properties drop the single-function
// export so the two @Funq methods route by CloudEvent type; the unit test is a
// placeholder that keeps src/test compiling after the handler is replaced
// (NativeFunctionIT extends it). func create still scaffolds everything else
// (pom.xml, Input.java, deps).
func installQuarkusHandlerFixture(t *testing.T, funcDir string) {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot determine source file path")
	}
	fixtureDir := filepath.Join(filepath.Dir(thisFile), "testdata", "kafka_quarkus_delivery")

	copies := map[string]string{
		"Function.java":          filepath.Join("src", "main", "java", "functions", "Function.java"),
		"application.properties": filepath.Join("src", "main", "resources", "application.properties"),
		"FunctionTest.java":      filepath.Join("src", "test", "java", "functions", "FunctionTest.java"),
	}
	for fixture, rel := range copies {
		b, err := os.ReadFile(filepath.Join(fixtureDir, fixture))
		if err != nil {
			t.Fatalf("reading fixture %s: %v", fixture, err)
		}
		dst := filepath.Join(funcDir, rel)
		if err := os.WriteFile(dst, b, 0o644); err != nil {
			t.Fatalf("installing fixture %s into scaffold: %v", fixture, err)
		}
	}
}

// produceRecordWithHeaders produces a single record to test-topic over SASL_SSL
// with the given Kafka record headers (comma-separated key:value pairs, per the
// CloudEvents Kafka protocol binding, e.g. "ce_type:...,content-type:...") and
// value, by running a short-lived pod with the broker's own Kafka image. The
// header block and the value are separated by a tab (the console producer's
// default headers delimiter), so colons/commas in the JSON value are not parsed
// as header separators.
func produceRecordWithHeaders(t *testing.T, ctx context.Context, cliSet *kubernetes.Clientset, image, headers, value string) {
	t.Helper()
	script := clientPropsScript +
		fmt.Sprintf("printf '%%s\\t%%s\\n' '%s' '%s' | /opt/kafka/bin/kafka-console-producer.sh "+
			"--bootstrap-server %s --topic test-topic "+
			"--producer.config /tmp/client.properties --property parse.headers=true\n",
			headers, value, kafkaBootstrap)
	out := runKafkaToolPod(t, ctx, cliSet, "kprodh-"+rand.String(5), image, script)
	t.Logf("producer(headers) output:\n%s", out)
}
