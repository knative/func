package function

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/cloudevents/sdk-go/v2/event"

	cloudevents "github.com/cloudevents/sdk-go/v2"
)

// Handle an event.
func Handle(ctx context.Context, event cloudevents.Event) (*event.Event, error) {

	/*
	 * YOUR CODE HERE
	 *
	 * Try running `go test`.  Add more test as you code in `handle_test.go`.
	 */

	fmt.Printf("Incoming Event: %v\n", event) // print the received event to standard output
	payload := ""
	err := json.Unmarshal(event.Data(), &payload)
	if err != nil {
		fmt.Printf("%v\n", err)
		return nil, err
	}

	payload = "echo " + payload
	outputEvent := cloudevents.NewEvent()
	outputEvent.SetSource("http://example.com/echo")
	outputEvent.SetType("Echo")
	outputEvent.SetData(cloudevents.ApplicationJSON, &payload)

	fmt.Printf("Outgoing Event: %v\n", outputEvent)

	return &outputEvent, nil
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
