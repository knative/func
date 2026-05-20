package version_test

import (
	"testing"

	"knative.dev/func/pkg/version"
)

// TestGet_Empty verifies that Get returns the DefaultVers fallback when no
// build-time version has been injected (Vers == "").
func TestGet_Empty(t *testing.T) {
	orig := version.Vers
	version.Vers = ""
	defer func() { version.Vers = orig }()

	v := version.Get()
	if v == nil {
		t.Fatal("expected non-nil *semver.Version")
	}
	// String() must be clean semver without a leading 'v'
	if got := v.String(); got != "0.0.0+source" {
		t.Errorf("String() = %q; want %q", got, "0.0.0+source")
	}
	// Original() must round-trip the full default string including 'v'
	if got := v.Original(); got != "v0.0.0+source" {
		t.Errorf("Original() = %q; want %q", got, "v0.0.0+source")
	}
}

// TestGet_InjectedVersion verifies that a build-time version is parsed and
// exposed correctly.
func TestGet_InjectedVersion(t *testing.T) {
	orig := version.Vers
	version.Vers = "v1.2.3"
	defer func() { version.Vers = orig }()

	v := version.Get()
	if v == nil {
		t.Fatal("expected non-nil *semver.Version")
	}
	if got := v.String(); got != "1.2.3" {
		t.Errorf("String() = %q; want %q", got, "1.2.3")
	}
	if got := v.Original(); got != "v1.2.3" {
		t.Errorf("Original() = %q; want %q", got, "v1.2.3")
	}
}

// TestGet_InvalidFallsBack verifies that an unparseable injected version does
// not panic and falls back to DefaultVers.
func TestGet_InvalidFallsBack(t *testing.T) {
	orig := version.Vers
	version.Vers = "not-a-semver!!!"
	defer func() { version.Vers = orig }()

	v := version.Get()
	if v == nil {
		t.Fatal("expected non-nil *semver.Version even for invalid input")
	}
	if got := v.String(); got != "0.0.0+source" {
		t.Errorf("String() = %q; want %q on invalid input", got, "0.0.0+source")
	}
}
