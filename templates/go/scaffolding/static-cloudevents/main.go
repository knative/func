package main

import (
	"fmt"
	"os"

	ce "github.com/lkingland/func-runtime-go/cloudevents"

	f "f"
)

func main() {
	if err := ce.Start(ce.DefaultHandler{f.Handle}); err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
}
