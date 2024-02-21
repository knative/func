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

// Start is called whenever a function instance is started.
//
// Provided to this start method are all arguments and environment variables
// which apply to this function.  For better function portability, testability
// and robustness, it is encouraged to use this method for accessing function
// configuration rather than looking for environment variables or flags.
// func (f *MyFunction) Start(ctx context.Context, args map[string]string) error {
//   fmt.Println("Function Started")
//   return nil
// }

// Stop is called whenever a function is stopped.
//
// This may happen for reasons such as being rescheduled onto a different node,
// being updated with a newer version, or if the number of function instances
// is being scaled down due to low load.  This is a good place to cleanup and
// realease any resources which expect to be manually released.
//
// func (f *Function) Stop(ctx context.Context) error { return nil }

// Alive is an optional method which allows you to more deeply indicate that
// your function is alive.  The default liveness implementation returns true
// if the function process is not deadlocked and able to respond.  A custom
// implementation of this method may be useful when a function should not be
// considered alive if any dependent services are alive, or other more
// complex logic.
//
// func (f *Function) Alive(ctx context.Context) (bool, error) {
//   return true, nil
// }

// Ready is an optional method which, when implemented, will ensure that
// requests are not made to the Function's request handler until this method
// reports true.
//
// func (f *Function) Ready(ctx context.Context) (bool, error) {
//   return true, nil
// }

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
