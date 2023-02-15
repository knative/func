package tekton

import (
	"errors"
	"fmt"

	"knative.dev/func/pkg/builders"
	"knative.dev/func/pkg/builders/s2i"
	fn "knative.dev/func/pkg/functions"
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
	if f.Build.Builder == builders.Pack {
		if f.Runtime == "" {
			return ErrRuntimeRequired
		}

		if f.Runtime == "go" || f.Runtime == "rust" {
			return ErrRuntimeNotSupported{f.Runtime}
		}

		if len(f.Build.Buildpacks) > 0 {
			return ErrBuilpacksNotSupported
		}
	} else if f.Build.Builder == builders.S2I {
		_, err := s2i.BuilderImage(f, builders.S2I)
		return err
	} else {
		return builders.ErrUnknownBuilder{Name: f.Build.Builder}
	}

	return nil
}
