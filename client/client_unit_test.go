package client

import "testing"

// TestPathToDomain validatest the calculation used to derive a default domain
// for a Service Function from the current directory path (used as a default
// in the absence of an explicit setting).
// NOTE: although the implementation uses os.PathSeparator and is developed with
// non-unix platforms in mind, these tests are written using unix-style paths.
func TestPathToDomain(t *testing.T) {
	var noLimit = -1
	tests := []struct {
		Path   string // input path
		Limit  int    // upward recursion limit
		Domain string // Expected returned domain value
	}{
		// empty input should not find a domain.
		{"", noLimit, ""},

		// trailing slashes ignored
		{"/home/user/example.com/admin/", noLimit, "admin.example.com"},

		// root path should not find a domain.
		{"/", noLimit, ""},

		// valid path but without a domain should find no domain.
		{"/home/user", noLimit, ""},

		// valid domain as the current directory should be found.
		{"/home/user/example.com", noLimit, "example.com"},

		// valid domain as the current directory should be found even with recursion disabled.
		{"/home/user/example.com", 0, "example.com"},

		// Subdomain
		{"/src/example.com/www", noLimit, "www.example.com"},

		// Subdomain with recursion disabled should not find the domain.
		{"/src/example.com/www", 0, ""},

		// Sub-subdomain
		{"/src/example.com/www/test1", noLimit, "test1.www.example.com"},

		// Sub-subdomain with exact recursion limit to catch the domain
		{"/src/example.com/www/test1", 2, "test1.www.example.com"},

		// CWD a valid TLD+1 (not suggested, but supported)
		{"/src/my.example.com", noLimit, "my.example.com"},

		// Multi-level TLDs
		{"/src/example.co.uk", noLimit, "example.co.uk"},

		// Multi-level TLDs with sub
		{"/src/example.co.uk/www", noLimit, "www.example.co.uk"},

		// Expected behavior is `test1.my.example.com` but will yield `test1.my`
		// because .my is a TLD hence the reason why dots in directories to denote
		// multiple levels of subdomain are technically supported but not
		// recommended: unexpected behavior because the public suffices list is
		// shifty.
		{"/src/example.com/test1.my", noLimit, "test1.my"},

		// Ensure that cluster.local is explicitly allowed.
		{"/src/cluster.local/www", noLimit, "www.cluster.local"},
	}

	for _, test := range tests {
		domain := pathToDomain(test.Path, test.Limit)
		if domain != test.Domain {
			t.Fatalf("expected path '%v' (limit %v) to yield domain '%v', got '%v'", test.Path, test.Limit, test.Domain, domain)
		}
	}
}
