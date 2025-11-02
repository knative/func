//go:build !integration
// +build !integration

package utils

import (
	"fmt"
	"strings"
	"testing"
)

// TestValidateFunctionName tests that only correct function names are accepted
func TestValidateFunctionName(t *testing.T) {
	cases := []struct {
		In    string
		Valid bool
	}{
		{"", false},
		{"*", false},
		{"-", false},
		{"example", true},
		{"example-com", true},
		{"example.com", false},
		{"-example-com", false},
		{"example-com-", false},
		{"Example", false},
		{"EXAMPLE", false},
		{"42", false},
	}

	for _, c := range cases {
		err := ValidateFunctionName(c.In)
		if err != nil && c.Valid {
			t.Fatalf("Unexpected error: %v, for '%v'", err, c.In)
		}
		if err == nil && !c.Valid {
			t.Fatalf("Expected error for invalid entry: %v", c.In)
		}
	}
}

func TestValidateFunctionNameErrMsg(t *testing.T) {
	invalidFnName := "EXAMPLE"
	errMsgPrefix := fmt.Sprintf("Function name '%v'", invalidFnName)

	err := ValidateFunctionName(invalidFnName)
	if err != nil {
		if !strings.HasPrefix(err.Error(), errMsgPrefix) {
			t.Fatalf("Unexpected error message: %v, the message should start with '%v' string", err.Error(), errMsgPrefix)
		}
	} else {
		t.Fatalf("Expected error for invalid entry: %v", invalidFnName)
	}
}

func TestValidateEnvVarName(t *testing.T) {
	cases := []struct {
		In    string
		Valid bool
	}{
		{"", false},
		{"*", false},
		{"example", true},
		{"example-com", true},
		{"example.com", true},
		{"-example-com", true},
		{"example-com-", true},
		{"Example", true},
		{"EXAMPLE", true},
		{";Example", false},
		{":Example", false},
		{",Example", false},
	}

	for _, c := range cases {
		err := ValidateEnvVarName(c.In)
		if err != nil && c.Valid {
			t.Fatalf("Unexpected error: %v, for '%v'", err, c.In)
		}
		if err == nil && !c.Valid {
			t.Fatalf("Expected error for invalid entry: %v", c.In)
		}
	}
}

func TestValidateConfigMapKey(t *testing.T) {
	cases := []struct {
		In    string
		Valid bool
	}{
		{"", false},
		{"*", false},
		{"example", true},
		{"example-com", true},
		{"example.com", true},
		{"-example-com", true},
		{"example-com-", true},
		{"Example", true},
		{"Example_com", true},
		{"Example_com.com", true},
		{"EXAMPLE", true},
		{";Example", false},
		{":Example", false},
		{",Example", false},
	}

	for _, c := range cases {
		err := ValidateConfigMapKey(c.In)
		if err != nil && c.Valid {
			t.Fatalf("Unexpected error: %v, for '%v'", err, c.In)
		}
		if err == nil && !c.Valid {
			t.Fatalf("Expected error for invalid entry: %v", c.In)
		}
	}
}

func TestValidateSecretKey(t *testing.T) {
	cases := []struct {
		In    string
		Valid bool
	}{
		{"", false},
		{"*", false},
		{"example", true},
		{"example-com", true},
		{"example.com", true},
		{"-example-com", true},
		{"example-com-", true},
		{"Example", true},
		{"Example_com", true},
		{"Example_com.com", true},
		{"EXAMPLE", true},
		{";Example", false},
		{":Example", false},
		{",Example", false},
	}

	for _, c := range cases {
		err := ValidateSecretKey(c.In)
		if err != nil && c.Valid {
			t.Fatalf("Unexpected error: %v, for '%v'", err, c.In)
		}
		if err == nil && !c.Valid {
			t.Fatalf("Expected error for invalid entry: %v", c.In)
		}
	}
}

func TestValidateLabelName(t *testing.T) {
	cases := []struct {
		In    string
		Valid bool
	}{
		{"", false},
		{"*", false},
		{"example", true},
		{"example-com", true},
		{"example.com", true},
		{"-example-com", false},
		{"example-com-", false},
		{"Example", true},
		{"EXAMPLE", true},
		{"example.com/example", true},
		{";Example", false},
		{":Example", false},
		{",Example", false},
	}

	for _, c := range cases {
		err := ValidateLabelKey(c.In)
		if err != nil && c.Valid {
			t.Fatalf("Unexpected error: %v, for '%v'", err, c.In)
		}
		if err == nil && !c.Valid {
			t.Fatalf("Expected error for invalid entry: %v", c.In)
		}
	}
}

func TestValidateLabelValue(t *testing.T) {
	cases := []struct {
		In    string
		Valid bool
	}{
		{"", true},
		{"*", false},
		{"example", true},
		{"example-com", true},
		{"example.com", true},
		{"-example-com", false},
		{"example-com-", false},
		{"Example", true},
		{"EXAMPLE", true},
		{"example.com/example", false},
		{";Example", false},
		{":Example", false},
		{",Example", false},
		{"{{env.EXAMPLE}}", true},
	}

	for _, c := range cases {
		err := ValidateLabelValue(c.In)
		if err != nil && c.Valid {
			t.Fatalf("Unexpected error: %v, for '%v'", err, c.In)
		}
		if err == nil && !c.Valid {
			t.Fatalf("Expected error for invalid entry: %v", c.In)
		}
	}
}

