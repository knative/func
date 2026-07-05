package functions

import (
	"errors"
	"fmt"
	"strings"
	"testing"
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
