/*
This file is for exposure stuff -> about functions being externally exposed (
outside of cluster) and what constants does functions project define with
validation and helper functions
*/
package functions

import (
	"fmt"
	"slices"

	"knative.dev/func/pkg/deployers"
)

const (
	// ExposeNone intends to deploy cluster-local (no exposure)
	ExposeNone = "none"
	// ExposeRoute is an OpenShift Route, an OpenShift-only resource
	ExposeRoute = "route"
)

// ExposeModes are the mechanisms accepted in addition to "". Adding one here
// extends validation and shell completion together
var ExposeModes = []string{ExposeNone, ExposeRoute}

// ValidateExpose reports whether expose is a valid exposure mode: "" or
// ExposeNone for cluster-local, or one of ExposeModes. Applies to both intent
// (Function.Expose) and observed status (DeploySpec.Expose). Rejects anything
// else.
func ValidateExpose(expose string) error {
	if expose == "" || slices.Contains(ExposeModes, expose) {
		return nil
	}
	return fmt.Errorf("%w: %q", ErrInvalidExpose, expose)
}

// ActiveExpose reports whether mode names an external mechanism, as opposed to
// cluster-local ("" or ExposeNone).
func ActiveExpose(mode string) bool {
	return mode != "" && mode != ExposeNone
}

// ExposureRecordMissing is true when an active exposure intent produced no
// applied record.
//
// This will fire in library-built client use where the exposer is nil but
// the Function intends to expose (ActiveExpose(f.Expose)). Relevant only for
// raw and keda; the knative deployer is exempt because the Expose key has no
// effect there.
func ExposureRecordMissing(intent, applied, deployer string) bool {
	if !ActiveExpose(intent) || deployer == deployers.Knative {
		return false
	}
	return applied == ""
}

// wrapper for validating exposure for f.Validate()
func validateExpose(vals ...string) (errs []string) {
	for _, v := range vals {
		err := ValidateExpose(v)
		if err != nil {
			errs = append(errs, err.Error())
		}
	}
	return
}
