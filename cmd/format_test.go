package cmd

import (
	"bytes"
	"io"
	"strings"
	"testing"
)

type mockFormatter struct {
	invoked string
}

func (m *mockFormatter) Human(w io.Writer) error {
	m.invoked = "human"
	_, err := w.Write([]byte("human"))
	return err
}

func (m *mockFormatter) Plain(w io.Writer) error {
	m.invoked = "plain"
	_, err := w.Write([]byte("plain"))
	return err
}

func (m *mockFormatter) JSON(w io.Writer) error {
	m.invoked = "json"
	_, err := w.Write([]byte("json"))
	return err
}

func (m *mockFormatter) YAML(w io.Writer) error {
	m.invoked = "yaml"
	_, err := w.Write([]byte("yaml"))
	return err
}

func (m *mockFormatter) URL(w io.Writer) error {
	m.invoked = "url"
	_, err := w.Write([]byte("url"))
	return err
}

func Test_validateOutputFormat(t *testing.T) {
	tests := []struct {
		name    string
		format  string
		wantErr bool
	}{
		{"valid human", "human", false},
		{"valid plain", "plain", false},
		{"valid json", "json", false},
		{"valid yaml", "yaml", false},
		{"valid url", "url", false},
		{"case-insensitive JSON", "JSON", false},
		{"case-insensitive YAML", "YAML", false},
		{"case-insensitive Human", "Human", false},
		{"case-insensitive PLAIN", "PLAIN", false},
		{"case-insensitive UrL", "UrL", false},
		{"invalid format xml", "xml", true},
		{"invalid format bogus", "bogus", true},
		{"invalid format csv", "csv", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateOutputFormat(tt.format)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateOutputFormat(%q) error = %v, wantErr %v", tt.format, err, tt.wantErr)
			}
		})
	}
}

func Test_write_CaseInsensitive(t *testing.T) {
	tests := []struct {
		format      string
		wantInvoked string
	}{
		{"json", "json"},
		{"JSON", "json"},
		{"yaml", "yaml"},
		{"YAML", "yaml"},
		{"human", "human"},
		{"Human", "human"},
		{"plain", "plain"},
		{"PLAIN", "plain"},
		{"url", "url"},
		{"URL", "url"},
	}

	for _, tt := range tests {
		t.Run(tt.format, func(t *testing.T) {
			var buf bytes.Buffer
			m := &mockFormatter{}
			err := write(&buf, m, tt.format)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if m.invoked != tt.wantInvoked {
				t.Errorf("write() invoked = %q, want %q", m.invoked, tt.wantInvoked)
			}
		})
	}
}

func Test_write_UnrecognizedFormat(t *testing.T) {
	var buf bytes.Buffer
	m := &mockFormatter{}
	err := write(&buf, m, "invalid-format")
	if err == nil {
		t.Fatal("expected error for unrecognized format, got nil")
	}
	if !strings.Contains(err.Error(), "format not recognized: invalid-format") {
		t.Errorf("expected format not recognized error, got %v", err)
	}
}
