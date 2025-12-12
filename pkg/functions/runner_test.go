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

func TestParseAddress(t *testing.T) {
	tests := []struct {
		name         string
		input        string
		expectedHost string
		expectedPort string
	}{
		{
			name:         "empty value",
			input:        "",
			expectedHost: "127.0.0.1",
			expectedPort: "8080",
		},
		{
			name:         "host-only as hostname",
			input:        "localhost",
			expectedHost: "localhost",
			expectedPort: "8080",
		},
		{
			name:         "host-only as ipv4",
			input:        "127.0.0.2",
			expectedHost: "127.0.0.2",
			expectedPort: "8080",
		},
		{
			name:         "host-only as ipv6",
			input:        "::1",
			expectedHost: "::1",
			expectedPort: "8080",
		},
		{
			name:         "hostport as hostname",
			input:        "localhost:5000",
			expectedHost: "localhost",
			expectedPort: "5000",
		},
		{
			name:         "hostport as ipv4",
			input:        "127.0.0.2:5000",
			expectedHost: "127.0.0.2",
			expectedPort: "5000",
		},
		{
			name:         "hostport as ipv6",
			input:        "[::1]:5000",
			expectedHost: "::1",
			expectedPort: "5000",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, p := ParseAddress(tt.input)
			if h != tt.expectedHost {
				t.Errorf("ParseAddress() got host = %v, want host %v", h, tt.expectedHost)
			}
			if p != tt.expectedPort {
				t.Errorf("ParseAddress() got port = %v, want port %v", p, tt.expectedPort)
			}
		})
	}
}
