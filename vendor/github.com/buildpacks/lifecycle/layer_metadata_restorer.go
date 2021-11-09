package lifecycle

import (
	"fmt"
	"path/filepath"

	"github.com/pkg/errors"

	"github.com/buildpacks/lifecycle/buildpack"
	"github.com/buildpacks/lifecycle/launch"
	"github.com/buildpacks/lifecycle/platform"
)

//go:generate mockgen -package testmock -destination testmock/layer_metadata_restorer.go github.com/buildpacks/lifecycle LayerMetadataRestorer
type LayerMetadataRestorer interface {
	Restore(buildpacks []buildpack.GroupBuildpack, appMeta platform.LayersMetadata, cacheMeta platform.CacheMetadata, layerSHAStore LayerSHAStore) error
}

type DefaultLayerMetadataRestorer struct {
	logger     Logger
	layersDir  string
	skipLayers bool
}

type LayerSHAStore interface {
	add(buildpackID, sha string, layer *bpLayer) error
	get(buildpackID string, layer bpLayer) (string, error)
}

func NewLayerSHAStore(useShaFiles bool) LayerSHAStore {
	if useShaFiles {
		return &filesLayerSHAStore{}
	}
	return &mapLayerSHAStore{make(map[string]layerToSha)}
}

type filesLayerSHAStore struct {
}

func (flss *filesLayerSHAStore) add(buildpackID, sha string, layer *bpLayer) error {
	return layer.writeSha(sha)
}

func (flss *filesLayerSHAStore) get(buildpackID string, layer bpLayer) (string, error) {
	data, err := layer.read()
	if err != nil {
		return "", errors.Wrapf(err, "reading layer")
	}
	return data.SHA, nil
}

type mapLayerSHAStore struct {
	buildpacksToLayersShaMap map[string]layerToSha
}

type layerToSha struct {
	layerToShaMap map[string]string
}

func (mlss *mapLayerSHAStore) add(buildpackID, sha string, layer *bpLayer) error {
	mlss.addLayerToMap(buildpackID, layer.name(), sha)
	return nil
}

func (mlss *mapLayerSHAStore) get(buildpackID string, layer bpLayer) (string, error) {
	return mlss.getShaByBuildpackLayer(buildpackID, layer.name()), nil
}

func (mlss *mapLayerSHAStore) addLayerToMap(buildpackID, layerName, sha string) {
	_, exists := mlss.buildpacksToLayersShaMap[buildpackID]
	if !exists {
		mlss.buildpacksToLayersShaMap[buildpackID] = layerToSha{make(map[string]string)}
	}
	mlss.buildpacksToLayersShaMap[buildpackID].layerToShaMap[layerName] = sha
}

// if the layer exists for the buildpack ID, its SHA will be returned. Otherwise, an empty string will be returned.
func (mlss *mapLayerSHAStore) getShaByBuildpackLayer(buildpackID, layerName string) string {
	if layerToSha, buildpackExists := mlss.buildpacksToLayersShaMap[buildpackID]; buildpackExists {
		if sha, layerExists := layerToSha.layerToShaMap[layerName]; layerExists {
			return sha
		}
	}
	return ""
}

func NewLayerMetadataRestorer(logger Logger, layersDir string, skipLayers bool) LayerMetadataRestorer {
	return &DefaultLayerMetadataRestorer{
		logger:     logger,
		layersDir:  layersDir,
		skipLayers: skipLayers,
	}
}

func (la *DefaultLayerMetadataRestorer) Restore(buildpacks []buildpack.GroupBuildpack, appMeta platform.LayersMetadata, cacheMeta platform.CacheMetadata, layerSHAStore LayerSHAStore) error {
	if err := la.restoreStoreTOML(appMeta, buildpacks); err != nil {
		return err
	}

	if err := la.restoreLayerMetadata(layerSHAStore, appMeta, cacheMeta, buildpacks); err != nil {
		return err
	}

	return nil
}

func (la *DefaultLayerMetadataRestorer) restoreStoreTOML(appMeta platform.LayersMetadata, buildpacks []buildpack.GroupBuildpack) error {
	for _, bp := range buildpacks {
		if store := appMeta.MetadataForBuildpack(bp.ID).Store; store != nil {
			if err := WriteTOML(filepath.Join(la.layersDir, launch.EscapeID(bp.ID), "store.toml"), store); err != nil {
				return err
			}
		}
	}
	return nil
}

