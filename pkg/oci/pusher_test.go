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

	fn "knative.dev/func/pkg/functions"
	"knative.dev/func/pkg/oci/mock"
	. "knative.dev/func/pkg/testing"

	"github.com/google/go-containerregistry/pkg/registry"
)

// TestPusher_Push ensures the base case that the pusher contacts the
// registry with a correctly formed request.
func TestPusher_Push(t *testing.T) {
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
		fn.WithBuilder(NewBuilder("", false)),
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

// TestPusher_Auth ensures that the pusher authenticates via basic auth when
// supplied with a username/password via the context.
func TestPusher_BasicAuth(t *testing.T) {
	var (
		root, done = Mktemp(t)
		username   = "username"
		password   = "password"
		success    = false
		verbose    = false
		err        error
	)
	defer done()

	// A mock registry with middleware which performs basic auth
	server := mock.NewRegistry()
	server.HandlerFunc = func(w http.ResponseWriter, r *http.Request) {
		u, p, ok := r.BasicAuth()
		if !ok {
			// no header.  ask for auth
			w.Header().Add("www-authenticate", "Basic realm=\"Registry Realm\"")
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
		} else if u != username || p != password {
			// header exists, but creds are either missing or incorrect
			t.Fatalf("Unauthorized.  Expected user %q pass %q, got user %q pass %q", username, password, u, p)
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		} else {
			// (at least one) request authenticated
			success = true
		}

		// always delegate to the registry impl which implements the protocol
		server.RegistryImpl.ServeHTTP(w, r)
	}
	defer server.Close()

	// Client
	// initialized with an OCI builder and pusher.
	client := fn.New(
		fn.WithBuilder(NewBuilder("", verbose)),
		fn.WithPusher(NewPusher(false, false, verbose)))

	// Function
	// Built and tagged to push to the mock registry
	f := fn.Function{
		Root:     root,
		Runtime:  "go",
		Name:     "f",
		Registry: server.Addr().String() + "/funcs"}

	if f, err = client.Init(f); err != nil {
		t.Fatal(err)
	}
	if f, err = client.Build(context.Background(), f); err != nil {
		t.Fatal(err)
	}

	// Push
	// Enables optional basic authentication via the push context to use instead
	// of the default behavior of using the multi-auth chain of config files
	// and various known credentials managers.
	ctx := context.Background()
	ctx = context.WithValue(ctx, fn.PushUsernameKey{}, username)
	ctx = context.WithValue(ctx, fn.PushPasswordKey{}, password)

	if _, err = client.Push(ctx, f); err != nil {
		t.Fatal(err)
	}

	// Assert
	// Success is set by the handler middeware of the mock registry on the
	// first successfully submitted basic auth request.
	if !success {
		t.Fatal("did not receive the image index JSON")
	}
}
