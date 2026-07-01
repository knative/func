package function

import (
	"context"
	"testing"

	"knative.dev/func-go/kafka"
)

// TestHandle ensures that Handle accepts a valid Kafka message without error.
func TestHandle(t *testing.T) {
	msg := kafka.Message{
		Key:   []byte("test-key"),
		Value: []byte("test-value"),
		Topic: "test-topic",
	}

	err := Handle(context.Background(), msg)
	if err != nil {
		t.Fatal(err)
	}
}
