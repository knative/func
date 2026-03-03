// Package main is a Go WASI HTTP function targeting wasip2 via TinyGo.
//
// Build with:
//
//	tinygo build -target=wasip2 -o function.wasm .
package main

import (
	"fmt"
	"net/http"
)

func init() {
	http.HandleFunc("/", handle)
}

// handle processes the incoming HTTP request.
//
// YOUR CODE HERE — replace the body of this function with your logic.
func handle(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, "Hello from WASI! Path: %s\n", r.URL.Path)
}

// main is required by TinyGo but is never called at runtime.
func main() {}
