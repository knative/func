package e2e

import (
	"regexp"
	"strings"
	"testing"
)

type Asserts struct {
	t *testing.T
}

func NewAsserts(t *testing.T) Asserts {
	return Asserts{t : t}
}

func (a *Asserts) MustMatch(result string, expectedRegex string) {
	if !regexp.MustCompile(expectedRegex).Match([]byte(result)) {
		a.t.Fatalf("Output didn't match %s", expectedRegex)
	}
}

func (a *Asserts) Equals(result string, expected string) {
	if expected != result {
		a.t.Fatalf("Expected %v but got %v", expected, result)
	}
}

func (a *Asserts) NotEquals(result string, expected string) {
	if expected == result {
		a.t.Fatalf("Not Expected %v but got it", expected)
	}
}

func (a *Asserts) StringContains(result string, substring string) {
	if !strings.Contains(result, substring) {
		a.t.Fatalf("Output does not contains expected substrig: %v", substring)
	}
}

func (a *Asserts) StringNotContains(result string, substring string) {
	if strings.Contains(result, substring) {
		a.t.Fatalf("Output contains not expected substring: %v", substring)
	}
}

func (a *Asserts) NotEmpty(result string, errorMessage string) {
	if result == "" {
		if errorMessage == "" {
			errorMessage = "Result is empty and it should not be"
		}
		a.t.Fatal(errorMessage)
	}
}

func (a *Asserts) True(condition bool, errorMessage string) {
	if !condition {
		a.t.Fatal(errorMessage)
	}
}

func (a *Asserts) Http2xx(statusCode int) {
	a.Http2xxWithMessage(statusCode, "")
}

func (a *Asserts) Http2xxWithMessage(statusCode int, errorMessage string) {
	if statusCode < 200 || statusCode > 299 {
		a.t.Fatal("Invalid response code ", errorMessage, ". ", errorMessage)
	}
}
