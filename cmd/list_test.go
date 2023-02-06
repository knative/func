package cmd

import (
	"testing"

	fn "knative.dev/func"
	"knative.dev/func/mock"
)

// TestList_Namespace ensures that list command options for specifying a
// namespace (--namespace) or all namespaces (--all-namespaces) are  properly
// evaluated.
func TestList_Namespace(t *testing.T) {
	_ = fromTempDirectory(t)

	tests := []struct {
		name      string
		all       bool   // --all-namespaces
		namespace string // use specific namespace
		expected  string // expected
		err       bool   // expected error
	}{
		{
			name:     "default",
			expected: "func", // see ./testdata/default_kubeconfig
		},
		{
			name:      "namespace provided",
			namespace: "ns",
			expected:  "ns",
		},
		{
			name:     "all namespaces",
			all:      true,
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
			var (
				lister = mock.NewLister()
				client = fn.New(fn.WithLister(lister))
			)
			cmd := NewListCmd(func(cc ClientConfig, options ...fn.Option) (*fn.Client, func()) {
				if cc.Namespace != test.expected {
					t.Fatalf("expected '%v', got '%v'", test.expected, cc.Namespace)
				}
				return client, func() {}
			})
			args := []string{}
			if test.namespace != "" {
				args = append(args, "--namespace", test.namespace)
			}
			if test.all {
				args = append(args, "-A")
			}
			cmd.SetArgs(args)

			err := cmd.Execute()
			if err != nil && !test.err {
				// TODO: typed error for --namespace with -A.  Perhaps ErrFlagConflict?
				t.Fatalf("unexpected error: %v", err)
			}
			if err == nil && test.err {
				t.Fatalf("did not receive expected error ")
			}
		})
	}
}
