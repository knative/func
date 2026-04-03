package cmd

import (
	"bytes"
	"strings"
	"testing"

	"github.com/ory/viper"
)

// TestVersion_KverPrefixStripped verifies that a Knative version tag with the
// "knative-" prefix is stripped to just the semver part in verbose output.
func TestVersion_KverPrefixStripped(t *testing.T) {
	viper.Reset()
	var out bytes.Buffer
	cmd := NewRootCmd(RootCommandConfig{
		Name: "func",
		Version: Version{
			Vers: "v0.42.0",
			Kver: "knative-v1.10.0",
		},
	})
	cmd.SetArgs([]string{"version", "-v"})
	cmd.SetOut(&out)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "Knative: v1.10.0") {
		t.Errorf("expected 'knative-' prefix to be stripped, got:\n%s", out.String())
	}
}

// TestVersion_OutputJSON verifies that --output json produces valid JSON
// containing the version field.
func TestVersion_OutputJSON(t *testing.T) {
	viper.Reset()
	var out bytes.Buffer
	cmd := NewRootCmd(RootCommandConfig{
		Name:    "func",
		Version: Version{Vers: "v0.42.0"},
	})
	cmd.SetArgs([]string{"version", "--output", "json"})
	cmd.SetOut(&out)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"version"`) {
		t.Errorf("expected JSON output to contain version field, got:\n%s", out.String())
	}
}

// TestVersion_OutputYAML verifies that --output yaml produces YAML output
// containing the version field.
func TestVersion_OutputYAML(t *testing.T) {
	viper.Reset()
	var out bytes.Buffer
	cmd := NewRootCmd(RootCommandConfig{
		Name:    "func",
		Version: Version{Vers: "v0.42.0"},
	})
	cmd.SetArgs([]string{"version", "--output", "yaml"})
	cmd.SetOut(&out)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "version: v0.42.0") {
		t.Errorf("expected YAML output to contain version field, got:\n%s", out.String())
	}
}

// TestVersion_URLUnsupported verifies that the URL output format returns an error.
func TestVersion_URLUnsupported(t *testing.T) {
	v := Version{Vers: "v0.42.0"}
	var buf bytes.Buffer
	if err := v.URL(&buf); err == nil {
		t.Error("expected URL format to return an error, got nil")
	}
}
