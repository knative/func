package cmd

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
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
		{"hyphen in name", []string{"my-ext=v"}},
		{"underscore in name", []string{"my_ext=v"}},
		{"upper case name", []string{"MyExt=v"}},
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

// TestInvoke_ValidationErrorNotPrinted ensures invoke returns a validation
// error without also printing it to stdout. The CLI prints returned errors
// once, so printing here showed the same error twice.
func TestInvoke_ValidationErrorNotPrinted(t *testing.T) {
	root := FromTempDirectory(t)
	// An env entry with a name but no value fails validation.
	funcYaml := "name: myfunc\nruntime: go\nrun:\n  envs:\n  - name: FOO\n"
	if err := os.WriteFile(filepath.Join(root, fn.FunctionFile), []byte(funcYaml), 0644); err != nil {
		t.Fatal(err)
	}

	cmd := NewInvokeCmd(NewTestClient())
	cmd.SetArgs([]string{})
	stdout := piped(t)
	err := cmd.Execute()
	out := stdout()

	if err == nil || !strings.Contains(err.Error(), "missing value field") {
		t.Fatalf("expected the validation error to be returned, got: %v", err)
	}
	if out != "" {
		t.Fatalf("expected nothing on stdout, got: %q", out)
	}
}

// TestInvokeExtensionsRequireCloudEvent ensures --extension is rejected unless
// the effective format (flag, else func.yaml) is cloudevent.
func TestInvokeExtensionsRequireCloudEvent(t *testing.T) {
	const guardErr = "only valid with cloudevents"
	cases := []struct {
		name     string
		invoke   string // func.yaml 'invoke'
		args     []string
		rejected bool
	}{
		{"func.yaml http", "http", []string{"-e", "foo=bar"}, true},
		{"func.yaml cloudevent", "cloudevent", []string{"-e", "foo=bar"}, false},
		{"flag http overrides func.yaml", "cloudevent", []string{"-f", "http", "-e", "foo=bar"}, true},
		{"flag cloudevent overrides func.yaml", "http", []string{"-f", "cloudevent", "-e", "foo=bar"}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := FromTempDirectory(t)
			f, err := fn.New().Init(fn.Function{Runtime: "go", Root: root})
			if err != nil {
				t.Fatal(err)
			}
			f.Invoke = tc.invoke
			if err := f.Write(); err != nil {
				t.Fatal(err)
			}
			cmd := NewInvokeCmd(NewTestClient())
			cmd.SetArgs(tc.args)
			err = cmd.Execute()
			if got := err != nil && strings.Contains(err.Error(), guardErr); got != tc.rejected {
				t.Fatalf("expected rejected=%v, got error: %v", tc.rejected, err)
			}
		})
	}
}

// TestInvokeExtensionsReservedAttributes ensures core CloudEvents attribute
// names are rejected as extensions, and that the error points at the flag
// which sets the attribute when one exists.
func TestInvokeExtensionsReservedAttributes(t *testing.T) {
	cases := []struct {
		name string
		ext  string
		flag string // invoke flag that sets this attribute, if any
	}{
		{"id", "id=abc", "--id"},
		{"type", "type=com.example", "--type"},
		{"source", "source=/example", "--source"},
		{"datacontenttype", "datacontenttype=text/plain", "--content-type"},
		{"specversion", "specversion=1.0", ""},
		{"subject", "subject=hello", ""},
		{"time", "time=2020-01-01T00:00:00Z", ""},
		{"dataschema", "dataschema=https://example.com/schema", ""},
		{"mixed case", "ID=abc", "--id"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := invokeConfig{Extensions: []string{"ok=1", tc.ext}}
			_, err := c.parseExtensions()
			if err == nil {
				t.Fatalf("expected error for %q", tc.ext)
			}
			msg := err.Error()
			if !strings.Contains(msg, "reserved CloudEvents attribute") {
				t.Fatalf("expected reserved-attribute error, got %v", err)
			}
			if tc.flag == "" {
				if strings.Contains(msg, "set it with") {
					t.Fatalf("expected no flag hint, got %v", err)
				}
				return
			}
			if !strings.Contains(msg, tc.flag) {
				t.Fatalf("expected error to mention %s, got %v", tc.flag, err)
			}
		})
	}
}

// TestInvoke_WrapsNotInitialized ensures invoke wraps uninitialized errors
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

// TestInvoke_InvalidFormat ensures invoke rejects unsupported message formats
func TestInvoke_InvalidFormat(t *testing.T) {
	for _, format := range []string{"grpc", "websocket", "xml", "invalid"} {
		t.Run(format, func(t *testing.T) {
			cmd := NewInvokeCmd(NewTestClient())
			cmd.SetArgs([]string{"--format=" + format})
			err := cmd.Execute()
			if err == nil {
				t.Fatalf("expected error for invalid format %q, got nil", format)
			}
			if !strings.Contains(err.Error(), "message format") || !strings.Contains(err.Error(), "not supported") {
				t.Fatalf("expected error message to mention unsupported format, got: %v", err)
			}
		})
	}
}

// TestInvoke_InvalidRequestType ensures invoke rejects unsupported HTTP request types
func TestInvoke_InvalidRequestType(t *testing.T) {
	for _, reqType := range []string{"DELETE", "PUT", "PATCH", "HEAD", "OPTIONS"} {
		t.Run(reqType, func(t *testing.T) {
			cmd := NewInvokeCmd(NewTestClient())
			cmd.SetArgs([]string{"--request-type=" + reqType})
			err := cmd.Execute()
			if err == nil {
				t.Fatalf("expected error for invalid request type %q, got nil", reqType)
			}
			if !strings.Contains(err.Error(), "request type") || !strings.Contains(err.Error(), "not supported") {
				t.Fatalf("expected error message to mention unsupported request type, got: %v", err)
			}
		})
	}
}

// TestInvoke_FormatNormalization ensures invoke normalizes format case and aliases
func TestInvoke_FormatNormalization(t *testing.T) {
	for _, tc := range []struct {
		input    string
		expected string
	}{
		{"http", "http"},
		{"HTTP", "http"},
		{"cloudevent", "cloudevent"},
		{"CLOUDEVENT", "cloudevent"},
		{"cloudevents", "cloudevent"},
		{"CloudEvents", "cloudevent"},
	} {
		t.Run(tc.input, func(t *testing.T) {
			cmd := NewInvokeCmd(NewTestClient())
			cmd.SetArgs([]string{"--format=" + tc.input})
			// newInvokeConfig should accept these without returning a format error
			err := cmd.Execute()
			if err != nil && strings.Contains(err.Error(), "message format") {
				t.Fatalf("format %q should be accepted, got format error: %v", tc.input, err)
			}
		})
	}
}
