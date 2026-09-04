package keda

import (
	"testing"

	v1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	fn "knative.dev/func/pkg/functions"
)

func TestTriggers_NoScale(t *testing.T) {
	f := fn.Function{Name: "test"}
	got := triggers(f)
	if got != nil {
		t.Errorf("expected nil, got %v", got)
	}
}

func TestTriggers_Explicit(t *testing.T) {
	lag := int64(5)
	f := fn.Function{
		Name: "test",
		Deploy: fn.DeploySpec{
			Options: fn.Options{
				Scale: &fn.ScaleOptions{
					KEDA: &fn.KEDAScaleOptions{
						Triggers: []fn.KEDATrigger{
							{Type: "kafka", LagThreshold: &lag},
						},
					},
				},
			},
		},
	}
	got := triggers(f)
	if len(got) != 1 {
		t.Fatalf("expected 1 trigger, got %d", len(got))
	}
	if got[0].Type != "kafka" {
		t.Errorf("expected kafka, got %s", got[0].Type)
	}
	if *got[0].LagThreshold != 5 {
		t.Errorf("expected lagThreshold 5, got %d", *got[0].LagThreshold)
	}
}

func TestParseSecretRef(t *testing.T) {
	tests := []struct {
		input    string
		wantName string
		wantKey  string
	}{
		{"{{ secret:my-secret:my-key }}", "my-secret", "my-key"},
		{"{{ secret:foo:bar }}", "foo", "bar"},
		{"plaintext-value", "", ""},
		{"{{ configMap:cm:key }}", "", ""},
		{"{{ invalid }}", "", ""},
	}
	for _, tt := range tests {
		name, key := parseSecretRef(tt.input)
		if name != tt.wantName || key != tt.wantKey {
			t.Errorf("parseSecretRef(%q) = (%q, %q), want (%q, %q)", tt.input, name, key, tt.wantName, tt.wantKey)
		}
	}
}

func TestFindSecretForPath(t *testing.T) {
	secret := "my-cluster-ca"
	path := "/etc/kafka/ca"
	volumes := []fn.Volume{
		{Secret: &secret, Path: &path},
	}

	name, key := findSecretForPath("/etc/kafka/ca/ca.crt", volumes)
	if name != "my-cluster-ca" || key != "ca.crt" {
		t.Errorf("got (%q, %q), want (my-cluster-ca, ca.crt)", name, key)
	}

	name, _ = findSecretForPath("/other/path", volumes)
	if name != "" {
		t.Errorf("expected empty for non-matching path, got %q", name)
	}
}

func testDeployment() *v1.Deployment {
	return &v1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-func",
			Namespace: "default",
			UID:       types.UID("test-uid-123"),
		},
		Spec: v1.DeploymentSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{Name: "user-container"}},
				},
			},
		},
	}
}

func TestBuildTriggerAuth(t *testing.T) {
	f := fn.Function{
		Name: "test-func",
		Run: fn.RunSpec{
			Kafka: &fn.KafkaConfig{
				Brokers:          "broker:9093",
				Topic:            "topic",
				ConsumerGroup:    "group",
				SecurityProtocol: "SASL_SSL",
				TLS: &fn.KafkaTLS{
					CACert: "/etc/kafka/ca/ca.crt",
				},
				SASL: &fn.KafkaSASL{
					Mechanism: "SCRAM-SHA-512",
					User:      "admin",
					Password:  "{{ secret:my-user:password }}",
				},
			},
			Volumes: []fn.Volume{
				{Secret: strPtr("my-cluster-ca"), Path: strPtr("/etc/kafka/ca")},
			},
		},
	}

	ta := buildTriggerAuth(f, testDeployment(), "default")
	if ta == nil {
		t.Fatal("expected TriggerAuthentication, got nil")
	}

	if ta.GetName() != "test-func-kafka-auth" {
		t.Errorf("name = %q, want test-func-kafka-auth", ta.GetName())
	}

	spec, ok := ta.Object["spec"].(map[string]interface{})
	if !ok {
		t.Fatal("missing spec")
	}
	refs, ok := spec["secretTargetRef"].([]interface{})
	if !ok {
		t.Fatal("missing secretTargetRef")
	}
	if len(refs) != 2 {
		t.Fatalf("expected 2 secretTargetRef entries, got %d", len(refs))
	}

	ref0 := refs[0].(map[string]interface{})
	if ref0["parameter"] != "password" || ref0["name"] != "my-user" || ref0["key"] != "password" {
		t.Errorf("unexpected password ref: %v", ref0)
	}

	ref1 := refs[1].(map[string]interface{})
	if ref1["parameter"] != "ca" || ref1["name"] != "my-cluster-ca" || ref1["key"] != "ca.crt" {
		t.Errorf("unexpected ca ref: %v", ref1)
	}

	envs, ok := spec["env"].([]interface{})
	if !ok {
		t.Fatal("missing env")
	}
	if len(envs) != 1 {
		t.Fatalf("expected 1 env entry, got %d", len(envs))
	}
	env0 := envs[0].(map[string]interface{})
	if env0["parameter"] != "username" || env0["name"] != "KAFKA_SASL_USER" {
		t.Errorf("unexpected env ref: %v", env0)
	}
}

