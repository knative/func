package functions

import (
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
)

// ImageCommit gets the commit SHA label from a function image.
// Returns an empty string and no error if the image was built without this label.
func ImageCommit(image string) (string, error) {
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

	return cfg.Config.Labels[CommitLabelKey], nil
}
