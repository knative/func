package oci

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httputil"
	"os"
	"testing"

	v1 "github.com/google/go-containerregistry/pkg/v1"
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
		success    = false
		err        error
	)
	defer done()

	// Start a handler on an OS-chosen port which confirms that an incoming
	// requests looks for the most part like what we'd expect to see from a
	// container push.
	handler := http.NewServeMux()
	handler.HandleFunc("/", func(res http.ResponseWriter, req *http.Request) {
		if verbose {
			fmt.Println("Mock registry server received request:")
			fmt.Println("--------------------------------------")
			requestDump, err := httputil.DumpRequest(req, true)
			if err != nil {
				t.Fatal(err)
			}
			fmt.Println(string(requestDump))
		}

		// Ignore all requests except the PUTs
		if req.Method != http.MethodPut {
			return
		}

		// Ignore all PUTs except the the image index
		if req.Header.Get("Content-Type") != "application/vnd.oci.image.index.v1+json" {
			return
		}

		// Decode the request as an index JSON
		index := v1.IndexManifest{}
		d := json.NewDecoder(req.Body)
		defer req.Body.Close()
		if err := d.Decode(&index); err != nil {
			t.Fatal(err)
		}

		success = true

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
		fn.WithPusher(NewPusher(insecure, verbose)))

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
