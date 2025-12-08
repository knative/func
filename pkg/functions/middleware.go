package functions

import (
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
)

// MiddlewareVersion gets the used middleware version of a function image.
// Returns an empty string and no error in case the function image was built
// without this information.
func MiddlewareVersion(image string) (string, error) {
	ref, err := name.ParseReference(image)
	if err != nil {
		return "", err
	}

	desc, err := remote.Get(ref)
	if err != nil {
		return "", err
	}

	img, err := desc.Image()
	if err != nil {
		return "", err
	}

	cfg, err := img.ConfigFile()
	if err != nil {
		return "", err
	}

	if cfg.Config.Labels == nil {
		return "", nil
	}

	return cfg.Config.Labels[MiddlewareVersionLabelKey], nil
}
