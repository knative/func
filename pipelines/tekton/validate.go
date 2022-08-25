package tekton

import (
	"errors"
	"fmt"

	fn "knative.dev/kn-plugin-func"
	"knative.dev/kn-plugin-func/builders"
	"knative.dev/kn-plugin-func/s2i"
)

var (
	// ErrRuntimeRequired indicates the required value of Function Runtime was not provided
	ErrRuntimeRequired = errors.New("runtime is required to build")

	ErrBuilpacksNotSupported = errors.New("additional Buildpacks are not supported for on cluster build")
)

type ErrRuntimeNotSupported struct {
	Runtime string
}

func (e ErrRuntimeNotSupported) Error() string {
	return fmt.Sprintf("runtime %q is not supported for on cluster build", e.Runtime)
}

func validatePipeline(f fn.Function) error {
	if f.Builder == builders.Pack {
		if f.Runtime == "" {
			return ErrRuntimeRequired
		}

		if f.Runtime == "go" || f.Runtime == "rust" {
			return ErrRuntimeNotSupported{f.Runtime}
		}

		if len(f.Buildpacks) > 0 {
			return ErrBuilpacksNotSupported
		}
	} else if f.Builder == builders.S2I {
		_, err := s2i.BuilderImage(f, builders.S2I)
		return err
	} else {
		return builders.ErrUnknownBuilder{Name: f.Builder}
	}

	return nil
}
