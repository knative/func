//go:build !integration
// +build !integration

package function

import (
	"context"
	"strings"
	"testing"

	. "knative.dev/kn-plugin-func/testing"
)

// TestInstances_LocalErrors tests the three possible error states for a function
// when attempting to access a local instance.
func TestInstances_LocalErrors(t *testing.T) {
	root, rm := Mktemp(t)
	defer rm()

	// Create a function that will not be running
	if err := New().Create(Function{Runtime: "go", Root: root}); err != nil {
		t.Fatal(err)
	}
	// Load the function
	f, err := NewFunction(root)
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name string
		f    Function
		want error
	}{
		{
			name: "Not running", // Function exists but is not running
			f:    f,
			want: ErrNotRunning,
		},
		{
			name: "Not initialized", // A function directory is provided, but no function exists
			f:    Function{Root: "testdata/not-initialized"},
			want: ErrNotInitialized,
		},
		{
			name: "Root required", // No root directory is provided
			f:    Function{},
			want: ErrRootRequired,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			i := Instances{}
			if _, err := i.Local(context.TODO(), tt.f); err != tt.want {
				t.Errorf("Local() error = %v, wantErr %v", err, tt.want)
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
	if err := New().Create(Function{Runtime: "go", Root: root}); err != nil {
		t.Fatal(err)
	}
	// Load the function
	_, err := NewFunction(root)
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name string
		root string
		want string
	}{
		{
			name: "foo",
			root: "foo", // bad root
			want: "func.yaml: no such file or directory",
		},
		{
			name: "foo", // name and root are mismatched
			root: root,
			want: "name passed does not match name of the function at root",
		},
	}
	for _, tt := range tests {
		i := Instances{}
		if _, err := i.Remote(context.TODO(), tt.name, tt.root); !strings.Contains(err.Error(), tt.want) {
			t.Errorf("Remote() %v error = %v, wantErr %v", "Mismatched name and root", err.Error(), tt.want)
		}
	}

}
