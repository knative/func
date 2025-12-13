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
		name          string
		input         string
		expectedHost  string
		expectedPort  string
		explicitPort  bool
	}{
		{
			name:         "empty value",
			input:        "",
			expectedHost: defaultRunHost,
			expectedPort: defaultRunPort,
			explicitPort: false,
		},
		{
			name:         "host-only hostname",
			input:        "localhost",
			expectedHost: "localhost",
			expectedPort: defaultRunPort,
			explicitPort: false,
		},
		{
			name:         "host-only ipv4",
			input:        "127.0.0.2",
			expectedHost: "127.0.0.2",
			expectedPort: defaultRunPort,
			explicitPort: false,
		},
		{
			name:         "host-only ipv6",
			input:        "::1",
			expectedHost: "::1",
			expectedPort: defaultRunPort,
			explicitPort: false,
		},
		{
			name:         "hostname with explicit port",
			input:        "localhost:5000",
			expectedHost: "localhost",
			expectedPort: "5000",
			explicitPort: true,
		},
		{
			name:         "ipv4 with explicit port",
			input:        "127.0.0.2:5000",
			expectedHost: "127.0.0.2",
			expectedPort: "5000",
			explicitPort: true,
		},
		{
			name:         "ipv6 with explicit port",
			input:        "[::1]:5000",
			expectedHost: "::1",
			expectedPort: "5000",
			explicitPort: true,
		},
		{
			name:         "ipv4 with empty port",
			input:        "127.0.0.1:",
			expectedHost: "127.0.0.1",
			expectedPort: defaultRunPort,
			explicitPort: false,
		},
		{
			name:         "empty host with explicit port",
			input:        ":5000",
			expectedHost: defaultRunHost,
			expectedPort: "5000",
			explicitPort: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, p, explicit := ParseAddress(tt.input)

			if h != tt.expectedHost {
				t.Errorf("host = %v, want %v", h, tt.expectedHost)
			}
			if p != tt.expectedPort {
				t.Errorf("port = %v, want %v", p, tt.expectedPort)
			}
			if explicit != tt.explicitPort {
				t.Errorf("explicitPort = %v, want %v", explicit, tt.explicitPort)
			}
		})
	}
}