package lifecycle

import (
	"github.com/buildpacks/imgutil"
	"github.com/pkg/errors"

	"github.com/buildpacks/lifecycle/api"
	"github.com/buildpacks/lifecycle/buildpack"
	"github.com/buildpacks/lifecycle/cache"
	"github.com/buildpacks/lifecycle/image"
	"github.com/buildpacks/lifecycle/internal/layer"
	"github.com/buildpacks/lifecycle/log"
	"github.com/buildpacks/lifecycle/platform"
)

type AnalyzerFactory struct {
	platformAPI     *api.Version
	apiVerifier     BuildpackAPIVerifier
	cacheHandler    CacheHandler
	configHandler   ConfigHandler
	imageHandler    ImageHandler
	registryHandler RegistryHandler
}

func NewAnalyzerFactory(
	platformAPI *api.Version,
	apiVerifier BuildpackAPIVerifier,
	cacheHandler CacheHandler,
	configHandler ConfigHandler,
	imageHandler ImageHandler,
	registryHandler RegistryHandler,
) *AnalyzerFactory {
	return &AnalyzerFactory{
		platformAPI:     platformAPI,
		apiVerifier:     apiVerifier,
		cacheHandler:    cacheHandler,
		configHandler:   configHandler,
		imageHandler:    imageHandler,
		registryHandler: registryHandler,
	}
}

type Analyzer struct {
	PreviousImage imgutil.Image
	RunImage      imgutil.Image
	Logger        log.Logger
	SBOMRestorer  layer.SBOMRestorer

	// Platform API < 0.7
	Buildpacks            []buildpack.GroupElement
	Cache                 Cache
	LayerMetadataRestorer layer.MetadataRestorer
	RestoresLayerMetadata bool
}

func (f *AnalyzerFactory) NewAnalyzer(
	additionalTags []string,
	cacheImageRef string,
	launchCacheDir string,
	layersDir string,
	legacyCacheDir string,
	legacyGroup buildpack.Group,
	legacyGroupPath string,
	outputImageRef string,
	previousImageRef string,
	runImageRef string,
	skipLayers bool,
	logger log.Logger,
) (*Analyzer, error) {
	analyzer := &Analyzer{
		LayerMetadataRestorer: &layer.NopMetadataRestorer{},
		Logger:                logger,
		SBOMRestorer:          &layer.NopSBOMRestorer{},
	}

	if f.platformAPI.AtLeast("0.7") {
		if err := f.ensureRegistryAccess(additionalTags, cacheImageRef, outputImageRef, runImageRef, previousImageRef); err != nil {
			return nil, err
		}
	} else {
		if err := f.setBuildpacks(analyzer, legacyGroup, legacyGroupPath, logger); err != nil {
			return nil, err
		}
		if err := f.setCache(analyzer, cacheImageRef, legacyCacheDir); err != nil {
			return nil, err
		}
		analyzer.LayerMetadataRestorer = layer.NewDefaultMetadataRestorer(layersDir, skipLayers, logger)
		analyzer.RestoresLayerMetadata = true
	}

	if f.platformAPI.AtLeast("0.8") && !skipLayers {
		analyzer.SBOMRestorer = &layer.DefaultSBOMRestorer{ // FIXME: eventually layer.NewSBOMRestorer should always return the default one, and then we can use the constructor
			LayersDir: layersDir,
			Logger:    logger,
		}
	}

	if err := f.setPrevious(analyzer, previousImageRef, launchCacheDir); err != nil {
		return nil, err
	}
	if err := f.setRun(analyzer, runImageRef); err != nil {
		return nil, err
	}
	return analyzer, nil
}

func (f *AnalyzerFactory) ensureRegistryAccess(
	additionalTags []string,
	cacheImageRef string,
	outputImageRef string,
	runImageRef string,
	previousImageRef string,
) error {
	var readImages, writeImages []string
	writeImages = append(writeImages, cacheImageRef)
	if !f.imageHandler.Docker() {
		readImages = append(readImages, previousImageRef, runImageRef)
		writeImages = append(writeImages, outputImageRef)
		writeImages = append(writeImages, additionalTags...)
	}

	if err := f.registryHandler.EnsureReadAccess(readImages...); err != nil {
		return errors.Wrap(err, "validating registry read access")
	}
	if err := f.registryHandler.EnsureWriteAccess(writeImages...); err != nil {
		return errors.Wrap(err, "validating registry write access")
	}
	return nil
}

