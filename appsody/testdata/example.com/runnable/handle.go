package function

import (
	"context"
	"fmt"
	"os"

	cloudevents "github.com/cloudevents/sdk-go"
)

// Handle a CloudEvent.
// Supported function signatures:
//   func()
//   func() error
//   func(context.Context)
//   func(context.Context) error
//   func(cloudevents.Event)
//   func(cloudevents.Event) error
//   func(context.Context, cloudevents.Event)
//   func(context.Context, cloudevents.Event) error
//   func(cloudevents.Event, *cloudevents.EventResponse)
//   func(cloudevents.Event, *cloudevents.EventResponse) error
//   func(context.Context, cloudevents.Event, *cloudevents.EventResponse)
//   func(context.Context, cloudevents.Event, *cloudevents.EventResponse) error
func Handle(ctx context.Context, event cloudevents.Event) error {
	if err := event.Validate(); err != nil {
		fmt.Fprintf(os.Stderr, "invalid event received. %v", err)
		return err
	}
	fmt.Printf("%v\n", event)
	return nil
}
