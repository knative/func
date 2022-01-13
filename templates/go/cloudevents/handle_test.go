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