// TestValidateDomain tests that only correct DNS subdomain names are accepted
func TestValidateDomain(t *testing.T) {
// TestValidateNamespace tests that only correct Kubernetes namespace names are accepted
func TestValidateNamespace(t *testing.T) {
	cases := []struct {
		In    string
		Valid bool
	}{
		// Valid domains
		{"", true},                               // empty is valid (means use default)
		{"example.com", true},                    // standard domain
		{"api.example.com", true},                // subdomain
		{"my-app.example.com", true},             // subdomain with hyphen
		{"app-123.example.com", true},            // subdomain with number
		{"123.example.com", true},                // label starting with number
		{"a.b.c.d.e.com", true},                  // many subdomains
		{"localhost", true},                      // single label (valid)
		{"cluster.local", true},                  // Kubernetes internal domain
		{"my-app-123.staging.example.com", true}, // complex valid domain
		{"app.staging.v1.example.com", true},     // multi-level subdomain
		{"example-app.com", true},                // hyphen in domain
		{"a.co", true},                           // short domain
		{"123app.example.com", true},             // label starting with number
		// Invalid domains
		{"Example.Com", false},        // uppercase not allowed
		{"MY-APP.COM", false},         // uppercase not allowed
		{"my_app.com", false},         // underscore not allowed
		{"my app.com", false},         // space not allowed
		{"invalid domain.com", false}, // space not allowed
		{"my@app.com", false},         // @ not allowed
		{"app!.com", false},           // ! not allowed
		{"-example.com", false},       // cannot start with hyphen
		{"example-.com", false},       // label cannot end with hyphen
		{"example.-com.com", false},   // label cannot start with hyphen
		{"my..app.com", false},        // consecutive dots not allowed
		{".example.com", false},       // cannot start with dot
		{"my:app.com", false},         // colon not allowed
		{"my;app.com", false},         // semicolon not allowed
		{"my,app.com", false},         // comma not allowed
		{"my*app.com", false},         // asterisk not allowed
		{" example.com", false},       // leading whitespace not allowed
		{"example.com ", false},       // trailing whitespace not allowed
		{"example.com.", false},       // trailing dot not allowed
		{"example@domain.com", false}, // @ not allowed
		{"ex!ample.com", false},       // ! not allowed
	}

	for _, c := range cases {
		err := ValidateDomain(c.In)
		if err != nil && c.Valid {
			t.Fatalf("Unexpected error for valid domain: %v, domain: '%v'", err, c.In)
		}
		if err == nil && !c.Valid {
			t.Fatalf("Expected error for invalid domain: '%v'", c.In)
		// Valid namespaces
		{"default", true},
		{"kube-system", true},
		{"my-namespace", true},
		{"myapp", true},
		{"my-app-123", true},
		{"prod", true},
		{"test-123", true},
		{"a", true},
		{"a-b", true},
		{"abc-123-xyz", true},

		// Invalid namespaces
		{"123app", false},            // cannot start with number (K8s requirement)
		{"123invalid", false},        // cannot start with number (K8s requirement)
		{"1", false},                 // cannot start with number (K8s requirement)
		{"My-App", false},            // uppercase not allowed
		{"MY-APP", false},            // uppercase not allowed
		{"my_app", false},            // underscore not allowed
		{"my app", false},            // spaces not allowed
		{"invalid namespace", false}, // spaces not allowed
		{"my@app", false},            // @ not allowed
		{"invalid@namespace", false}, // @ not allowed
		{"-myapp", false},            // cannot start with hyphen
		{"myapp-", false},            // cannot end with hyphen
		{"my..app", false},           // dots not allowed
		{"my/app", false},            // slash not allowed
		{"my:app", false},            // colon not allowed
		{"my;app", false},            // semicolon not allowed
		{"my,app", false},            // comma not allowed
		{"my*app", false},            // asterisk not allowed
		{"my!app", false},            // exclamation not allowed
	}

	for _, c := range cases {
		err := ValidateNamespace(c.In)
		if err != nil && c.Valid {
			t.Fatalf("Unexpected error for valid namespace: %v, namespace: '%v'", err, c.In)
		}
		if err == nil && !c.Valid {
			t.Fatalf("Expected error for invalid namespace: '%v'", c.In)
		}
	}
}

func TestValidateDomainErrMsg(t *testing.T) {
	invalidDomain := "my@app.com"
	errMsgPrefix := fmt.Sprintf("Domain '%v'", invalidDomain)

	err := ValidateDomain(invalidDomain)
func TestValidateNamespaceErrMsg(t *testing.T) {
	invalidNamespace := "my@app"
	errMsgPrefix := fmt.Sprintf("Namespace '%v'", invalidNamespace)

	err := ValidateNamespace(invalidNamespace)
	if err != nil {
		if !strings.HasPrefix(err.Error(), errMsgPrefix) {
			t.Fatalf("Unexpected error message: %v, the message should start with '%v' string", err.Error(), errMsgPrefix)
		}
	} else {
		t.Fatalf("Expected error for invalid domain: %v", invalidDomain)
	}
}

// TestValidateDomainEmptyString ensures empty string is handled specially
func TestValidateDomainEmptyString(t *testing.T) {
	// Empty string should be valid (means use cluster default)
	err := ValidateDomain("")
	if err != nil {
		t.Fatalf("Empty string should be valid (means use default): %v", err)
	}

	// String with only whitespace should error
	err = ValidateDomain("   ")
	if err == nil {
		t.Fatal("String with only whitespace should be invalid")
	}
}
		t.Fatalf("Expected error for invalid namespace: %v", invalidNamespace)
	}
}
