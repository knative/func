package functions

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/go-git/go-git/v5/plumbing/transport"
	githttp "github.com/go-git/go-git/v5/plumbing/transport/http"
)

// setupGitCredentialStore writes a temp ~/.git-credentials file and points
// HOME at the temp directory so os.UserHomeDir() returns it during the test.
func setupGitCredentialStore(t *testing.T, credLines ...string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home) // Windows

	var creds string
	for _, line := range credLines {
		creds += line + "\n"
	}
	if err := os.WriteFile(filepath.Join(home, ".git-credentials"), []byte(creds), 0600); err != nil {
		t.Fatal(err)
	}
}

// TestCredentialsForURL_HTTPS verifies that credentials stored in
// ~/.git-credentials are returned for a matching HTTPS URL.
func TestCredentialsForURL_HTTPS(t *testing.T) {
	setupGitCredentialStore(t, "https://alice:s3cr3t@example.com")

	auth := credentialsForURL("https://example.com/org/repo")
	if auth == nil {
		t.Fatal("expected non-nil AuthMethod, got nil")
	}
	basic, ok := auth.(*githttp.BasicAuth)
	if !ok {
		t.Fatalf("expected *githttp.BasicAuth, got %T", auth)
	}
	if basic.Username != "alice" {
		t.Errorf("username: want %q, got %q", "alice", basic.Username)
	}
	if basic.Password != "s3cr3t" {
		t.Errorf("password: want %q, got %q", "s3cr3t", basic.Password)
	}
}

// TestCredentialsForURL_TokenAuth verifies that a token stored with an empty
// username (common for GitHub/GitLab PATs via x-oauth-basic) is accepted.
func TestCredentialsForURL_TokenAuth(t *testing.T) {
	setupGitCredentialStore(t, "https://x-oauth-basic:ghp_tok3n@github.com")

	auth := credentialsForURL("https://github.com/org/repo")
	if auth == nil {
		t.Fatal("expected non-nil AuthMethod, got nil")
	}
	basic, ok := auth.(*githttp.BasicAuth)
	if !ok {
		t.Fatalf("expected *githttp.BasicAuth, got %T", auth)
	}
	if basic.Password != "ghp_tok3n" {
		t.Errorf("password: want %q, got %q", "ghp_tok3n", basic.Password)
	}
}

// TestCredentialsForURL_NoMatchingEntry verifies that nil is returned when
// ~/.git-credentials has no entry for the requested host.
func TestCredentialsForURL_NoMatchingEntry(t *testing.T) {
	setupGitCredentialStore(t, "https://user:pass@other.com")

	auth := credentialsForURL("https://example.com/repo")
	if auth != nil {
		t.Fatalf("expected nil AuthMethod for unmatched host, got %v", auth)
	}
}

// TestCredentialsForURL_NonHTTP verifies that nil is returned immediately for
// non-HTTP(S) schemes without reading any credential file.
func TestCredentialsForURL_NonHTTP(t *testing.T) {
	setupGitCredentialStore(t, "https://user:pass@example.com")

	for _, u := range []string{
		"git@example.com:org/repo.git",
		"ssh://git@example.com/repo",
		"file:///local/repo",
	} {
		if auth := credentialsForURL(u); auth != nil {
			t.Errorf("credentialsForURL(%q): expected nil for non-HTTP(S) scheme, got %v", u, auth)
		}
	}
}

// TestCredentialsForURL_NoCredentialsFile verifies that nil is returned when
// neither ~/.git-credentials nor ~/.netrc exist.
func TestCredentialsForURL_NoCredentialsFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	auth := credentialsForURL("https://example.com/repo")
	if auth != nil {
		t.Fatalf("expected nil AuthMethod when no credentials files exist, got %v", auth)
	}
}

// TestCredentialsForURL_NetRC verifies fallback to ~/.netrc when
// ~/.git-credentials has no match.
func TestCredentialsForURL_NetRC(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	netrc := "machine example.com login bob password secret\n"
	if err := os.WriteFile(filepath.Join(home, ".netrc"), []byte(netrc), 0600); err != nil {
		t.Fatal(err)
	}

	auth := credentialsForURL("https://example.com/repo")
	if auth == nil {
		t.Fatal("expected non-nil AuthMethod from .netrc, got nil")
	}
	basic, ok := auth.(*githttp.BasicAuth)
	if !ok {
		t.Fatalf("expected *githttp.BasicAuth, got %T", auth)
	}
	if basic.Username != "bob" {
		t.Errorf("username: want %q, got %q", "bob", basic.Username)
	}
	if basic.Password != "secret" {
		t.Errorf("password: want %q, got %q", "secret", basic.Password)
	}
}

