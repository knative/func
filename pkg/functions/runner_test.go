package functions

import (
	"context"
	"errors"
	"testing"
)

// TestGetRunFuncErrors ensures that known runtimes which do not yet
// have their runner implemented return a "not yet available" message, as
// distinct from unrecognized runtimes which state as much.
func TestGetRunFuncErrors(t *testing.T) {
	tests := []struct {
		Runtime    string
		ExpectedIs error
		ExpectedAs any
	}{
		{"", ErrRuntimeRequired, nil},
		{"go", nil, nil},
		{"python", nil, nil},
		{"rust", nil, &ErrRunnerNotImplemented{}},
		{"node", nil, &ErrRunnerNotImplemented{}},
		{"typescript", nil, &ErrRunnerNotImplemented{}},
		{"quarkus", nil, &ErrRunnerNotImplemented{}},
		{"springboot", nil, &ErrRunnerNotImplemented{}},
		{"other", nil, &ErrRuntimeNotRecognized{}},
	}
	for _, test := range tests {
		t.Run(test.Runtime, func(t *testing.T) {

			ctx := context.Background()
			job := Job{Function: Function{Runtime: test.Runtime}}
			_, err := getRunFunc(ctx, &job)

			if test.ExpectedAs != nil && !errors.As(err, test.ExpectedAs) {
				t.Fatalf("did not receive expected error type for %v runtime.", test.Runtime)
			}
			t.Logf("ok: %v", err)
		})
	}
}

func TestParseAddressFlag(t *testing.T) {
	tests := []struct {
		name string
		val  string
		host string
		port string
	}{
		{
			name: "empty value",
			val:  "",
			host: "localhost",
			port: "8080",
		},
		{
			name: "host-only as hostname",
			val:  "localhost",
			host: "localhost",
			port: "8080",
		},
		{
			name: "host-only as ipv4",
			val:  "127.0.0.2",
			host: "127.0.0.2",
			port: "8080",
		},
		{
			name: "host-only as ipv6",
			val:  "::1",
			host: "::1",
			port: "8080",
		},
		{
			name: "hostport as hostname",
			val:  "localhost:5000",
			host: "localhost",
			port: "5000",
		},
		{
			name: "hostport as ipv4",
			val:  "127.0.0.2:5000",
			host: "127.0.0.2",
			port: "5000",
		},
		{
			name: "hostport as ipv6",
			val:  "[::1]:5000",
			host: "::1",
			port: "5000",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, p := ParseAddressFlag(tt.val)
			if h != tt.host {
				t.Errorf("ParseAddressFlag() got host = %v, want host %v", h, tt.host)
			}
			if p != tt.port {
				t.Errorf("ParseAddressFlag() got port = %v, want port %v", p, tt.port)
			}
		})
	}
}

