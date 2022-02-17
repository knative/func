package function

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	cloudevents "github.com/cloudevents/sdk-go/v2"

	"github.com/cloudevents/sdk-go/v2/event"
)

// TestHandle ensures that Handle accepts a valid CloudEvent without error.
func TestHandle(t *testing.T) {
	// A minimal, but valid, event.
	event := event.New()
	event.SetID("TEST-EVENT-01")
	event.SetType("UppercaseRequestedEvent")
	event.SetSource("http://localhost:8080/")
	event.SetSubject("Convert to UpperCase")
	input := Input{}
	input.Input = "hello"
	event.SetData(cloudevents.ApplicationJSON, &input)
	// Invoke the defined handler.
	ce, err := Handle(context.Background(), event)
	if err != nil {
		t.Fatal(err)
	}

	if ce == nil {
		t.Errorf("The output CloudEvent cannot be nil")
	}
	if ce.Type() != "UpperCasedEvent" {
		t.Errorf("Wrong CloudEvent Type received: %v , expected UpperCasedEvent", ce.Type())
	}
	output := Output{}
	err = json.Unmarshal(ce.Data(), &output)
	if err != nil {
		t.Fatal(err)
	}
	if output.Operation != event.Subject() {
		t.Errorf("The output.Operation: %v should be the same as the input event Subject: %v", output.Operation, event.Subject())
	}
	if output.Input != input.Input {
		t.Errorf("The output.Input: %v should be the same as the input.Input: %v", output.Input, input.Input)
	}

	if output.Output != strings.ToUpper(input.Input) {
		t.Errorf("The output.Output: %v should be the same as the input.Input in uppercase: %v", output.Output, strings.ToUpper(input.Input))
	}
}
