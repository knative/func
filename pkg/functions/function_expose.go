package functions

import (
	"fmt"
)

// ValidateExpose checks a deploy.expose value. Valid are "route" for an
// OpenShift Route, "none" for cluster-local only, and "" which means route on
// OpenShift and none anywhere else. Anything else is an error.
func ValidateExpose(expose string) error {
	if expose == "" || expose == "none" || expose == "route" {
		return nil
	}
	return fmt.Errorf("%w: %q", ErrInvalidExpose, expose)
}

// validateExpose is the Function.Validate() form: messages rather than an
// error. It is the only check callers get when they never run the CLI's own
// flag validation - the --remote path, and library callers building a
// Function directly.
func validateExpose(expose string) (errors []string) {
	if err := ValidateExpose(expose); err != nil {
		errors = append(errors, fmt.Sprintf(
			"specified option \"deploy.expose=%s\" is not valid, allowed values are \"route\", \"none\" or empty", expose))
	}
	return
}
