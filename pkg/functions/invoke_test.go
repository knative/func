package functions

import (
	"context"
	"os"
	"testing"

	"github.com/segmentio/kafka-go"
)

type mockKafkaWriter struct {
	writeMessagesFunc func(ctx context.Context, msgs ...kafka.Message) error
	closeFunc         func() error
}

func (m *mockKafkaWriter) WriteMessages(ctx context.Context, msgs ...kafka.Message) error {
	if m.writeMessagesFunc != nil {
		return m.writeMessagesFunc(ctx, msgs...)
	}
	return nil
}

func (m *mockKafkaWriter) Close() error {
	if m.closeFunc != nil {
		return m.closeFunc()
	}
	return nil
}

func TestInvokeKafka(t *testing.T) {
	// Setup test directories/files for a mock function
	tempDir, err := os.MkdirTemp("", "func-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	client := New()

	// Initialize function
	f, err := client.Init(Function{
		Root:    tempDir,
		Runtime: "go",
		Name:    "my-kafka-func",
	})
	if err != nil {
		t.Fatal(err)
	}

	// Add KAFKA_BROKERS and KAFKA_TOPICS to function environment
	f.Run.Envs.Add("KAFKA_BROKERS", "localhost:9092,localhost:9093")
	f.Run.Envs.Add("KAFKA_TOPICS", "test-topic-1,test-topic-2")
	f.Invoke = "kafka"
	if err := f.Write(); err != nil {
		t.Fatal(err)
	}

	// Mock newKafkaWriter
	var writtenMessages []kafka.Message
	var closedCount int
	origNewKafkaWriter := newKafkaWriter
	t.Cleanup(func() {
		newKafkaWriter = origNewKafkaWriter
	})

	newKafkaWriter = func(brokers []string, topic string) kafkaWriter {
		return &mockKafkaWriter{
			writeMessagesFunc: func(ctx context.Context, msgs ...kafka.Message) error {
				writtenMessages = append(writtenMessages, msgs...)
				return nil
			},
			closeFunc: func() error {
				closedCount++
				return nil
			},
		}
	}

	m := InvokeMessage{
		Data: []byte("test kafka message"),
	}

	metadata, body, err := client.Invoke(context.Background(), tempDir, "", m)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	if metadata != nil {
		t.Errorf("expected nil metadata, got %v", metadata)
	}

	expectedBody := "Message sent to Kafka topic(s): test-topic-1,test-topic-2\n"
	if body != expectedBody {
		t.Errorf("expected body %q, got %q", expectedBody, body)
	}

	if len(writtenMessages) != 2 {
		t.Errorf("expected 2 messages to be written (one for each topic), got %d", len(writtenMessages))
	}

	for _, msg := range writtenMessages {
		if string(msg.Value) != "test kafka message" {
			t.Errorf("expected message value 'test kafka message', got %q", string(msg.Value))
		}
	}

	if closedCount != 2 {
		t.Errorf("expected close to be called 2 times, got %d", closedCount)
	}
}

func TestInvokeKafkaMissingConfig(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "func-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	client := New()

	// Initialize function
	f, err := client.Init(Function{
		Root:    tempDir,
		Runtime: "go",
		Name:    "my-kafka-func-missing",
	})
	if err != nil {
		t.Fatal(err)
	}

	f.Invoke = "kafka"
	if err := f.Write(); err != nil {
		t.Fatal(err)
	}

	m := InvokeMessage{
		Data: []byte("test kafka message"),
	}

	_, _, err = client.Invoke(context.Background(), tempDir, "", m)
	if err == nil {
		t.Fatal("expected error due to missing Kafka configuration, got nil")
	}
}
