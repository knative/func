//go:build !integration
// +build !integration

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
