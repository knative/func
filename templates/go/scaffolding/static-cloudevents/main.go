package main

import (
	"fmt"
	"os"

	ce "knative.dev/func-go/cloudevents"

	f "function"
)

func main() {
	if err := ce.Start(ce.DefaultHandler{Handler: f.Handle}); err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
}
