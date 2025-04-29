package main

import (
	"fmt"
	"os"

	"knative.dev/func-go/http"

	f "function"
)

func main() {
	if err := http.Start(f.New()); err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
}
