package function

import (
	"context"
	"testing"

	"github.com/cloudevents/sdk-go/v2/event"
)

// TestHandle ensures that Handle accepts a valid CloudEvent without error.
func TestHandle(t *testing.T) {
	// A minimal, but valid, event.
	event := event.New()
	event.SetID("TEST-EVENT-01")
	event.SetType("com.example.event.test")
	event.SetSource("http://localhost:8080/")

	// Invoke the defined handler.
	if err := Handle(context.Background(), event); err != nil {
		t.Fatal(err)
	}
}

// TestHandleInvalid ensures that an invalid input event generates an error.
func TestInvalidInput(t *testing.T) {
	invalidEvent := event.New() // missing required fields

	// Attempt to handle the invalid event, ensuring that the handler validats events.
	if err := Handle(context.Background(), invalidEvent); err == nil {
		t.Fatalf("handler did not generate error on invalid event.  Missing .Validate() check?")
	}
}
