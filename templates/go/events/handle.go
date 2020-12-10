package function

import (
	"context"
	"fmt"
	"os"

	event "github.com/cloudevents/sdk-go/v2"
)

// Handle a CloudEvent.
// Supported Function signatures:
// * func()
// * func() error
// * func(context.Context)
// * func(context.Context) error
// * func(event.Event)
// * func(event.Event) error
// * func(context.Context, event.Event)
// * func(context.Context, event.Event) error
// * func(event.Event) *event.Event
// * func(event.Event) (*event.Event, error)
// * func(context.Context, event.Event) *event.Event
// * func(context.Context, event.Event) (*event.Event, error)
func Handle(ctx context.Context, event event.Event) error {
	if err := event.Validate(); err != nil {
		fmt.Fprintf(os.Stderr, "invalid event received. %v", err)
		return err
	}
	fmt.Printf("%v\n", event)
	return nil
}
