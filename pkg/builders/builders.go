/*
Package builders provides constants for builder implementation short names,
shared error types and convienience functions.
*/
package builders

import (
	"fmt"
	"strconv"
	"strings"

	fn "knative.dev/func/pkg/functions"
)

const (
	Host    = "host"
	Pack    = "pack"
	S2I     = "s2i"
	Default = S2I
)

// Known builder names with a pretty-printed string representation
type Known []string

func All() Known {
	return Known([]string{Host, Pack, S2I})
}

func (k Known) String() string {
	var b strings.Builder
	for i, v := range k {
		if i < len(k)-2 {
			b.WriteString(strconv.Quote(v) + ", ")
		} else if i < len(k)-1 {
			b.WriteString(strconv.Quote(v) + " and ")
		} else {
			b.WriteString(strconv.Quote(v))
		}
	}
	return b.String()
}

// ErrUnknownBuilder may be used by whomever is choosing a concrete
// implementation of a builder to invoke based on potentially invalid input.
type ErrUnknownBuilder struct {
	Name  string
	Known Known
}

func (e ErrUnknownBuilder) Error() string {
	if len(e.Known) == 0 {
		return fmt.Sprintf("\"%v\" is not a known builder", e.Name)
	}
	if len(e.Known) == 1 {
		return fmt.Sprintf("\"%v\" is not a known builder. The available builder is %v", e.Name, e.Known)
	}
	return fmt.Sprintf("\"%v\" is not a known builder. Available builders are %s", e.Name, e.Known)
}

// ErrBuilderNotSupported
type ErrBuilderNotSupported struct {
	Builder string
}

func (e ErrBuilderNotSupported) Error() string {
	return fmt.Sprintf("builder %q is not supported", e.Builder)
}

// ErrRuntimeRequired
type ErrRuntimeRequired struct {
	Builder string
}

func (e ErrRuntimeRequired) Error() string {
	return fmt.Sprintf("runtime required to choose a default '%v' builder image", e.Builder)
}

// ErrNoDefaultImage
type ErrNoDefaultImage struct {
	Builder string
	Runtime string
}

func (e ErrNoDefaultImage) Error() string {
	return fmt.Sprintf("the '%v' runtime defines no default '%v' builder image", e.Runtime, e.Builder)
}

// Image is a convenience function for choosing the correct builder image
// given a function, a builder, and defaults grouped by runtime.
//   - ErrRuntimeRequired if no runtime was provided on the given function
//   - ErrNoDefaultImage if the function has no builder image already defined
//     for the given runtime and there is no default in the provided map.
func Image(f fn.Function, builder string, defaults map[string]string) (string, error) {
	v, ok := f.Build.BuilderImages[builder]
	if ok {
		return v, nil // found value
	}
	if f.Runtime == "" {
		return "", ErrRuntimeRequired{Builder: builder}
	}
	v, ok = defaults[f.Runtime]
	if ok {
		return v, nil // Found default
	}
	return "", ErrNoDefaultImage{Builder: builder, Runtime: f.Runtime}

}
