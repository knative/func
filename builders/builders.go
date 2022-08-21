/*
	Package builders provides constants for builder implementation short names,
	shared error types and convienience functions.
*/
package builders

import (
	"fmt"

	fn "knative.dev/kn-plugin-func"
)

const (
	Pack    = "pack"
	S2I     = "s2i"
	Default = Pack
)

// All builder short names as a slice.
func All() []string {
	return []string{Pack, S2I}
}

// ErrRuntimeRequired
type ErrRuntimeRequired struct {
	Builder string
}

func (e ErrRuntimeRequired) Error() string {
	return fmt.Sprintf("runtime required to choose a default '%v' builder image", e.Builder)
}

// ErrUnknownRuntime
type ErrUnknownRuntime struct {
	Builder string
	Runtime string
}

func (e ErrUnknownRuntime) Error() string {
	return fmt.Sprintf("'%v' is not a known language runtime for the '%v' builder", e.Runtime, e.Builder)
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
// - ErrRuntimeRequired if no runtime was provided on the given function
// - ErrNoDefaultImage if the function has no builder image already defined
//   for the given runtieme and there is no default in the provided map.
func Image(f fn.Function, builder string, defaults map[string]string) (string, error) {
	v, ok := f.BuilderImages[builder]
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
