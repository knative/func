package k8s

import (
	"errors"
	"strings"

	"k8s.io/apimachinery/pkg/util/validation"
)

// ToK8sAllowedName converts a name to a name that's allowed for k8s service.
// k8s does not support service names with dots.  So encode it such that
// www.my-domain,com -> www-my--domain-com
// Input errors if not a 1035 label.
// "a DNS-1035 label must consist of lower case alphanumeric characters or '-',
// start with an alphabetic character, and end with an alphanumeric character"
func ToK8sAllowedName(in string) (string, error) {

	out := []rune{}
	for _, c := range in {
		// convert dots to hyphens
		if c == '.' {
			out = append(out, '-')
		} else if c == '-' {
			out = append(out, '-')
			out = append(out, '-')
		} else {
			out = append(out, c)
		}
	}

	result := string(out)

	if errs := validation.IsDNS1035Label(result); len(errs) > 0 {
		return "", errors.New(strings.Join(errs, ","))
	}

	return result, nil
}

// FromK8sAllowedName converts a name which has been encoded as
// an allowed k8s name using the algorithm of ToK8sAllowedName back to the original.
// www-my--domain-com -> www.my-domain.com
// Input errors if not a 1035 label.
func FromK8sAllowedName(in string) (string, error) {

	if errs := validation.IsDNS1035Label(in); len(errs) > 0 {
		return "", errors.New(strings.Join(errs, ","))
	}

	rr := []rune(in)
	out := []rune{}

	for i := 0; i < len(rr); i++ {
		c := rr[i]
		if c == '-' {
			// If the next rune is either nonexistent
			// or not also a dash, this is an encoded dot.
			if i+1 == len(rr) || rr[i+1] != '-' {
				out = append(out, '.')
				continue
			}

			// If the next rune is also a dash, this is
			// an escaping dash, so append a slash, and
			// increment the pointer such that the next
			// loop begins with the next potential tuple.
			if rr[i+1] == '-' {
				out = append(out, '-')
				i++
				continue
			}
		}
		out = append(out, c)
	}

	return string(out), nil
}
