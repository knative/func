package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
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

// TestInvokeExtensionsMapValid ensures well-formed key=value extensions are
// parsed correctly, including values that themselves contain '='.
func TestInvokeExtensionsMapValid(t *testing.T) {
	c := invokeConfig{Extensions: []string{"key=value", "foo=bar=baz"}}
	m, err := c.parseExtensions()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m["key"] != "value" {
		t.Fatalf("expected key=value, got %q", m["key"])
	}
	if m["foo"] != "bar=baz" {
		t.Fatalf("expected foo=bar=baz, got %q", m["foo"])
	}
}

// TestInvokeExtensionsMapMalformed ensures that extension entries missing '='
// or with an empty key return an error rather than being silently dropped.
func TestInvokeExtensionsMapMalformed(t *testing.T) {
	cases := []struct {
		name string
		exts []string
	}{
		{"missing equals", []string{"valid=ok", "badformat"}},
		{"empty key", []string{"valid=ok", "=nokey"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := invokeConfig{Extensions: tc.exts}
			_, err := c.parseExtensions()
			if err == nil {
				t.Fatalf("expected error for %v, got nil", tc.exts)
			}
		})
	}
}

// TestInvoke_FileFlag ensures that when --file is provided, the file's
// contents are read and sent as the invocation data.
func TestInvoke_FileFlag(t *testing.T) {
	root := FromTempDirectory(t)

	// Create a test function to be invoked
	if _, err := fn.New().Init(fn.Function{Runtime: "go", Root: root}); err != nil {
		t.Fatal(err)
	}

	// Create a test data file
	testData := "custom file content for invoke test"
	testFile := filepath.Join(root, "testdata.txt")
	if err := os.WriteFile(testFile, []byte(testData), 0644); err != nil {
		t.Fatal(err)
	}

	// Track what data is received by the mock server
	var (
		receivedData []byte
		mu           sync.Mutex
	)

	// Mock Runner: starts a service which captures request body
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
			body, _ := io.ReadAll(req.Body)
			mu.Lock()
			receivedData = body
			mu.Unlock()
			_, _ = res.Write([]byte("ok"))
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

	// Run the mock http service
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

	// Test that the invoke command reads and sends the file content
	cmd := NewInvokeCmd(NewClient)
	cmd.SetArgs([]string{"--file", testFile, "--content-type", "text/plain"})

	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	mu.Lock()
	got := string(receivedData)
	mu.Unlock()
	if got != testData {
		t.Fatalf("expected file content %q to be sent, got %q", testData, got)
	}
}

// TestInvoke_FileFlagNonExistent ensures that specifying a non-existent
// file via --file returns an appropriate error.
func TestInvoke_FileFlagNonExistent(t *testing.T) {
	root := FromTempDirectory(t)

	// Create a test function
	if _, err := fn.New().Init(fn.Function{Runtime: "go", Root: root}); err != nil {
		t.Fatal(err)
	}

	cmd := NewInvokeCmd(NewClient)
	cmd.SetArgs([]string{"--file", "nonexistent_file.txt"})
	err := cmd.Execute()

	if err == nil {
		t.Fatal("invoking with a nonexistent file should error")
	}
	if !strings.Contains(err.Error(), "nonexistent_file.txt") {
		t.Fatalf("error should mention the missing file, got: %v", err)
	}
}
