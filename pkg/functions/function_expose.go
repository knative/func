package functions

import (
	"fmt"
	"strings"
)

// ParseExpose splits an expose value into technology ("gateway"|"none") and
// optional reference on the FIRST ":" - unambiguous, since k8s
// namespace/name values cannot contain ":". The reference is checked for
// shape only ("namespace/name" or "namespace/"); no cluster lookups.
func ParseExpose(expose string) (tech, ref string, err error) {
	if expose == "" || expose == "none" {
		return expose, "", nil
	}

	tech, ref, _ = strings.Cut(expose, ":")

	if tech != "gateway" {
		return "", "", fmt.Errorf("%w: %q", ErrInvalidExpose, expose)
	}

	if ref != "" {
		if ns, _, found := strings.Cut(ref, "/"); !found || ns == "" {
			return "", "", fmt.Errorf(
				"%w: %q (gateway reference must be \"namespace/name\" or \"namespace/\", got %q)", ErrInvalidExpose, expose, ref)
		}
	}

	return tech, ref, nil
}

// validateExpose delegates to ParseExpose(), the single source of truth for
// valid expose values.
func validateExpose(expose string) (errors []string) {
	if _, _, err := ParseExpose(expose); err != nil {
		errors = append(errors, err.Error())
	}
	return
}
