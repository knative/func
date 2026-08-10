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

func TestBuildRunnerEnv_KafkaConfig(t *testing.T) {
	job := &Job{
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
	env, err := buildRunnerEnv(job, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"FUNC_TRANSPORT":       "kafka",
		"KAFKA_BROKERS":        "broker1:9092",
		"KAFKA_TOPIC":          "my-topic",
		"KAFKA_CONSUMER_GROUP": "my-group",
	}
	for key, val := range want {
		found := false
		for _, e := range env {
			if e == key+"="+val {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected %s=%s in env, not found", key, val)
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
