package function

import (
	"testing"

	"github.com/cloudevents/sdk-go/v2/event"
)

// TestHandle ensures that Handle accepts a valid CloudEvent without error.
func TestHandle(t *testing.T) {
	// Assemble
	e := event.New()
	e.SetID("id")
	e.SetType("type")
	e.SetSource("source")
	e.SetData("text/plain", "data")

	// Act
	data, err := New().Handle(e)

	// Assert
	if err != nil {
		t.Errorf("didnt expect err, got: %v", err)
	}
	if data == nil {
		t.Errorf("received nil event") // fail on nil
	}
	if string(data.Data()) != `{"message":"OK"}` {
		t.Errorf("the received event expected data to be '{\"message\":\"OK\"}', got '%s'", data.Data())
	}
}
