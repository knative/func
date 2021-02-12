// +build !integration

package k8s

import "testing"

// TestToK8sAllowedName ensures that a valid name is
// encoded into k8s allowed name.
func TestToK8sAllowedName(t *testing.T) {
	cases := []struct {
		In  string
		Out string
		Err bool
	}{
		{"", "", true},  // invalid name
		{"*", "", true}, // invalid name
		{"example", "example", true},
		{"example.com", "example-com", false},
		{"my-domain.com", "my--domain-com", false},
	}

	for _, c := range cases {
		out, err := ToK8sAllowedName(c.In)
		if err != nil && !c.Err {
			t.Fatalf("Unexpected error: %v, for '%v', want '%v', but got '%v'", err, c.In, c.Out, out)
		}
		if out != c.Out {
			t.Fatalf("expected '%v' to yield '%v', got '%v'", c.In, c.Out, out)
		}
	}
}

// TestFromK8sAllowedName ensures that an allowed k8s name is correctly
// decoded back into the original name.
func TestFromK8sAllowedName(t *testing.T) {
	cases := []struct {
		In  string
		Out string
		Err bool
	}{
		{"", "", true},  // invalid subdomain
		{"*", "", true}, // invalid subdomain
		{"example-com", "example.com", false},
		{"my--domain-com", "my-domain.com", false},
		{"cdn----1-my--domain-com", "cdn--1.my-domain.com", false},
	}

	for _, c := range cases {
		out, err := FromK8sAllowedName(c.In)
		if err != nil && !c.Err {
			t.Fatalf("Unexpected error: %v", err)
		}
		if out != c.Out {
			t.Fatalf("expected '%v' to yield '%v', got '%v'", c.In, c.Out, out)
		}
	}

}
