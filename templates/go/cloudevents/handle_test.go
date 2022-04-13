package function

import (
	"context"
	"encoding/json"
	"testing"

	cloudevents "github.com/cloudevents/sdk-go/v2"

	"github.com/cloudevents/sdk-go/v2/event"
)

// TestHandle ensures that Handle accepts a valid CloudEvent without error.
func TestHandle(t *testing.T) {
	// A minimal, but valid, event.
	event := event.New()
	event.SetID("TEST-EVENT-01")
	event.SetType("MyEvent")
	event.SetSource("http://localhost:8080/")
	event.SetSubject("Echo")
	input := Echo{Message: "hello"}
	event.SetData(cloudevents.ApplicationJSON, input)
	// Invoke the defined handler.
	ce, err := Handle(context.Background(), event)
	if err != nil {
		t.Fatal(err)
	}

	if ce == nil {
		t.Errorf("The output CloudEvent cannot be nil")
	}
	if ce.Type() != "Echo" {
		t.Errorf("Wrong CloudEvent Type received: %v , expected Echo", ce.Type())
	}
	var output Echo
	err = json.Unmarshal(ce.Data(), &output)
	if err != nil {
		t.Fatal(err)
	}
	if expected := "echo " + input.Message; output.Message != expected {
		t.Errorf("The expected output should be: %v, and it was: %v", expected, output.Message)
	}

}
