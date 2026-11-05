package functions

import (
	"errors"
	"testing"
)

// TestGetRunFuncErrors ensures that known runtimes which do not yet
// have their runner implemented return a "not yet available" message, as
// distinct from unrecognized runtimes which state as much.
func TestGetRunFuncErrors(t *testing.T) {
	tests := []struct {
		Runtime    string
		ExpectedIs error
		ExpectedAs any
	}{
		{"", ErrRuntimeRequired, nil},
		{"go", nil, nil},
		{"python", nil, nil},
		{"rust", nil, &ErrRunnerNotImplemented{}},
		{"node", nil, &ErrRunnerNotImplemented{}},
		{"typescript", nil, &ErrRunnerNotImplemented{}},
		{"quarkus", nil, &ErrRunnerNotImplemented{}},
		{"springboot", nil, &ErrRunnerNotImplemented{}},
		{"other", nil, &ErrRuntimeNotRecognized{}},
	}
	for _, test := range tests {
		t.Run(test.Runtime, func(t *testing.T) {

			ctx := t.Context()
			job := Job{Function: Function{Runtime: test.Runtime}}
			_, err := getRunFunc(ctx, &job)

			if test.ExpectedAs != nil && !errors.As(err, test.ExpectedAs) {
				t.Fatalf("did not receive expected error type for %v runtime.", test.Runtime)
			}
			t.Logf("ok: %v", err)
		})
	}
}

// envHas reports whether env contains an exact "key=value" entry.
func envHas(env []string, kv string) bool {
	for _, e := range env {
		if e == kv {
			return true
		}
	}
	return false
}

// TestKafkaAdapterEnv_Config verifies the KAFKA_* config plus the target/health
// wiring is placed on the adapter sub-process env, and that FUNC_TRANSPORT (the
// retired in-process selector) is gone.
func TestKafkaAdapterEnv_Config(t *testing.T) {
	job := &Job{
		Host: "127.0.0.1",
		Port: "8080",
		Function: Function{
			Root:    t.TempDir(),
			Runtime: "go",
			Run: RunSpec{
				Kafka: &KafkaConfig{
					Brokers:       "broker1:9092",
					Topic:         "my-topic",
					ConsumerGroup: "my-group",
				},
			},
		},
	}
	env := kafkaAdapterEnv(job, "127.0.0.1:8081")
	want := map[string]string{
		"FUNCTION_TARGET":      "http://127.0.0.1:8080/",
		"LISTEN_ADDRESS":       "127.0.0.1:8081",
		"KAFKA_BROKERS":        "broker1:9092",
		"KAFKA_TOPIC":          "my-topic",
		"KAFKA_CONSUMER_GROUP": "my-group",
	}
	for key, val := range want {
		if !envHas(env, key+"="+val) {
			t.Errorf("expected %s=%s in adapter env, not found", key, val)
		}
	}
	for _, e := range env {
		if e == "FUNC_TRANSPORT=kafka" {
			t.Error("FUNC_TRANSPORT should no longer be set; the in-process path is retired")
		}
	}
}

