package cmd

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/ory/viper"
)

// jsonEnvelope mirrors JSONResponse but defers decoding of the data payload,
// so tests can assert on the payload in its real type rather than on the
// interface{} soup a direct JSONResponse unmarshal produces.
type jsonEnvelope struct {
	APIVersion string          `json:"apiVersion"`
	Status     string          `json:"status"`
	Data       json.RawMessage `json:"data,omitempty"`
	Error      *JSONError      `json:"error,omitempty"`
}

// decodeJSONEnvelope asserts that b is a well-formed success envelope and
// unmarshals its data payload into out.  Pass a nil out to check only the
// envelope.  Tests use this rather than checking `data != nil` so that a
// regression in what a command actually reports cannot slip through.
func decodeJSONEnvelope(t *testing.T, b []byte, out any) {
	t.Helper()
	var env jsonEnvelope
	if err := json.Unmarshal(b, &env); err != nil {
		t.Fatalf("output is not valid JSON: %v\ngot: %s", err, b)
	}
	if env.APIVersion != jsonAPIVersion {
		t.Errorf("expected apiVersion %q, got %q", jsonAPIVersion, env.APIVersion)
	}
	if env.Status != "ok" {
		t.Fatalf("expected status 'ok', got %q (error: %+v)", env.Status, env.Error)
	}
	if out == nil {
		return
	}
	if len(env.Data) == 0 {
		t.Fatalf("envelope carries no data payload: %s", b)
	}
	if err := json.Unmarshal(env.Data, out); err != nil {
		t.Fatalf("data payload does not decode into %T: %v\ngot: %s", out, err, env.Data)
	}
}

// runVersionCapture runs the root command's version subcommand with args and
// returns what it wrote to stdout.
func runVersionCapture(t *testing.T, args ...string) string {
	t.Helper()
	viper.Reset()
	var out bytes.Buffer
	cmd := NewRootCmd(RootCommandConfig{
		Name:    "func",
		Version: Version{Vers: "v0.42.0"},
	})
	cmd.SetArgs(append([]string{"version"}, args...))
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	return out.String()
}

// TestJSON_DialectParity ensures --json and --output json emit byte-identical
// envelopes.  func has exactly one JSON dialect, so a consumer written against
// either spelling parses the other; a second dialect is the regression this
// guards against.
func TestJSON_DialectParity(t *testing.T) {
	viaJSONFlag := runVersionCapture(t, "--json")
	viaOutputFlag := runVersionCapture(t, "--output", "json")

	if d := cmp.Diff(viaOutputFlag, viaJSONFlag); d != "" {
		t.Error("--json and --output json disagree (-output, +json):", d)
	}

	var v Version
	decodeJSONEnvelope(t, []byte(viaJSONFlag), &v)
	if v.Vers != "v0.42.0" {
		t.Errorf("expected version 'v0.42.0' in the data payload, got %q", v.Vers)
	}
}

// TestJSON_EnvVar ensures $FUNC_JSON enables JSON mode, which the --json flag
// help advertises.  Detecting the flag alone would leave the env var honored by
// the top-level error sink but ignored by the success paths.
func TestJSON_EnvVar(t *testing.T) {
	t.Setenv("FUNC_JSON", "true")

	var v Version
	decodeJSONEnvelope(t, []byte(runVersionCapture(t)), &v)
	if v.Vers != "v0.42.0" {
		t.Errorf("expected version 'v0.42.0' in the data payload, got %q", v.Vers)
	}
}
