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

//go:generate go run go.bytecodealliance.org/cmd/wit-bindgen-go generate --world boson --out gen --package-root function/gen ./wit

package main

import (
	"go.bytecodealliance.org/cm"

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
	path := greet(request.PathWithQuery().Value())

	headers := types.NewFields()
	resp := types.NewOutgoingResponse(headers)
	resp.SetStatusCode(200)

	bodyResult := resp.Body()
	body := *bodyResult.OK()
	streamResult := body.Write()
	stream := *streamResult.OK()

	stream.BlockingWriteAndFlush(cm.ToList([]byte(path)))

	stream.ResourceDrop()
	types.OutgoingBodyFinish(body, cm.None[types.Trailers]())
	types.ResponseOutparamSet(responseOut, cm.OK[cm.Result[types.ErrorCodeShape, types.OutgoingResponse, types.ErrorCode]](resp))
}

// greet returns a greeting string for the given URL path.
// Extracted as a pure function to allow unit testing without a WASM runtime.
func greet(path string) string {
	return "Hello from WASI! Path: " + path + "\n"
}

// main is required by TinyGo but is never called at runtime.
func main() {}
