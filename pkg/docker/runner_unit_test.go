package docker

import (
	"testing"

	fn "knative.dev/func/pkg/functions"
)

func TestNewContainerConfig_KafkaTLSSASL(t *testing.T) {
	f := fn.Function{
		Build: fn.BuildSpec{Image: "example.com/image:latest"},
		Run: fn.RunSpec{
			Kafka: &fn.KafkaConfig{
				Brokers:          "broker:9093",
				Topic:            "my-topic",
				ConsumerGroup:    "my-group",
				SecurityProtocol: "SASL_SSL",
				TLS: &fn.KafkaTLS{
					CACert:     "/ca.crt",
					ClientCert: "/client.crt",
					ClientKey:  "/client.key",
					SkipVerify: true,
				},
				SASL: &fn.KafkaSASL{
					Mechanism: "SCRAM-SHA-512",
					User:      "my-user",
					Password:  "my-pass",
				},
			},
		},
	}
	c, err := newContainerConfig(f, "", false)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"FUNC_TRANSPORT":          "kafka",
		"KAFKA_BROKERS":           "broker:9093",
		"KAFKA_TOPIC":             "my-topic",
		"KAFKA_CONSUMER_GROUP":    "my-group",
		"KAFKA_SECURITY_PROTOCOL": "SASL_SSL",
		"KAFKA_TLS_CA_CERT":       "/ca.crt",
		"KAFKA_TLS_CLIENT_CERT":   "/client.crt",
		"KAFKA_TLS_CLIENT_KEY":    "/client.key",
		"KAFKA_TLS_SKIP_VERIFY":   "true",
		"KAFKA_SASL_MECHANISM":    "SCRAM-SHA-512",
		"KAFKA_SASL_USER":         "my-user",
		"KAFKA_SASL_PASSWORD":     "my-pass",
	}
	envMap := make(map[string]string, len(c.Env))
	for _, e := range c.Env {
		for i := range e {
			if e[i] == '=' {
				envMap[e[:i]] = e[i+1:]
				break
			}
		}
	}
	for key, val := range want {
		if got, ok := envMap[key]; !ok {
			t.Errorf("expected %s=%s in env, not found", key, val)
		} else if got != val {
			t.Errorf("expected %s=%s, got %s=%s", key, val, key, got)
		}
	}
}

func TestNewContainerConfig_KafkaTLSSkipVerifyFalse(t *testing.T) {
	f := fn.Function{
		Build: fn.BuildSpec{Image: "example.com/image:latest"},
		Run: fn.RunSpec{
			Kafka: &fn.KafkaConfig{
				Brokers:          "broker:9093",
				Topic:            "my-topic",
				ConsumerGroup:    "my-group",
				SecurityProtocol: "SSL",
				TLS:              &fn.KafkaTLS{CACert: "/ca.crt"},
			},
		},
	}
	c, err := newContainerConfig(f, "", false)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, e := range c.Env {
		if e == "KAFKA_TLS_SKIP_VERIFY=false" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected KAFKA_TLS_SKIP_VERIFY=false when TLS block present with SkipVerify unset")
	}
}
