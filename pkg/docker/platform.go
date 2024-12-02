package docker

import (
	"fmt"

	"github.com/containerd/platforms"
	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	gcrTypes "github.com/google/go-containerregistry/pkg/v1/types"
)

// GetPlatformImage returns image reference for specific platform.
// If the image is not multi-arch it returns ref argument directly (provided platform matches).
// If the image is multi-arch it returns digest based reference (provided the platform is part of the multi-arch image).
func GetPlatformImage(ref, platform string) (string, error) {

	plat, err := platforms.Parse(platform)
	if err != nil {
		return "", fmt.Errorf("cannot parse platform: %w", err)
	}

	r, err := name.ParseReference(ref)
	if err != nil {
		return "", fmt.Errorf("cannot parse reference: %w", err)
	}

	desc, err := remote.Get(r)
	if err != nil {
		return "", fmt.Errorf("cannot get remote image: %w", err)
	}

	if desc.MediaType != gcrTypes.OCIImageIndex && desc.MediaType != gcrTypes.DockerManifestList {
		// it's non-multi-arch image
		var img v1.Image
		var cfg *v1.ConfigFile
		img, err = desc.Image()
		if err != nil {
			return "", fmt.Errorf("cannot get image from the descriptor: %w", err)
		}
		cfg, err = img.ConfigFile()
		if err != nil {
			return "", fmt.Errorf("cannot get config file for the image: %w", err)
		}

		if plat.OS == cfg.OS &&
			plat.Architecture == cfg.Architecture {
			return ref, nil
		}
		return "", fmt.Errorf("the %q platform is not supported by the %q image", platform, ref)
	}

	idx, err := desc.ImageIndex()
	if err != nil {
		return "", fmt.Errorf("cannot get image index: %w", err)
	}

	idxMft, err := idx.IndexManifest()
	if err != nil {
		return "", fmt.Errorf("cannot get index manifest: %w", err)
	}

	if len(idxMft.Manifests) > 1000 {
		return "", fmt.Errorf("platform image has too many manifests")
	}

	for _, manifest := range idxMft.Manifests {
		if plat.OS == manifest.Platform.OS &&
			plat.Architecture == manifest.Platform.Architecture {
			return r.Context().Name() + "@" + manifest.Digest.String(), nil
		}
	}

	return "", fmt.Errorf("the %q platform is not supported by the %q image", platform, ref)
}
