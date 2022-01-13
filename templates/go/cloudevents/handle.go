package function

import (
	"context"
	"fmt"

	event "github.com/cloudevents/sdk-go/v2"
)

// Handle an event.
func Handle(ctx context.Context, event event.Event) error {

	/*
	 * YOUR CODE HERE
	 *
	 * Try running `go test`.  Add more test as you code in `handle_test.go`.
	 */

	// Example implementation:
	fmt.Printf("%v\n", event) // print the received event to standard output

	return nil
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
