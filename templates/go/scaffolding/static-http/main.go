package main

import (
	"fmt"
	"net/http"
	"os"

	fhttp "knative.dev/func-go/http"

	f "f"
)

type fx func(http.ResponseWriter, *http.Request)

func (f fx) Handle(w http.ResponseWriter, r *http.Request) {
	f(w, r)
}

func main() {
	if err := fhttp.Start(fx(f.Handle)); err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
}
