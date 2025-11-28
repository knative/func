package tekton

import (
	"errors"
	"fmt"

	"knative.dev/func/pkg/builders"
	fn "knative.dev/func/pkg/functions"
	"knative.dev/func/pkg/s2i"
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
	return fmt.Sprintf("runtime %q is not supported for on cluster build with default builders", e.Runtime)
}

func validatePipeline(f fn.Function) error {
	switch f.Build.Builder {
	case builders.Pack:
		if f.Runtime == "" {
			return ErrRuntimeRequired
		}
		if len(f.Build.Buildpacks) > 0 {
			return ErrBuilpacksNotSupported
		}
	case builders.S2I:
		_, err := s2i.BuilderImage(f, builders.S2I)
		return err
	case builders.Host:
		return fmt.Errorf("the %q builder is not supported for remote deployments. Use %q or %q instead", builders.Host, builders.Pack, builders.S2I)
	default:
		return builders.ErrUnknownBuilder{Name: f.Build.Builder, Known: builders.Known{builders.Pack, builders.S2I}}
	}

	return nil

}
