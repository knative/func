package cmd

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ory/viper"
	fn "knative.dev/func/pkg/functions"
	"knative.dev/func/pkg/mock"
	. "knative.dev/func/pkg/testing"
)

// TestInvoke command executes the invocation path.
func TestInvoke(t *testing.T) {
	root := FromTempDirectory(t)

	var invoked int32

	// Create a test function to be invoked
	if _, err := fn.New().Init(fn.Function{Runtime: "go", Root: root}); err != nil {
		t.Fatal(err)
	}

	// Mock Runner
	// Starts a service which sets invoked=1 on any request
	runner := mock.NewRunner()
	runner.RunFn = func(ctx context.Context, f fn.Function, _ string, _ time.Duration) (job *fn.Job, err error) {
		var (
			l net.Listener
			h = http.NewServeMux()
			s = http.Server{Handler: h}
		)
		if l, err = net.Listen("tcp4", "127.0.0.1:"); err != nil {
			t.Fatal(err)
		}
		h.HandleFunc("/", func(res http.ResponseWriter, req *http.Request) {
			atomic.StoreInt32(&invoked, 1)
			_, _ = res.Write([]byte("invoked"))
		})
		go func() {
			if err = s.Serve(l); err != nil && !errors.Is(err, http.ErrServerClosed) {
				fmt.Fprintf(os.Stderr, "error serving: %v", err)
			}
		}()
		host, port, _ := net.SplitHostPort(l.Addr().String())
		errs := make(chan error, 10)
		stop := func() error { _ = s.Close(); return nil }
		return fn.NewJob(f, host, port, errs, stop, false)
	}

	// Run the mock http service function interloper
	f, err := fn.NewFunction(root)
	if err != nil {
		t.Fatal(err)
	}
	client := fn.New(fn.WithRunner(runner))
	job, err := client.Run(t.Context(), f)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = job.Stop() })

	// Test that the invoke command invokes
	cmd := NewInvokeCmd(NewClient)
	cmd.SetArgs([]string{})

	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	if atomic.LoadInt32(&invoked) != 1 {
		t.Fatal("function was not invoked")
	}
}

// TestInvokeExtensions tests the extensions parsing and validation on invoke command.
func TestInvokeExtensions(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)

	// (a) running invoke with --extension foo=bar and --format http should return an error containing 'only valid with cloudevents'
	t.Run("only valid with cloudevents", func(t *testing.T) {
		viper.Reset()
		t.Cleanup(viper.Reset)

		root := FromTempDirectory(t)

		// Create a test function to be invoked
		if _, err := fn.New().Init(fn.Function{Runtime: "go", Root: root}); err != nil {
			t.Fatal(err)
		}

		cmd := NewInvokeCmd(NewClient)
		cmd.SetArgs([]string{"--extension", "foo=bar", "--format", "http"})

		err := cmd.Execute()
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "only valid with cloudevents") {
			t.Fatalf("unexpected error message: %v", err)
		}
	})

	// (b) running invoke with --extension foo (no equals sign) should return an error containing 'invalid extension format'
	t.Run("invalid extension format", func(t *testing.T) {
		viper.Reset()
		t.Cleanup(viper.Reset)

		root := FromTempDirectory(t)

		// Create a test function to be invoked
		if _, err := fn.New().Init(fn.Function{Runtime: "go", Root: root}); err != nil {
			t.Fatal(err)
		}

		cmd := NewInvokeCmd(NewClient)
		cmd.SetArgs([]string{"--extension", "foo", "--format", "cloudevent"})

		err := cmd.Execute()
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "invalid extension format") {
			t.Fatalf("unexpected error message: %v", err)
		}
	})

	// (c) extensionsMap() with --extension foo=bar --extension baz=qux returns a map with both keys.
	t.Run("extensionsMap returns all keys", func(t *testing.T) {
		viper.Reset()
		t.Cleanup(viper.Reset)

		cfg := invokeConfig{
			Extensions: []string{"foo=bar", "baz=qux"},
		}
		m, err := cfg.extensionsMap()
		if err != nil {
			t.Fatal(err)
		}
		if len(m) != 2 {
			t.Fatalf("expected map of size 2, got: %v", m)
		}
		if m["foo"] != "bar" {
			t.Errorf("expected key 'foo' to be 'bar', got '%s'", m["foo"])
		}
		if m["baz"] != "qux" {
			t.Errorf("expected key 'baz' to be 'qux', got '%s'", m["baz"])
		}
	})
}
