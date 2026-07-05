package functions

import (
	"errors"
	"testing"
)

func Test_ParseExpose(t *testing.T) {
	tests := []struct {
		expose  string
		tech    string
		ref     string
		wantErr bool
	}{
		{expose: "", tech: ""},
		{expose: "gateway", tech: "gateway"},
		{expose: "gateway:infra/gw", tech: "gateway", ref: "infra/gw"},
		{expose: "gateway:infra/", tech: "gateway", ref: "infra/"},
		{expose: "none", tech: "none"},
		{expose: "auto", wantErr: true},
		{expose: "bogus", wantErr: true},
		{expose: "gateway:badref", wantErr: true},
		{expose: "gateway:/gw", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.expose, func(t *testing.T) {
			tech, ref, err := ParseExpose(tt.expose)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("ParseExpose(%q): expected an error, got nil", tt.expose)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseExpose(%q): unexpected error: %v", tt.expose, err)
			}
			if tech != tt.tech || ref != tt.ref {
				t.Errorf("ParseExpose(%q) = (%q, %q), want (%q, %q)", tt.expose, tech, ref, tt.tech, tt.ref)
			}
		})
	}
}

func Test_validateExpose(t *testing.T) {
	tests := []struct {
		expose string
		errs   int
	}{
		{"", 0},
		{"gateway", 0},
		{"gateway:infra/gw", 0},
		{"gateway:infra/", 0},
		{"none", 0},
		{"auto", 1},
		{"bogus", 1},
		{"gateway:badref", 1},
		{"gateway:/gw", 1},
	}
	for _, tt := range tests {
		t.Run(tt.expose, func(t *testing.T) {
			if got := validateExpose(tt.expose); len(got) != tt.errs {
				t.Errorf("validateExpose(%q) = %v\n got %d errors but want %d", tt.expose, got, len(got), tt.errs)
			}
		})
	}
}

// Test_ParseExpose_TypedError pins the sentinel contract: every invalid
// value wraps ErrInvalidExpose so callers can branch with errors.Is.
func Test_ParseExpose_TypedError(t *testing.T) {
	for _, v := range []string{"bogus", "ingress", "gateway:no-slash", "gateway:/name"} {
		if _, _, err := ParseExpose(v); !errors.Is(err, ErrInvalidExpose) {
			t.Errorf("ParseExpose(%q): expected errors.Is(err, ErrInvalidExpose), got %v", v, err)
		}
	}
}
