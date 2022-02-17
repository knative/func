package function

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/cloudevents/sdk-go/v2/event"

	cloudevents "github.com/cloudevents/sdk-go/v2"
)

type Output struct {
	Output    string `json:"output"`
	Input     string `json:"input"`
	Operation string `json:"operation"`
}

type Input struct {
	Input string `json:"input"`
}

func uppercase(event cloudevents.Event) (*event.Event, error) {
	input := Input{}
	err := json.Unmarshal(event.Data(), &input)
	if err != nil {
		fmt.Printf("%v\n", err)
		return nil, err
	}
	fmt.Printf("%v\n", input)
	outputEvent := cloudevents.NewEvent()
	outputEvent.SetSource("http://example.com/uppercase")
	outputEvent.SetType("UpperCasedEvent")
	output := Output{}
	output.Input = input.Input
	output.Output = strings.ToUpper(input.Input)
	output.Operation = event.Subject()
	outputEvent.SetData(cloudevents.ApplicationJSON, &output)

	fmt.Printf("Outgoing Event: %v\n", outputEvent)

	return &outputEvent, nil
}

// Handle an event.
func Handle(ctx context.Context, event cloudevents.Event) (*event.Event, error) {

	// Example implementation:
	fmt.Printf("Incoming Event: %v\n", event) // print the received event to standard output
	if event.Type() == "uppercase" {
		return uppercase(event)
	}
	return nil, errors.New("No action for event type: " + event.Type())
}

/*
Other supported function signatures:

	Handle()
	Handle() error
	Handle(context.Context)
	Handle(context.Context) error
	Handle(event.Event)
	Handle(event.Event) error
	Handle(context.Context, event.Event)
	Handle(context.Context, event.Event) error
	Handle(event.Event) *event.Event
	Handle(event.Event) (*event.Event, error)
	Handle(context.Context, event.Event) *event.Event
	Handle(context.Context, event.Event) (*event.Event, error)

*/