func (la *DefaultLayerMetadataRestorer) restoreLayerMetadata(layerSHAStore LayerSHAStore, appMeta platform.LayersMetadata, cacheMeta platform.CacheMetadata, buildpacks []buildpack.GroupBuildpack) error {
	if la.skipLayers {
		la.logger.Infof("Skipping buildpack layer analysis")
		return nil
	}

	for _, buildpack := range buildpacks {
		buildpackDir, err := readBuildpackLayersDir(la.layersDir, buildpack, la.logger)
		if err != nil {
			return errors.Wrap(err, "reading buildpack layer directory")
		}

		// Restore metadata for launch=true layers.
		// The restorer step will restore the layer data for cache=true layers if possible or delete the layer.
		appLayers := appMeta.MetadataForBuildpack(buildpack.ID).Layers
		cachedLayers := cacheMeta.MetadataForBuildpack(buildpack.ID).Layers
		for layerName, layer := range appLayers {
			identifier := fmt.Sprintf("%s:%s", buildpack.ID, layerName)
			if !layer.Launch {
				la.logger.Debugf("Not restoring metadata for %q, marked as launch=false", identifier)
				continue
			}
			if layer.Build && !layer.Cache {
				// layer is launch=true, build=true. Because build=true, the layer contents must be present in the build container.
				// There is no reason to restore the metadata file, because the buildpack will always recreate the layer.
				la.logger.Debugf("Not restoring metadata for %q, marked as build=true, cache=false", identifier)
				continue
			}
			if layer.Cache {
				if cacheLayer, ok := cachedLayers[layerName]; !ok || !cacheLayer.Cache {
					// The layer is not cache=true in the cache metadata and will not be restored.
					// Do not write the metadata file so that it is clear to the buildpack that it needs to recreate the layer.
					// Although a launch=true (only) layer still needs a metadata file, the restorer will remove the file anyway when it does its cleanup (for buildpack apis < 0.6).
					la.logger.Debugf("Not restoring metadata for %q, marked as cache=true, but not found in cache", identifier)
					continue
				}
			}
			la.logger.Infof("Restoring metadata for %q from app image", identifier)
			if err := la.writeLayerMetadata(layerSHAStore, buildpackDir, layerName, layer, buildpack.ID); err != nil {
				return err
			}
		}

		// Restore metadata for cache=true layers.
		// The restorer step will restore the layer data if possible or delete the layer.
		for layerName, layer := range cachedLayers {
			identifier := fmt.Sprintf("%s:%s", buildpack.ID, layerName)
			if !layer.Cache {
				la.logger.Debugf("Not restoring %q from cache, marked as cache=false", identifier)
				continue
			}
			// If launch=true, the metadata was restored from the app image or the layer is stale.
			if layer.Launch {
				la.logger.Debugf("Not restoring %q from cache, marked as launch=true", identifier)
				continue
			}
			la.logger.Infof("Restoring metadata for %q from cache", identifier)
			if err := la.writeLayerMetadata(layerSHAStore, buildpackDir, layerName, layer, buildpack.ID); err != nil {
				return err
			}
		}
	}
	return nil
}

func (la *DefaultLayerMetadataRestorer) writeLayerMetadata(layerSHAStore LayerSHAStore, buildpackDir bpLayersDir, layerName string, metadata platform.BuildpackLayerMetadata, buildpackID string) error {
	layer := buildpackDir.newBPLayer(layerName, buildpackDir.buildpack.API, la.logger)
	la.logger.Debugf("Writing layer metadata for %q", layer.Identifier())
	if err := layer.writeMetadata(metadata.LayerMetadataFile); err != nil {
		return err
	}
	return layerSHAStore.add(buildpackID, metadata.SHA, layer)
}

func retrieveCacheMetadata(cache Cache, logger Logger) (platform.CacheMetadata, error) {
	// Create empty cache metadata in case a usable cache is not provided.
	var cacheMeta platform.CacheMetadata
	if cache != nil {
		var err error
		if !cache.Exists() {
			logger.Info("Layer cache not found")
		}
		cacheMeta, err = cache.RetrieveMetadata()
		if err != nil {
			return cacheMeta, errors.Wrap(err, "retrieving cache metadata")
		}
	} else {
		logger.Debug("Usable cache not provided, using empty cache metadata")
	}

	return cacheMeta, nil
}
