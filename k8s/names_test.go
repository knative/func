package k8s

import "testing"

// TestToSubdomain ensures that a valid domain name is
// encoded into the expected subdmain.
func TestToSubdomain(t *testing.T) {
	cases := []struct {
		In  string
		Out string
		Err bool
	}{
		{"", "", true},        // invalid domain
		{"*", "", true},       // invalid domain
		{"example", "", true}, // invalid domain
		{"example.com", "example-com", false},
		{"my-domain.com", "my--domain-com", false},
	}

	for _, c := range cases {
		out, err := ToSubdomain(c.In)
		if err != nil && !c.Err {
			t.Fatalf("Unexpected error: %v", err)
		}
		if out != c.Out {
			t.Fatalf("expected '%v' to yield '%v', got '%v'", c.In, c.Out, out)
		}
	}
}

// TestFromSubdomain ensures that a valid subdomain is decoded
// back into a domain.
func TestFromSubdomain(t *testing.T) {
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
		out, err := FromSubdomain(c.In)
		if err != nil && !c.Err {
			t.Fatalf("Unexpected error: %v", err)
		}
		if out != c.Out {
			t.Fatalf("expected '%v' to yield '%v', got '%v'", c.In, c.Out, out)
		}
	}

}
