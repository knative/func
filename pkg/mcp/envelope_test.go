package mcp

import (
	"strings"
	"testing"
)

// enveloped wraps a raw payload in the versioned envelope the func CLI emits
// in JSON mode, so executor fixtures state the payload they care about rather
// than repeating envelope boilerplate.
func enveloped(data string) []byte {
	return []byte(`{"apiVersion":"v1","status":"ok","data":` + data + `}`)
}

func TestUnwrapJSON_Success(t *testing.T) {
	var out struct {
		Name string `json:"name"`
	}
	if err := unwrapJSON(enveloped(`{"name":"my-function"}`), &out); err != nil {
		t.Fatal(err)
	}
	if out.Name != "my-function" {
		t.Errorf("expected name 'my-function', got %q", out.Name)
	}
}

// TestUnwrapJSON_ErrorEnvelope ensures a "status":"error" envelope surfaces the
// CLI's own classification, which is the whole point of the structured
// contract: consumers must not have to substring-match the message.
func TestUnwrapJSON_ErrorEnvelope(t *testing.T) {
	stdout := []byte(`{"apiVersion":"v1","status":"error","error":{` +
		`"category":"CLUSTER_ERROR","code":"CLUSTER_NOT_ACCESSIBLE",` +
		`"retryable":true,"message":"connection refused",` +
		`"hint":"Verify your cluster is running"}}`)

	var out map[string]any
	err := unwrapJSON(stdout, &out)
	if err == nil {
		t.Fatal("expected an error for a status:error envelope")
	}

	var envErr jsonEnvelopeErr
	if !asEnvelopeErr(err, &envErr) {
		t.Fatalf("expected a jsonEnvelopeErr, got %T: %v", err, err)
	}
	if envErr.Category != "CLUSTER_ERROR" {
		t.Errorf("expected category 'CLUSTER_ERROR', got %q", envErr.Category)
	}
	if envErr.Code != "CLUSTER_NOT_ACCESSIBLE" {
		t.Errorf("expected code 'CLUSTER_NOT_ACCESSIBLE', got %q", envErr.Code)
	}
	if !envErr.Retryable {
		t.Error("expected retryable true")
	}
	if !strings.Contains(err.Error(), "Verify your cluster is running") {
		t.Errorf("expected the hint in the error text, got %q", err.Error())
	}
}

func TestUnwrapJSON_NotJSON(t *testing.T) {
	var out map[string]any
	if err := unwrapJSON([]byte("not json"), &out); err == nil {
		t.Fatal("expected an error for non-JSON output")
	}
}

// TestUnwrapJSON_MissingData guards against a bare envelope silently
// unmarshaling into a zero-valued payload.
func TestUnwrapJSON_MissingData(t *testing.T) {
	var out map[string]any
	if err := unwrapJSON([]byte(`{"apiVersion":"v1","status":"ok"}`), &out); err == nil {
		t.Fatal("expected an error when the envelope carries no data")
	}
}

func asEnvelopeErr(err error, target *jsonEnvelopeErr) bool {
	e, ok := err.(jsonEnvelopeErr)
	if ok {
		*target = e
	}
	return ok
}
