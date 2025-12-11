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
		{"java", nil, &ErrRunnerNotImplemented{}},
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
		address      string
		expectedHost string
		expectedPort string
		expectedErr  bool
	}{
		{
			name:         "empty address",
			address:      "",
			expectedHost: defaultRunHost,
			expectedPort: defaultRunPort,
			expectedErr:  false,
		},
		{
			name:         "port only",
			address:      "9000",
			expectedHost: defaultRunHost,
			expectedPort: "9000",
			expectedErr:  false,
		},
		{
			name:         "host and port",
			address:      "127.0.0.1:3000",
			expectedHost: "127.0.0.1",
			expectedPort: "3000",
			expectedErr:  false,
		},
		{
			name:         "colon port",
			address:      ":8080",
			expectedHost: "",
			expectedPort: "8080",
			expectedErr:  false,
		},
		{
			name:         "host only localhost",
			address:      "localhost",
			expectedHost: "localhost",
			expectedPort: defaultRunPort,
			expectedErr:  false,
		},
		{
			name:         "host only ipv4",
			address:      "127.0.0.2",
			expectedHost: "127.0.0.2",
			expectedPort: defaultRunPort,
			expectedErr:  false,
		},
		{
			name:         "invalid address treated as host",
			address:      "invalid",
			expectedHost: "invalid",
			expectedPort: defaultRunPort,
			expectedErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			host, port, err := parseAddress(tt.address)
			if (err != nil) != tt.expectedErr {
				t.Errorf("parseAddress() error = %v, expectedErr %v", err, tt.expectedErr)
				return
			}
			if host != tt.expectedHost {
				t.Errorf("parseAddress() host = %v, want %v", host, tt.expectedHost)
			}
			if port != tt.expectedPort {
				t.Errorf("parseAddress() port = %v, want %v", port, tt.expectedPort)
			}
		})
	}
}