// TestCredentialsForURL_CrossScheme verifies that a credential stored under
// http:// is returned when the request URL uses https://, and vice-versa.
// This mirrors git's own behaviour: scheme is not part of the host identity
// for BasicAuth purposes.
func TestCredentialsForURL_CrossScheme(t *testing.T) {
	tests := []struct {
		name       string
		storedURL  string
		requestURL string
	}{
		{
			name:       "http credential matches https request",
			storedURL:  "http://alice:s3cr3t@example.com",
			requestURL: "https://example.com/org/repo",
		},
		{
			name:       "https credential matches http request",
			storedURL:  "https://alice:s3cr3t@example.com",
			requestURL: "http://example.com/org/repo",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			setupGitCredentialStore(t, tc.storedURL)
			auth := credentialsForURL(tc.requestURL)
			if auth == nil {
				t.Fatalf("expected non-nil AuthMethod for cross-scheme match, got nil")
			}
			basic, ok := auth.(*githttp.BasicAuth)
			if !ok {
				t.Fatalf("expected *githttp.BasicAuth, got %T", auth)
			}
			if basic.Username != "alice" {
				t.Errorf("username: want %q, got %q", "alice", basic.Username)
			}
		})
	}
}

// TestCredentialsForURL_PortMismatch verifies that a credential with an
// explicit non-standard port does NOT match a URL on a different port.
// http://example.com:8080 must not satisfy a request to http://example.com.
func TestCredentialsForURL_PortMismatch(t *testing.T) {
	setupGitCredentialStore(t, "http://alice:s3cr3t@example.com:8080")

	auth := credentialsForURL("http://example.com/repo")
	if auth != nil {
		t.Fatalf("expected nil AuthMethod for port mismatch, got %v", auth)
	}
}

// TestCredentialsForURL_PortMatch verifies that a credential with an explicit
// non-standard port matches a request URL with the same port.
func TestCredentialsForURL_PortMatch(t *testing.T) {
	setupGitCredentialStore(t, "https://alice:s3cr3t@example.com:8443")

	auth := credentialsForURL("https://example.com:8443/repo")
	if auth == nil {
		t.Fatal("expected non-nil AuthMethod for matching custom port, got nil")
	}
	basic, ok := auth.(*githttp.BasicAuth)
	if !ok {
		t.Fatalf("expected *githttp.BasicAuth, got %T", auth)
	}
	if basic.Username != "alice" {
		t.Errorf("username: want %q, got %q", "alice", basic.Username)
	}
}

// TestCredentialsForURL_ImplicitPortsAreEquivalent verifies that the implicit
// default ports (80 for http, 443 for https) are treated as equivalent to no
// port at all, so http://example.com and https://example.com:443 match the
// same credential entry.
func TestCredentialsForURL_ImplicitPortsAreEquivalent(t *testing.T) {
	tests := []struct {
		name       string
		storedURL  string
		requestURL string
	}{
		{
			name:       "stored port 443 matches https no-port request",
			storedURL:  "https://alice:s3cr3t@example.com:443",
			requestURL: "https://example.com/repo",
		},
		{
			name:       "stored port 80 matches http no-port request",
			storedURL:  "http://alice:s3cr3t@example.com:80",
			requestURL: "http://example.com/repo",
		},
		{
			name:       "stored no-port matches request with explicit port 443",
			storedURL:  "https://alice:s3cr3t@example.com",
			requestURL: "https://example.com:443/repo",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			setupGitCredentialStore(t, tc.storedURL)
			auth := credentialsForURL(tc.requestURL)
			if auth == nil {
				t.Fatalf("expected non-nil AuthMethod for implicit-port equivalence, got nil")
			}
		})
	}
}

// TestIsAuthError verifies the sentinel value detection used for retry logic.
func TestIsAuthError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"unrelated error", errors.New("repository not found"), false},
		{"ErrAuthenticationRequired", transport.ErrAuthenticationRequired, true},
		{"wrapped ErrAuthenticationRequired", fmt.Errorf("clone failed: %w", transport.ErrAuthenticationRequired), true},
		{"ErrAuthorizationFailed is not auth-required", transport.ErrAuthorizationFailed, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := isAuthError(tc.err); got != tc.want {
				t.Errorf("isAuthError(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}
