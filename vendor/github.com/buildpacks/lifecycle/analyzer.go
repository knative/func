package lifecycle

import (
	"github.com/buildpacks/imgutil"
	"github.com/pkg/errors"

	"github.com/buildpacks/lifecycle/api"
	"github.com/buildpacks/lifecycle/buildpack"
	"github.com/buildpacks/lifecycle/platform"
)

type Platform interface {
	API() string
}

type Analyzer struct {
	PreviousImage imgutil.Image
	RunImage      imgutil.Image
	Logger        Logger
	Platform      Platform

	// Platform API < 0.7
	Buildpacks            []buildpack.GroupBuildpack
	Cache                 Cache
	LayerMetadataRestorer LayerMetadataRestorer
}

// Analyze fetches the layers metadata from the previous image and writes analyzed.toml.
func (a *Analyzer) Analyze() (platform.AnalyzedMetadata, error) {
	var (
		appMeta         platform.LayersMetadata
		cacheMeta       platform.CacheMetadata
		previousImageID *platform.ImageIdentifier
		runImageID      *platform.ImageIdentifier
		err             error
	)

	if a.PreviousImage != nil { // Previous image is optional in Platform API >= 0.7
		previousImageID, err = a.getImageIdentifier(a.PreviousImage)
		if err != nil {
			return platform.AnalyzedMetadata{}, errors.Wrap(err, "retrieving image identifier")
		}

		// continue even if the label cannot be decoded
		if err := DecodeLabel(a.PreviousImage, platform.LayerMetadataLabel, &appMeta); err != nil {
			appMeta = platform.LayersMetadata{}
		}
	} else {
		appMeta = platform.LayersMetadata{}
	}

	if a.RunImage != nil {
		runImageID, err = a.getImageIdentifier(a.RunImage)
		if err != nil {
			return platform.AnalyzedMetadata{}, errors.Wrap(err, "retrieving image identifier")
		}
	}

	if a.restoresLayerMetadata() {
		cacheMeta, err = retrieveCacheMetadata(a.Cache, a.Logger)
		if err != nil {
			return platform.AnalyzedMetadata{}, err
		}

		useShaFiles := true
		if err := a.LayerMetadataRestorer.Restore(a.Buildpacks, appMeta, cacheMeta, NewLayerSHAStore(useShaFiles)); err != nil {
			return platform.AnalyzedMetadata{}, err
		}
	}

	return platform.AnalyzedMetadata{
		PreviousImage: previousImageID,
		RunImage:      runImageID,
		Metadata:      appMeta,
	}, nil
}

func (a *Analyzer) restoresLayerMetadata() bool {
	return api.MustParse(a.Platform.API()).LessThan("0.7")
}

func (a *Analyzer) getImageIdentifier(image imgutil.Image) (*platform.ImageIdentifier, error) {
	if !image.Found() {
		a.Logger.Infof("Previous image with name %q not found", image.Name())
		return nil, nil
	}
	identifier, err := image.Identifier()
	if err != nil {
		return nil, err
	}
	a.Logger.Debugf("Analyzing image %q", identifier.String())
	return &platform.ImageIdentifier{
		Reference: identifier.String(),
	}, nil
}
