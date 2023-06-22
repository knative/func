// package function is an example of a Function implementation which
// responds to CloudEvents.
//
// This package name can be changed when using the "host" builder
// (as can the module in go.mod)
package function

import (
	"context"
	"fmt"

	"github.com/cloudevents/sdk-go/v2/event"
)

// MyFunction is your function's implementation.
// This struction name can be changed.
type MyFunction struct{}

// New constructs an instance of your function.  It is called each time a
// new function service is created.  This function must be named "New", accept
// no arguments and return an instance of a structure which exports one of the
// supported Handle signatures.
func New() *MyFunction {
	return &MyFunction{}
}

// Handle a request using your function instance.
//
// One of the following method signatures needs to be implemented for your
// function to start:
//
//	Handle()
//	Handle() error
//	Handle(context.Context)
//	Handle(context.Context) error
//	Handle(event.Event)
//	Handle(event.Event) error
//	Handle(context.Context, event.Event)
//	Handle(context.Context, event.Event) error
//	Handle(event.Event) *event.Event
//	Handle(event.Event) (*event.Event, error)
//	Handle(context.Context, event.Event) *event.Event
//	Handle(context.Context, event.Event) (*event.Event, error)
//
// One of these function signatures must be implemented for your function
// to start.
func (f *MyFunction) Handle(ctx context.Context, e event.Event) (*event.Event, error) {
	/*
	 * YOUR CODE HERE
	 *
	 * Try running `go test`.  Add more test as you code in `handle_test.go`.
	 */

	fmt.Println("Received event")
	fmt.Println(e) // echo to local output
	return &e, nil // echo to caller
}

// TODO: Start
// TODO: Stop
// TODO: Alive
// TODO: Ready

// Handle is an optional method which can be used to implement simple
// functions with little or no state and minimal testing requirements.  By
// instead choosing this package static function, one can forego the
// constructor and struct outlined above.  The same method signatures are
// supported here as well, simply withouth the struct pointer receiver.
//
// func Handle(ctx context.Context, e event.Event) (*event.Event, error) {
//
//  /* YOUR CODE HERE */
//
//	fmt.Println("Received event")
//  fmt.Println(e) // echo to local output
//  return &e, nil // echo to caller
// }
