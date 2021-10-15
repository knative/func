package lifecycle

import (
	"fmt"
	"path/filepath"

	"github.com/buildpacks/imgutil"
	"github.com/pkg/errors"

	"github.com/buildpacks/lifecycle/buildpack"
	"github.com/buildpacks/lifecycle/launch"
	"github.com/buildpacks/lifecycle/platform"
)

type Analyzer struct {
	Buildpacks []buildpack.GroupBuildpack
	LayersDir  string
	Logger     Logger
	SkipLayers bool
}

// Analyze restores metadata for launch and cache layers into the layers directory.
// If a usable cache is not provided, Analyze will not restore any cache=true layer metadata.
func (a *Analyzer) Analyze(image imgutil.Image, cache Cache) (platform.AnalyzedMetadata, error) {
	imageID, err := a.getImageIdentifier(image)
	if err != nil {
		return platform.AnalyzedMetadata{}, errors.Wrap(err, "retrieving image identifier")
	}

	var appMeta platform.LayersMetadata
	// continue even if the label cannot be decoded
	if err := DecodeLabel(image, platform.LayerMetadataLabel, &appMeta); err != nil {
		appMeta = platform.LayersMetadata{}
	}

	for _, bp := range a.Buildpacks {
		if store := appMeta.MetadataForBuildpack(bp.ID).Store; store != nil {
			if err := WriteTOML(filepath.Join(a.LayersDir, launch.EscapeID(bp.ID), "store.toml"), store); err != nil {
				return platform.AnalyzedMetadata{}, err
			}
		}
	}

	if a.SkipLayers {
		a.Logger.Infof("Skipping buildpack layer analysis")
	} else if err := a.analyzeLayers(appMeta, cache); err != nil {
		return platform.AnalyzedMetadata{}, err
	}

	return platform.AnalyzedMetadata{
		Image:    imageID,
		Metadata: appMeta,
	}, nil
}

func (a *Analyzer) analyzeLayers(appMeta platform.LayersMetadata, cache Cache) error {
	// Create empty cache metadata in case a usable cache is not provided.
	var cacheMeta platform.CacheMetadata
	if cache != nil {
		var err error
		if !cache.Exists() {
			a.Logger.Info("Layer cache not found")
		}
		cacheMeta, err = cache.RetrieveMetadata()
		if err != nil {
			return errors.Wrap(err, "retrieving cache metadata")
		}
	} else {
		a.Logger.Debug("Usable cache not provided, using empty cache metadata.")
	}

	for _, buildpack := range a.Buildpacks {
		buildpackDir, err := readBuildpackLayersDir(a.LayersDir, buildpack, a.Logger)
		if err != nil {
			return errors.Wrap(err, "reading buildpack layer directory")
		}

		// Restore metadata for launch=true layers.
		// The restorer step will restore the layer data for cache=true layers if possible or delete the layer.
		appLayers := appMeta.MetadataForBuildpack(buildpack.ID).Layers
		cachedLayers := cacheMeta.MetadataForBuildpack(buildpack.ID).Layers
		for name, layer := range appLayers {
			identifier := fmt.Sprintf("%s:%s", buildpack.ID, name)
			if !layer.Launch {
				a.Logger.Debugf("Not restoring metadata for %q, marked as launch=false", identifier)
				continue
			}
			if layer.Build && !layer.Cache {
				// layer is launch=true, build=true. Because build=true, the layer contents must be present in the build container.
				// There is no reason to restore the metadata file, because the buildpack will always recreate the layer.
				a.Logger.Debugf("Not restoring metadata for %q, marked as build=true, cache=false", identifier)
				continue
			}
			if layer.Cache {
				if cacheLayer, ok := cachedLayers[name]; !ok || !cacheLayer.Cache {
					// The layer is not cache=true in the cache metadata and will not be restored.
					// Do not write the metadata file so that it is clear to the buildpack that it needs to recreate the layer.
					// Although a launch=true (only) layer still needs a metadata file, the restorer will remove the file anyway when it does its cleanup (for buildpack apis < 0.6).
					a.Logger.Debugf("Not restoring metadata for %q, marked as cache=true, but not found in cache", identifier)
					continue
				}
			}
			a.Logger.Infof("Restoring metadata for %q from app image", identifier)
			if err := a.writeLayerMetadata(buildpackDir, name, layer); err != nil {
				return err
			}
		}

		// Restore metadata for cache=true layers.
		// The restorer step will restore the layer data if possible or delete the layer.
		for name, layer := range cachedLayers {
			identifier := fmt.Sprintf("%s:%s", buildpack.ID, name)
			if !layer.Cache {
				a.Logger.Debugf("Not restoring %q from cache, marked as cache=false", identifier)
				continue
			}
			// If launch=true, the metadata was restored from the app image or the layer is stale.
			if layer.Launch {
				a.Logger.Debugf("Not restoring %q from cache, marked as launch=true", identifier)
				continue
			}
			a.Logger.Infof("Restoring metadata for %q from cache", identifier)
			if err := a.writeLayerMetadata(buildpackDir, name, layer); err != nil {
				return err
			}
		}
	}
	return nil
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

func (a *Analyzer) writeLayerMetadata(buildpackDir bpLayersDir, name string, metadata platform.BuildpackLayerMetadata) error {
	layer := buildpackDir.newBPLayer(name, buildpackDir.buildpack.API, a.Logger)
	a.Logger.Debugf("Writing layer metadata for %q", layer.Identifier())
	if err := layer.writeMetadata(metadata.LayerMetadataFile); err != nil {
		return err
	}
	return layer.writeSha(metadata.SHA)
}
