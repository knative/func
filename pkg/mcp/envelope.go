package mcp

import (
	"encoding/json"
	"fmt"
)

// jsonEnvelope mirrors the versioned envelope which every `func` command wraps
// its JSON output in (see cmd/json.go).  It is duplicated here rather than
// imported because cmd/mcp.go imports this package, so importing cmd back
// would be an import cycle.
type jsonEnvelope struct {
	APIVersion string           `json:"apiVersion"`
	Status     string           `json:"status"`
	Data       json.RawMessage  `json:"data,omitempty"`
	Error      *jsonEnvelopeErr `json:"error,omitempty"`
}

// jsonEnvelopeErr is the structured failure the CLI reports in JSON mode.
type jsonEnvelopeErr struct {
	Category  string            `json:"category"`
	Code      string            `json:"code"`
	Retryable bool              `json:"retryable"`
	Message   string            `json:"message"`
	Hint      string            `json:"hint,omitempty"`
	Context   map[string]string `json:"context,omitempty"`
}

func (e jsonEnvelopeErr) Error() string {
	msg := fmt.Sprintf("%s/%s: %s", e.Category, e.Code, e.Message)
	if e.Hint != "" {
		msg += " (" + e.Hint + ")"
	}
	return msg
}

// unwrapJSON parses a func command's JSON stdout and unmarshals the envelope's
// data payload into out.  A "status":"error" envelope is returned as an error
// carrying the CLI's own classification, so tools surface the category, code
// and remediation hint rather than a bare parse failure.
func unwrapJSON(stdout []byte, out any) error {
	var env jsonEnvelope
	if err := json.Unmarshal(stdout, &env); err != nil {
		return fmt.Errorf("failed to parse JSON output: %w\n%s", err, string(stdout))
	}
	if env.Status == "error" {
		if env.Error != nil {
			return *env.Error
		}
		return fmt.Errorf("command reported an error without detail\n%s", string(stdout))
	}
	if env.Status != "ok" {
		return fmt.Errorf("unexpected envelope status %q\n%s", env.Status, string(stdout))
	}
	if len(env.Data) == 0 {
		return fmt.Errorf("JSON output carried no data payload\n%s", string(stdout))
	}
	if err := json.Unmarshal(env.Data, out); err != nil {
		return fmt.Errorf("failed to parse JSON data payload: %w\n%s", err, string(env.Data))
	}
	return nil
}
