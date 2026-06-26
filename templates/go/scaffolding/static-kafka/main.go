package main

import (
	"fmt"
	"os"

	"knative.dev/func-go/kafka"

	f "function"
)

func main() {
	if err := kafka.Start(kafka.DefaultHandler{Handler: f.Handle}); err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
}
