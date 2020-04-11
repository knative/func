package k8s

import (
	"errors"
	"strings"

	"k8s.io/apimachinery/pkg/util/validation"
)

// ToSubdomain converts a domain to a subdomain.
// If the input is not a valid domain an error is thrown.
func ToSubdomain(in string) (string, error) {
	if err := validation.IsFullyQualifiedDomainName(nil, in); err != nil {
		return "", err.ToAggregate()
	}

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
	return string(out), nil
}

// FromSubdomain converts a doman which has been encoded as
// a subdomain using the algorithm of ToSubdoman back to a domain.
// Input errors if not a 1035 label.
func FromSubdomain(in string) (string, error) {
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
