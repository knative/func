// +build !integration

package utils

import (
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
		err := ValidateLabelName(c.In)
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
