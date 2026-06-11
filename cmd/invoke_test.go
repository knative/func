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

// TestInvoke_WrapsNotInitalized ensures invoke wraps uninitialized errors
// through the CLI error wrapping layer instead of inline fmt.Errorf.

func TestInvoke_WrapsNotInitialized(t *testing.T) {
	_ = FromTempDirectory(t) // empty dir, no function
	cmd := NewInvokeCmd(NewTestClient())
	cmd.SetArgs([]string{})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error when invoking from empty directory")
	}
	var cliNotInit *ErrNotInitialized
	if !errors.As(err, &cliNotInit) {
		t.Fatalf("expected ErrNotInitialized, got %T: %v", err, err)
	}
	if cliNotInit.Cmd != "invoke" {
		t.Fatalf("expected Cmd 'invoke', got '%v'", cliNotInit.Cmd)
	}
	if !strings.Contains(err.Error(), "func invoke") {
		t.Fatalf("expected error to contain 'func invoke' guidance, got: %v", err)
	}
}
