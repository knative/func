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

// Test_validateExpose asserts valid input yields no errors, and invalid input
// exactly one, containing the offending value.
func Test_validateExpose(t *testing.T) {
	for _, v := range []string{"", "route", "none"} {
		t.Run(v, func(t *testing.T) {
			if errs := validateExpose(v); len(errs) != 0 {
				t.Fatalf("validateExpose(%q): expected no errors, got %v", v, errs)
			}
		})
	}

	for _, v := range []string{"auto", "bogus", "ingress"} {
		t.Run(v, func(t *testing.T) {
			errs := validateExpose(v)
			if len(errs) != 1 {
				t.Fatalf("validateExpose(%q): expected exactly one error, got %v", v, errs)
			}
			if !strings.Contains(errs[0], v) {
				t.Errorf("validateExpose(%q): expected the message to name the bad value, got %q", v, errs[0])
			}
		})
	}
}

// Test_Validate_Expose asserts Function.Validate() accepts valid deploy.expose
// values and rejects invalid ones, covering use of Function as a library.
func Test_Validate_Expose(t *testing.T) {
	for _, v := range []string{"", "route", "none"} {
		t.Run("valid/"+v, func(t *testing.T) {
			f := Function{Root: "/tmp/fn", Deploy: DeploySpec{Expose: v}}
			if err := f.Validate(); err != nil {
				t.Fatalf("expected deploy.expose=%q to validate, got %v", v, err)
			}
		})
	}

	t.Run("invalid surfaces through Function.Validate", func(t *testing.T) {
		f := Function{Root: "/tmp/fn", Deploy: DeploySpec{Expose: "bogus"}}
		err := f.Validate()
		if err == nil {
			t.Fatal("expected an invalid deploy.expose to fail Function.Validate()")
		}
		if !strings.Contains(err.Error(), "deploy.expose=bogus") {
			t.Errorf("expected the bundled error to name the offending field and value, got %v", err)
		}
	})
}
