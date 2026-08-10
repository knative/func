package main

import (
	"fmt"
	"os"

	ce "knative.dev/func-go/cloudevents"
	"knative.dev/func-go/kafka"

	f "function"
)

func main() {
	var err error
	if os.Getenv("FUNC_TRANSPORT") == "kafka" {
		err = kafka.Start(ce.DefaultHandler{Handler: f.Handle})
	} else {
		err = ce.Start(ce.DefaultHandler{Handler: f.Handle})
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
}
