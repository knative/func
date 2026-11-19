package docker

import (
	"strings"
	"testing"

	fn "knative.dev/func/pkg/functions"
)

// TestNewContainerConfig_NoKafkaEnv ensures the function container carries no
// Kafka configuration, even with run.kafka fully configured: a local run
// consumes Kafka through the Kafka runtime started next to the function (see
// functions.Client.Run), which delivers records to it over HTTP, as the
// sidecar does on the cluster. The FUNC_TRANSPORT=kafka selector of the retired
// in-process consumer must not be set either.
func TestNewContainerConfig_NoKafkaEnv(t *testing.T) {
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
	for _, e := range c.Env {
		if strings.HasPrefix(e, "KAFKA_") || strings.HasPrefix(e, "FUNC_TRANSPORT=") {
			t.Errorf("unexpected Kafka configuration on the function container: %v", e)
		}
	}
}
