package utils

import (
	"errors"
	"strings"

	"k8s.io/apimachinery/pkg/util/validation"
)

// ValidateFunctionName validatest that the input name is a valid function name, ie. valid DNS-1123 label.
// It must consist of lower case alphanumeric characters or '-' and start and end with an alphanumeric character
// (e.g. 'my-name',  or '123-abc', regex used for validation is '[a-z0-9]([-a-z0-9]*[a-z0-9])?')
func ValidateFunctionName(name string) error {

	if errs := validation.IsDNS1123Label(name); len(errs) > 0 {
		// In case of invalid name the error is this:
		//	"a DNS-1123 label must consist of lower case alphanumeric characters or '-',
		//   and must start and end with an alphanumeric character (e.g. 'my-name',
		//   or '123-abc', regex used for validation is '[a-z0-9]([-a-z0-9]*[a-z0-9])?')"
		// Let's reuse it for our purposes, ie. replace "DNS-1123 label" substring with "function name"
		return errors.New(strings.Replace(strings.Join(errs, ""), "a DNS-1123 label", "Function name", 1))
	}

	return nil
}
