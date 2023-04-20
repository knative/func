package cmd

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"sync/atomic"
	"testing"

	fn "knative.dev/func/pkg/functions"
	"knative.dev/func/pkg/mock"
)

// TestInvoke command executes the invocation path.
func TestInvoke(t *testing.T) {
	root := fromTempDirectory(t)

	var invoked int32

	// Create a test function to be invoked
	if _, err := fn.New().Init(fn.Function{Runtime: "go", Root: root}); err != nil {
		t.Fatal(err)
	}

	// Mock Runner
	// Starts a service which sets invoked=1 on any request
	runner := mock.NewRunner()
	runner.RunFn = func(ctx context.Context, f fn.Function) (job *fn.Job, err error) {
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
		_, port, _ := net.SplitHostPort(l.Addr().String())
		errs := make(chan error, 10)
		stop := func() error { _ = s.Close(); return nil }
		return fn.NewJob(f, port, errs, stop, false)
	}

	// Run the mock http service function interloper
	f, err := fn.NewFunction(root)
	if err != nil {
		t.Fatal(err)
	}
	client := fn.New(fn.WithRunner(runner))
	job, err := client.Run(context.Background(), f)
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

// TestInvoke_Namespace ensures that invocation uses the Function's namespace
// despite the currently active.
func TestInvoke_Namespace(t *testing.T) {
	root := fromTempDirectory(t)

	// Create a Function in a non-active namespace
	f := fn.Function{Runtime: "go", Root: root, Deploy: fn.DeploySpec{Namespace: "ns"}}
	_, err := fn.New().Init(f)
	if err != nil {
		t.Fatal(err)
	}

	// The shared Client constructor should receive the current function's
	// namespace when constructing its describer (used when finding the
	// function's route), not the currently active namespace.
	namespace := ""
	newClient := func(conf ClientConfig, opts ...fn.Option) (*fn.Client, func()) {
		namespace = conf.Namespace // should be set to that of the function
		return NewClient(conf, opts...)
	}
	cmd := NewInvokeCmd(newClient)
	_ = cmd.Execute() // invocation error expected

	if namespace != "ns" {
		t.Fatalf("expected client to receive function's current namespace 'ns', got '%v'", namespace)
	}
}