func TestBuildRunnerEnv_NoKafka(t *testing.T) {
	t.Setenv("FUNC_TRANSPORT", "inherited")
	job := &Job{
		Function: Function{
			Root:    t.TempDir(),
			Runtime: "go",
		},
	}
	env, err := buildRunnerEnv(job, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range env {
		if e == "FUNC_TRANSPORT=kafka" {
			t.Error("FUNC_TRANSPORT should not be set to kafka when Kafka is nil")
		}
	}
}

func TestBuildRunnerEnv_KafkaIncomplete(t *testing.T) {
	t.Setenv("FUNC_TRANSPORT", "inherited")
	job := &Job{
		Function: Function{
			Root:    t.TempDir(),
			Runtime: "go",
			Run: RunSpec{
				Kafka: &KafkaConfig{
					Brokers:       "broker1:9092",
					Topic:         "",
					ConsumerGroup: "my-group",
				},
			},
		},
	}
	env, err := buildRunnerEnv(job, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range env {
		if e == "FUNC_TRANSPORT=kafka" {
			t.Error("FUNC_TRANSPORT should not be set to kafka when Kafka topic is empty")
		}
	}
}

func TestKafkaAdapterEnv_TLSSASL(t *testing.T) {
	job := &Job{
		Host: "127.0.0.1",
		Port: "8080",
		Function: Function{
			Root:    t.TempDir(),
			Runtime: "go",
			Run: RunSpec{
				Kafka: &KafkaConfig{
					Brokers:          "broker:9093",
					Topic:            "my-topic",
					ConsumerGroup:    "my-group",
					SecurityProtocol: "SASL_SSL",
					TLS: &KafkaTLS{
						CACert:     "/ca.crt",
						ClientCert: "/client.crt",
						ClientKey:  "/client.key",
						SkipVerify: true,
					},
					SASL: &KafkaSASL{
						Mechanism: "SCRAM-SHA-512",
						User:      "my-user",
						Password:  "my-pass",
					},
				},
			},
		},
	}
	env := kafkaAdapterEnv(job, "127.0.0.1:8081")
	want := map[string]string{
		"KAFKA_SECURITY_PROTOCOL": "SASL_SSL",
		"KAFKA_TLS_CA_CERT":       "/ca.crt",
		"KAFKA_TLS_CLIENT_CERT":   "/client.crt",
		"KAFKA_TLS_CLIENT_KEY":    "/client.key",
		"KAFKA_TLS_SKIP_VERIFY":   "true",
		"KAFKA_SASL_MECHANISM":    "SCRAM-SHA-512",
		"KAFKA_SASL_USER":         "my-user",
		"KAFKA_SASL_PASSWORD":     "my-pass",
	}
	for key, val := range want {
		if !envHas(env, key+"="+val) {
			t.Errorf("expected %s=%s in adapter env, not found", key, val)
		}
	}
}

func TestKafkaAdapterEnv_TLSSkipVerifyFalse(t *testing.T) {
	job := &Job{
		Host: "127.0.0.1",
		Port: "8080",
		Function: Function{
			Root:    t.TempDir(),
			Runtime: "go",
			Run: RunSpec{
				Kafka: &KafkaConfig{
					Brokers:          "broker:9093",
					Topic:            "my-topic",
					ConsumerGroup:    "my-group",
					SecurityProtocol: "SSL",
					TLS:              &KafkaTLS{CACert: "/ca.crt"},
				},
			},
		},
	}
	env := kafkaAdapterEnv(job, "127.0.0.1:8081")
	if !envHas(env, "KAFKA_TLS_SKIP_VERIFY=false") {
		t.Error("expected KAFKA_TLS_SKIP_VERIFY=false when TLS block present with SkipVerify unset")
	}
}

// TestKafkaAdapterEnv_EmptyMechanism verifies an unset SASL mechanism still
// wires KAFKA_SASL_MECHANISM=PLAIN into the adapter env, matching the value the
// keda scaler derives from EffectiveMechanism so the two authenticate the same
// way.
func TestKafkaAdapterEnv_EmptyMechanism(t *testing.T) {
	job := &Job{
		Host: "127.0.0.1",
		Port: "8080",
		Function: Function{
			Root:    t.TempDir(),
			Runtime: "go",
			Run: RunSpec{
				Kafka: &KafkaConfig{
					Brokers:          "broker:9093",
					Topic:            "my-topic",
					ConsumerGroup:    "my-group",
					SecurityProtocol: "SASL_SSL",
					SASL:             &KafkaSASL{User: "u", Password: "p"},
				},
			},
		},
	}
	env := kafkaAdapterEnv(job, "127.0.0.1:8081")
	if !envHas(env, "KAFKA_SASL_MECHANISM=PLAIN") {
		t.Error("expected KAFKA_SASL_MECHANISM=PLAIN when SASL block present with mechanism unset")
	}
}
