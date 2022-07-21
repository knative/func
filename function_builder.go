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

func ValidateBuilder(builder string, allowUnset bool) (errors []string) {
	valid := false

	switch builder {
	case BuilderPack, BuilderS2i:
		valid = true
	case "":
		if allowUnset {
			valid = true
		}
	}

	if !valid {
		return []string{fmt.Sprintf("specified builder %q is not valid, allowed builders are %s", builder, SupportedBuilders())}
	}
	return
}

// SupportedBuilders prints string with supported build types
func SupportedBuilders() string {
	return fmt.Sprintf("%q or %q", BuilderPack, BuilderS2i)
}