func (f *AnalyzerFactory) setBuildpacks(analyzer *Analyzer, group buildpack.Group, path string, logger log.Logger) error {
	if len(group.Group) > 0 {
		analyzer.Buildpacks = group.Group
		return nil
	}
	var err error
	if analyzer.Buildpacks, _, err = f.configHandler.ReadGroup(path); err != nil {
		return err
	}
	for _, bp := range analyzer.Buildpacks {
		if err := f.apiVerifier.VerifyBuildpackAPI(buildpack.KindBuildpack, bp.String(), bp.API, logger); err != nil {
			return err
		}
	}
	return nil
}

func (f *AnalyzerFactory) setCache(analyzer *Analyzer, imageRef string, dir string) error {
	var err error
	analyzer.Cache, err = f.cacheHandler.InitCache(imageRef, dir)
	return err
}

func (f *AnalyzerFactory) setPrevious(analyzer *Analyzer, imageRef string, launchCacheDir string) error {
	if imageRef == "" {
		return nil
	}
	var err error
	analyzer.PreviousImage, err = f.imageHandler.InitImage(imageRef)
	if err != nil {
		return errors.Wrap(err, "getting previous image")
	}
	if launchCacheDir == "" || !f.imageHandler.Docker() {
		return nil
	}

	volumeCache, err := cache.NewVolumeCache(launchCacheDir)
	if err != nil {
		return errors.Wrap(err, "creating launch cache")
	}
	analyzer.PreviousImage = cache.NewCachingImage(analyzer.PreviousImage, volumeCache)
	return nil
}

func (f *AnalyzerFactory) setRun(analyzer *Analyzer, imageRef string) error {
	if imageRef == "" {
		return nil
	}
	var err error
	analyzer.RunImage, err = f.imageHandler.InitImage(imageRef)
	if err != nil {
		return errors.Wrap(err, "getting run image")
	}
	return nil
}

// Analyze fetches the layers metadata from the previous image and writes analyzed.toml.
func (a *Analyzer) Analyze() (platform.AnalyzedMetadata, error) {
	var (
		err             error
		appMeta         platform.LayersMetadata
		cacheMeta       platform.CacheMetadata
		previousImageID *platform.ImageIdentifier
		runImageID      *platform.ImageIdentifier
	)

	if a.PreviousImage != nil { // Previous image is optional in Platform API >= 0.7
		if previousImageID, err = a.getImageIdentifier(a.PreviousImage); err != nil {
			return platform.AnalyzedMetadata{}, errors.Wrap(err, "retrieving image identifier")
		}

		// continue even if the label cannot be decoded
		if err = image.DecodeLabel(a.PreviousImage, platform.LayerMetadataLabel, &appMeta); err != nil {
			appMeta = platform.LayersMetadata{}
		}

		if err = a.SBOMRestorer.RestoreFromPrevious(a.PreviousImage, bomSHA(appMeta)); err != nil {
			return platform.AnalyzedMetadata{}, errors.Wrap(err, "retrieving launch SBOM layer")
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

	if a.RestoresLayerMetadata {
		cacheMeta, err = retrieveCacheMetadata(a.Cache, a.Logger)
		if err != nil {
			return platform.AnalyzedMetadata{}, err
		}

		useShaFiles := true
		if err := a.LayerMetadataRestorer.Restore(a.Buildpacks, appMeta, cacheMeta, layer.NewSHAStore(useShaFiles)); err != nil {
			return platform.AnalyzedMetadata{}, err
		}
	}

	return platform.AnalyzedMetadata{
		PreviousImage: previousImageID,
		RunImage:      runImageID,
		Metadata:      appMeta,
	}, nil
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

func bomSHA(appMeta platform.LayersMetadata) string {
	if appMeta.BOM == nil {
		return ""
	}
	return appMeta.BOM.SHA
}

func retrieveCacheMetadata(fromCache Cache, logger log.Logger) (platform.CacheMetadata, error) {
	// Create empty cache metadata in case a usable cache is not provided.
	var cacheMeta platform.CacheMetadata
	if fromCache != nil {
		var err error
		if !fromCache.Exists() {
			logger.Info("Layer cache not found")
		}
		cacheMeta, err = fromCache.RetrieveMetadata()
		if err != nil {
			return cacheMeta, errors.Wrap(err, "retrieving cache metadata")
		}
	} else {
		logger.Debug("Usable cache not provided, using empty cache metadata")
	}

	return cacheMeta, nil
}
