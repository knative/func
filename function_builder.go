package function

import (
	"fmt"
)

const (
	BuilderPack    = "pack"
	BuilderS2i     = "s2i"
	DefaultBuilder = BuilderPack
)

func AllBuilders() []string {
	return []string{BuilderPack, BuilderS2i}
}

// ErrInvalidBuilder indicates that the passed builder was invalid.
type ErrInvalidBuilder error

// ValidateBuilder validatest that the input Build type is valid for deploy command
func ValidateBuilder(builder string) error {

	if builder == BuilderPack || builder == BuilderS2i {
		return nil
	}

	return ErrInvalidBuilder(fmt.Errorf("specified builder %q is not valid, allowed builders are %s", builder, SupportedBuilders()))
}

// SupportedBuilders prints string with supported build types
func SupportedBuilders() string {
	return fmt.Sprintf("%q or %q", BuilderPack, BuilderS2i)
}
