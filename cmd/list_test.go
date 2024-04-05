package cmd

import (
	"context"
	"testing"

	fn "knative.dev/func/pkg/functions"
	"knative.dev/func/pkg/mock"
	. "knative.dev/func/pkg/testing"
)

// TestList_Namespace ensures that list command handles namespace options
// namespace (--namespace) and all namespaces (--all-namespaces) correctly
// and that the current kube context is used by default.
func TestList_Namespace(t *testing.T) {
	_ = FromTempDirectory(t)

	tests := []struct {
		name      string
		namespace string // --namespace flag (use specific namespace)
		all       bool   // --all-namespaces (no namespace filter)
		allShort  bool   // -A (no namespace filter)
		expected  string // expected value passed to lister
		err       bool   // expected error
	}{
		{
			name:      "default (none specififed)",
			namespace: "",
			all:       false,
			allShort:  false,
			expected:  "func", // see testdata kubeconfig
		},
		{
			name:      "namespace provided",
			namespace: "ns",
			all:       false,
			allShort:  false,
			expected:  "ns",
		},
		{
			name:      "all namespaces",
			namespace: "",
			all:       true,
			allShort:  false,
			expected:  "", // --all-namespaces | -A explicitly mean none specified
		},
		{
			name:     "all namespaces - short flag",
			all:      false,
			allShort: true,
			expected: "", // blank is implemented by lister as meaning all
		},
		{
			name:      "both flags error",
			namespace: "ns",
			all:       true,
			err:       true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// create a mock lister implementation which validates the expected
			// value has been passed.
			lister := mock.NewLister()
			lister.ListFn = func(_ context.Context, namespace string) ([]fn.ListItem, error) {
				if namespace != test.expected {
					t.Fatalf("expected list namespace %q, got %q", test.expected, namespace)
				}
				return []fn.ListItem{}, nil
			}

			// Create an instance of the command which sets the flags
			// according to the test case
			cmd := NewListCmd(NewTestClient(fn.WithLister(lister)))
			args := []string{}
			if test.namespace != "" {
				args = append(args, "--namespace", test.namespace)
			}
			if test.all {
				args = append(args, "--all-namespaces")
			}
			if test.allShort {
				args = append(args, "-A")
			}
			cmd.SetArgs(args)

			// Execute
			err := cmd.Execute()

			// Check for expected error
			if err != nil {
				if !test.err {
					t.Fatalf("unexpected error: %v", err)
				}
				// expected error received
				return
			} else if test.err {
				t.Fatalf("did not receive expected error ")
			}

			// For tests which did not expect an error, ensure the lister
			// was invoked
			if !lister.ListInvoked {
				t.Fatalf("%v: the lister was not invoked", test.name)
			}

		})
	}
}
