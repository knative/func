// Package main is a Go WASI HTTP function targeting wasip2 via TinyGo.
//
// The //go:generate directive below runs wit-bindgen-go to produce Go bindings
// in gen/ from the WIT world defined in wit/world.wit (plus downloaded deps).
//
// Build prerequisites:
//   - tinygo 0.40.1+ (https://tinygo.org)
//   - wasm-tools (https://github.com/bytecodealliance/wasm-tools)
//   - go.bytecodealliance.org/cmd/wit-bindgen-go (via `go generate`)
//
// Build with:
//
//	func build

//go:generate go run go.bytecodealliance.org/cmd/wit-bindgen-go generate --world boson ./wit

package main

import (
	incominghandler "function/gen/wasi/http/incoming-handler"
	"function/gen/wasi/http/types"
)

func init() {
	incominghandler.Exports.Handle = handle
}

// handle processes the incoming WASI HTTP request.
//
// YOUR CODE HERE — replace the body of this function with your logic.
func handle(
	request types.IncomingRequest,
	responseOut types.ResponseOutparam,
) {
	path := greet(request.PathWithQuery().Unwrap())

	headers := types.NewFields()
	resp := types.NewOutgoingResponse(headers)
	resp.SetStatusCode(200)
	body := resp.Body()
	stream := body.Write()

	stream.BlockingWriteAndFlush([]byte(path))

	stream.Drop()
	types.OutgoingBodyFinish(body, types.None[types.Trailers]())
	types.ResponseOutparamSet(responseOut, types.Ok[types.OutgoingResponse, types.ErrorCode](resp))
}

// greet returns a greeting string for the given URL path.
// Extracted as a pure function to allow unit testing without a WASM runtime.
func greet(path string) string {
	return "Hello from WASI! Path: " + path + "\n"
}

// main is required by TinyGo but is never called at runtime.
func main() {}