func TestBuildScaledObject(t *testing.T) {
	lag := int64(20)
	f := fn.Function{
		Name: "test-func",
		Run: fn.RunSpec{
			Kafka: &fn.KafkaConfig{
				Brokers:          "broker:9093",
				Topic:            "my-topic",
				ConsumerGroup:    "my-group",
				SecurityProtocol: "SASL_SSL",
				TLS:              &fn.KafkaTLS{CACert: "/etc/kafka/ca/ca.crt"},
				SASL:             &fn.KafkaSASL{Mechanism: "SCRAM-SHA-512", Password: "{{ secret:s:k }}"},
			},
		},
	}
	trigger := fn.KEDATrigger{Type: "kafka", LagThreshold: &lag}

	so := buildScaledObject(f, trigger, testDeployment(), "default", 0, 10)
	if so == nil {
		t.Fatal("expected ScaledObject, got nil")
	}

	if so.GetName() != "test-func-kafka" {
		t.Errorf("name = %q, want test-func-kafka", so.GetName())
	}

	spec := so.Object["spec"].(map[string]interface{})
	if spec["minReplicaCount"] != int64(0) {
		t.Errorf("minReplicaCount = %v, want 0", spec["minReplicaCount"])
	}
	if spec["maxReplicaCount"] != int64(10) {
		t.Errorf("maxReplicaCount = %v, want 10", spec["maxReplicaCount"])
	}

	triggers := spec["triggers"].([]interface{})
	if len(triggers) != 1 {
		t.Fatalf("expected 1 trigger, got %d", len(triggers))
	}
	trigger0 := triggers[0].(map[string]interface{})
	meta := trigger0["metadata"].(map[string]interface{})
	if meta["bootstrapServers"] != "broker:9093" {
		t.Errorf("bootstrapServers = %v", meta["bootstrapServers"])
	}
	if meta["lagThreshold"] != "20" {
		t.Errorf("lagThreshold = %v, want 20", meta["lagThreshold"])
	}
	if meta["tls"] != "enable" {
		t.Errorf("tls = %v, want enable", meta["tls"])
	}
	if meta["sasl"] != "scram_sha512" {
		t.Errorf("sasl = %v, want scram_sha512", meta["sasl"])
	}

	authRef := trigger0["authenticationRef"].(map[string]interface{})
	if authRef["name"] != "test-func-kafka-auth" {
		t.Errorf("authenticationRef name = %v", authRef["name"])
	}
}

func TestBuildScaledObject_DefaultLag(t *testing.T) {
	f := fn.Function{
		Name: "test-func",
		Run: fn.RunSpec{
			Kafka: &fn.KafkaConfig{
				Brokers:       "broker:9092",
				Topic:         "t",
				ConsumerGroup: "g",
			},
		},
	}
	trigger := fn.KEDATrigger{Type: "kafka"}

	so := buildScaledObject(f, trigger, testDeployment(), "default", 1, 5)
	if so == nil {
		t.Fatal("expected ScaledObject, got nil")
	}

	spec := so.Object["spec"].(map[string]interface{})
	triggers := spec["triggers"].([]interface{})
	trigger0 := triggers[0].(map[string]interface{})
	meta := trigger0["metadata"].(map[string]interface{})
	if meta["lagThreshold"] != "10" {
		t.Errorf("default lagThreshold = %v, want 10", meta["lagThreshold"])
	}

	// No TLS/SASL, so no authenticationRef
	if _, ok := trigger0["authenticationRef"]; ok {
		t.Error("expected no authenticationRef for plaintext Kafka")
	}
}

func TestKedaSASLType(t *testing.T) {
	tests := map[string]string{
		"SCRAM-SHA-256": "scram_sha256",
		"SCRAM-SHA-512": "scram_sha512",
		"PLAIN":         "plain",
		"UNKNOWN":       "",
	}
	for in, want := range tests {
		if got := kedaSASLType(in); got != want {
			t.Errorf("kedaSASLType(%q) = %q, want %q", in, got, want)
		}
	}
}

func strPtr(s string) *string { return &s }
