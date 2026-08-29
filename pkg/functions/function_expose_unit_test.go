package functions

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"knative.dev/func/pkg/deployers"
)

func Test_ValidateExpose(t *testing.T) {
	for _, v := range []string{"", "route", "none"} {
		t.Run(v, func(t *testing.T) {
			if err := ValidateExpose(v); err != nil {
				t.Fatalf("ValidateExpose(%q): unexpected error: %v", v, err)
			}
		})
	}

	for _, v := range []string{"auto", "bogus", "ingress"} {
		t.Run(v, func(t *testing.T) {
			err := ValidateExpose(v)
			if !errors.Is(err, ErrInvalidExpose) {
				t.Fatalf("ValidateExpose(%q): expected errors.Is(err, ErrInvalidExpose), got %v", v, err)
			}
			if !strings.Contains(err.Error(), fmt.Sprintf("%q", v)) {
				t.Errorf("ValidateExpose(%q): expected error to quote the bad value, got %v", v, err)
			}
		})
	}
}

func Test_ActiveExpose(t *testing.T) {
	if ActiveExpose("") || ActiveExpose("none") {
		t.Error("empty and none must not be active")
	}
	if !ActiveExpose("route") {
		t.Error("route must be active")
	}
}

func Test_ExposureRecordMissing(t *testing.T) {
	tests := []struct {
		intent, applied, deployer string
		want                      bool
	}{
		{ExposeRoute, "", deployers.Kubernetes, true},
		{ExposeRoute, "", deployers.Keda, true},
		{ExposeRoute, "", deployers.Knative, false},
		{ExposeRoute, ExposeRoute, deployers.Kubernetes, false},
		{ExposeNone, "", deployers.Kubernetes, false},
		{"", "", deployers.Kubernetes, false},
	}
	for _, tt := range tests {
		t.Run(tt.intent+"/"+tt.applied+"/"+tt.deployer, func(t *testing.T) {
			if got := ExposureRecordMissing(tt.intent, tt.applied, tt.deployer); got != tt.want {
				t.Errorf("ExposureRecordMissing(%q, %q, %q) = %v, want %v",
					tt.intent, tt.applied, tt.deployer, got, tt.want)
			}
		})
	}
}
