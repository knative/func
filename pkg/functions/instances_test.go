package functions

import (
	"context"
	"errors"
	"strings"
	"testing"

	. "knative.dev/func/pkg/testing"
)

// TestInstances_LocalErrors tests the three possible error states for a function
// when attempting to access a local instance.
func TestInstances_LocalErrors(t *testing.T) {
	root, rm := Mktemp(t)
	defer rm()

	// Create a function that will not be running
	f, err := New().Init(Function{Runtime: "go", Root: root})
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name   string
		f      Function
		wantIs error
		wantAs any
	}{
		{
			name:   "Not running", // Function exists but is not running
			f:      f,
			wantIs: ErrNotRunning,
		},
		{
			name:   "Not initialized", // A function directory is provided, but no function exists
			f:      Function{Root: "testdata/not-initialized"},
			wantAs: &ErrNotInitialized{},
		},
		{
			name:   "Root required", // No root directory is provided
			f:      Function{},
			wantIs: ErrRootRequired,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			i := InstanceRefs{}
			_, err := i.Local(context.Background(), tt.f)
			if tt.wantIs != nil && !errors.Is(err, tt.wantIs) {
				t.Errorf("Local() error = %v, want %#v", err, tt.wantIs)
			}
			if tt.wantAs != nil && !errors.As(err, tt.wantAs) {
				t.Errorf("Local() error = %v, want %#v", err, tt.wantAs)
			}
		})
	}
}

// TestInstance_RemoteErrors tests the possible error states for a function when
// attempting to access a remote instance.
func TestInstance_RemoteErrors(t *testing.T) {
	root, rm := Mktemp(t)
	defer rm()

	// Create a function that will not be running
	_, err := New().Init(Function{Runtime: "go", Namespace: "ns1", Root: root})
	if err != nil {
		t.Fatal(err)
	}

	// Load the function
	if _, err := NewFunction(root); err != nil {
		t.Fatal(err)
	}

	var nameRequired = "requires function name"
	var nsRequired = "requires namespace"

	tests := []struct {
		test      string
		name      string
		namespace string
		want      string
	}{
		{
			test:      "missing namespace",
			name:      "foo",
			namespace: "",
			want:      nsRequired,
		},
		{
			test:      "missing name",
			name:      "",
			namespace: "ns",
			want:      nameRequired,
		},
		{
			test:      "missing both",
			name:      "",
			namespace: "",
			want:      nameRequired,
		},
	}
	for _, test := range tests {
		t.Run(test.test, func(t *testing.T) {
			i := InstanceRefs{}
			_, err := i.Remote(context.Background(), test.name, test.namespace)
			if err == nil {
				t.Fatal("did not receive expected error")
			}
			if !strings.Contains(err.Error(), test.want) {
				t.Errorf("expected error to contain '%v', got '%v'", test.want, err)
			}
		})
	}

}
