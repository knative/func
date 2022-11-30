package main

import (
	function "function"

	"knative.dev/func/runtime"
)

func main() {
	runtime.Start(function.Handle)
}
