package lifecycle

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/uuid"

	"github.com/buildpacks/lifecycle/buildpack"
	"github.com/buildpacks/lifecycle/internal/extend"
	"github.com/buildpacks/lifecycle/launch"
	"github.com/buildpacks/lifecycle/log"
)

const buildIDKey = "build_id"

type Extender struct {
	AppDir            string
	GeneratedDir      string
	GroupPath         string
	ImageRef          string
	LayersDir         string
	PlatformDir       string
	CacheTTL          time.Duration
	DockerfileApplier DockerfileApplier
	Extensions        []buildpack.GroupElement
	Logger            log.Logger
}

//go:generate mockgen -package testmock -destination testmock/dockerfile_applier.go github.com/buildpacks/lifecycle DockerfileApplier
type DockerfileApplier interface {
	Apply(workspace string, digest string, dockerfiles []extend.Dockerfile, options extend.Options) error
}

type ExtenderFactory struct {
	apiVerifier   BuildpackAPIVerifier
	configHandler ConfigHandler
}

func NewExtenderFactory(apiVerifier BuildpackAPIVerifier, configHandler ConfigHandler) *ExtenderFactory {
	return &ExtenderFactory{
		apiVerifier:   apiVerifier,
		configHandler: configHandler,
	}
}

func (f *ExtenderFactory) NewExtender(
	analyzedPath string,
	appDir string,
	generatedDir string,
	groupPath string,
	layersDir string,
	platformDir string,
	cacheTTL time.Duration,
	dockerfileApplier DockerfileApplier,
	logger log.Logger,
) (*Extender, error) {
	extender := &Extender{
		AppDir:            appDir,
		GeneratedDir:      generatedDir,
		LayersDir:         layersDir,
		PlatformDir:       platformDir,
		CacheTTL:          cacheTTL,
		DockerfileApplier: dockerfileApplier,
		Logger:            logger,
	}
	if err := f.setImageRef(extender, analyzedPath); err != nil {
		return nil, err
	}
	if err := f.setExtensions(extender, groupPath, logger); err != nil {
		return nil, err
	}
	return extender, nil
}

func (f *ExtenderFactory) setImageRef(extender *Extender, path string) error {
	analyzedMD, err := f.configHandler.ReadAnalyzed(path)
	if err != nil {
		return err
	}
	if analyzedMD.BuildImage != nil {
		extender.ImageRef = analyzedMD.BuildImage.Reference
	}
	return nil
}

func (f *ExtenderFactory) setExtensions(extender *Extender, path string, logger log.Logger) error {
	_, groupExt, err := f.configHandler.ReadGroup(path)
	if err != nil {
		return fmt.Errorf("reading group: %w", err)
	}
	for i := range groupExt {
		groupExt[i].Extension = true
	}
	if err = f.verifyAPIs(groupExt, logger); err != nil {
		return err
	}
	extender.Extensions = groupExt
	return nil
}

func (f *ExtenderFactory) verifyAPIs(groupExt []buildpack.GroupElement, logger log.Logger) error {
	for _, groupEl := range groupExt {
		if err := f.apiVerifier.VerifyBuildpackAPI(groupEl.Kind(), groupEl.String(), groupEl.API, logger); err != nil {
			return err
		}
	}
	return nil
}

func (e *Extender) ExtendBuild() error {
	e.Logger.Debugf("Extending %s", e.ImageRef)
	var dockerfiles []extend.Dockerfile
	for _, ext := range e.Extensions {
		buildDockerfile, err := e.buildDockerfileFor(ext.ID)
		if err != nil {
			return err
		}
		if buildDockerfile != nil {
			e.Logger.Debugf("Found build Dockerfile for extension '%s'", ext.ID)
			dockerfiles = append(dockerfiles, *buildDockerfile)
		}
	}
	options := extend.Options{
		CacheTTL:    e.CacheTTL,
		IgnorePaths: []string{e.AppDir, e.LayersDir, e.PlatformDir},
	}
	digest, err := name.NewDigest(e.ImageRef)
	if err != nil {
		return fmt.Errorf("failed to get digest for reference %s: %w", e.ImageRef, err)
	}
	return e.DockerfileApplier.Apply(e.AppDir, digest.DigestStr(), dockerfiles, options)
}

func (e *Extender) buildDockerfileFor(extID string) (*extend.Dockerfile, error) {
	dockerfilePath := filepath.Join(e.GeneratedDir, "build", launch.EscapeID(extID), "Dockerfile")
	if _, err := os.Stat(dockerfilePath); err != nil {
		return nil, nil
	}

	configPath := filepath.Join(e.GeneratedDir, "build", launch.EscapeID(extID), "extend-config.toml")
	var config buildpack.ExtendConfig
	_, err := toml.DecodeFile(configPath, &config)
	if err != nil {
		if !os.IsNotExist(err) {
			return nil, err
		}
	}

	var args []extend.Arg
	for _, configArg := range config.Build.Args {
		if configArg.Name == buildIDKey {
			return nil, errors.New("image extension provides build arg with key 'build_id' which is not allowed")
		}
		args = append(args, extend.Arg{
			Name:  configArg.Name,
			Value: configArg.Value,
		})
	}
	// prepend build_id to args
	args = append(args, extend.Arg{Name: buildIDKey, Value: uuid.New().String()})

	return &extend.Dockerfile{
		Path: dockerfilePath,
		Args: args,
	}, nil
}
