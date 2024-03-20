package oci

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"testing"

	"github.com/google/go-containerregistry/pkg/registry"

	fn "knative.dev/func/pkg/functions"
	. "knative.dev/func/pkg/testing"
)

// TestPusher ensures that the pusher contacts the endpoint on request with
// the expected content type (see the go-containerregistry library for
// tests which confirm it is functioning as expected from there, and the
// builder tests which ensure the container being pushed is OCI-compliant.)
func TestPusher(t *testing.T) {
	var (
		root, done = Mktemp(t)
		verbose    = false
		insecure   = true
		anon       = true
		success    = false
		err        error
	)
	defer done()

	// Start a handler on an OS-chosen port which confirms that an incoming
	// requests looks for the most part like what we'd expect to see from a
	// container push.
	regLog := io.Discard
	if verbose {
		regLog = os.Stderr
	}
	regHandler := registry.New(registry.Logger(log.New(regLog, "img reg handler: ", log.LstdFlags)))
	handler := http.NewServeMux()
	handler.HandleFunc("/", func(res http.ResponseWriter, req *http.Request) {
		regHandler.ServeHTTP(res, req)
		if req.Method == http.MethodPut && req.URL.Path == "/v2/funcs/f/manifests/latest" {
			success = true
		}
	})
	l, err := net.Listen("tcp4", "127.0.0.1:")
	if err != nil {
		t.Fatal(err)
	}
	s := http.Server{Handler: handler}
	go func() {
		if err = s.Serve(l); err != nil && !errors.Is(err, http.ErrServerClosed) {
			fmt.Fprintf(os.Stderr, "error serving: %v", err)
		}
	}()
	defer s.Close()

	// Create and push a function
	client := fn.New(
		fn.WithBuilder(NewBuilder("", verbose)),
		fn.WithPusher(NewPusher(insecure, anon, verbose)))

	f := fn.Function{Root: root, Runtime: "go", Name: "f", Registry: l.Addr().String() + "/funcs"}

	if f, err = client.Init(f); err != nil {
		t.Fatal(err)
	}

	if f, err = client.Build(context.Background(), f); err != nil {
		t.Fatal(err)
	}

	if _, err = client.Push(context.Background(), f); err != nil {
		t.Fatal(err)
	}

	// Confirm the handler received at least on PUT request containing
	// an image index.
	if !success {
		t.Fatal("did not receive the image index JSON")
	}
}
