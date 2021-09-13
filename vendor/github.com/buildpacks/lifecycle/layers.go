package lifecycle

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/pkg/errors"

	"github.com/buildpacks/lifecycle/api"
	"github.com/buildpacks/lifecycle/buildpack"
	"github.com/buildpacks/lifecycle/buildpack/layertypes"
	"github.com/buildpacks/lifecycle/launch"
	"github.com/buildpacks/lifecycle/platform"
)

type bpLayersDir struct {
	path      string
	layers    []bpLayer
	name      string
	buildpack buildpack.GroupBuildpack
	store     *buildpack.StoreTOML
}

func readBuildpackLayersDir(layersDir string, bp buildpack.GroupBuildpack, logger Logger) (bpLayersDir, error) {
	path := filepath.Join(layersDir, launch.EscapeID(bp.ID))
	bpDir := bpLayersDir{
		name:      bp.ID,
		path:      path,
		layers:    []bpLayer{},
		buildpack: bp,
	}

	fis, err := ioutil.ReadDir(path)
	if err != nil && !os.IsNotExist(err) {
		return bpLayersDir{}, err
	}

	names := map[string]struct{}{}
	var tomls []string
	for _, fi := range fis {
		if fi.IsDir() {
			bpDir.layers = append(bpDir.layers, *bpDir.newBPLayer(fi.Name(), bp.API, logger))
			names[fi.Name()] = struct{}{}
			continue
		}
		if strings.HasSuffix(fi.Name(), ".toml") {
			tomls = append(tomls, filepath.Join(path, fi.Name()))
		}
	}

	for _, tf := range tomls {
		name := strings.TrimSuffix(filepath.Base(tf), ".toml")
		if name == "store" {
			var bpStore buildpack.StoreTOML
			_, err := toml.DecodeFile(tf, &bpStore)
			if err != nil {
				return bpLayersDir{}, errors.Wrapf(err, "failed decoding store.toml for buildpack %q", bp.ID)
			}
			bpDir.store = &bpStore
			continue
		}
		if name == "launch" {
			// don't treat launch.toml as a layer
			continue
		}
		if name == "build" && api.MustParse(bp.API).Compare(api.MustParse("0.5")) >= 0 {
			// if the buildpack API supports build.toml don't treat it as a layer
			continue
		}
		if _, ok := names[name]; !ok {
			bpDir.layers = append(bpDir.layers, *bpDir.newBPLayer(name, bp.API, logger))
		}
	}
	sort.Slice(bpDir.layers, func(i, j int) bool {
		return bpDir.layers[i].identifier < bpDir.layers[j].identifier
	})
	return bpDir, nil
}

func forLaunch(l bpLayer) bool {
	md, err := l.read()
	return err == nil && md.Launch
}

func forMalformed(l bpLayer) bool {
	_, err := l.read()
	return err != nil
}

func forCached(l bpLayer) bool {
	md, err := l.read()
	return err == nil && md.Cache
}

func (bd *bpLayersDir) findLayers(f func(layer bpLayer) bool) []bpLayer {
	var selectedLayers []bpLayer
	for _, l := range bd.layers {
		if f(l) {
			selectedLayers = append(selectedLayers, l)
		}
	}
	return selectedLayers
}

func (bd *bpLayersDir) newBPLayer(name, buildpackAPI string, logger Logger) *bpLayer {
	return &bpLayer{
		layer: layer{
			path:       filepath.Join(bd.path, name),
			identifier: fmt.Sprintf("%s:%s", bd.buildpack.ID, name),
		},
		api:    buildpackAPI,
		logger: logger,
	}
}

type bpLayer struct { // TODO: need to refactor so api and logger won't be part of this struct
	layer
	api    string
	logger Logger
}

func (bp *bpLayer) read() (platform.BuildpackLayerMetadata, error) {
	tomlPath := bp.path + ".toml"
	layerMetadataFile, msg, err := buildpack.DecodeLayerMetadataFile(tomlPath, bp.api)
	if err != nil {
		return platform.BuildpackLayerMetadata{}, err
	}
	if msg != "" {
		if api.MustParse(bp.api).Compare(api.MustParse("0.6")) < 0 {
			bp.logger.Warn(msg)
		} else {
			return platform.BuildpackLayerMetadata{}, errors.New(msg)
		}
	}
	sha, err := ioutil.ReadFile(bp.path + ".sha")
	if err != nil {
		if os.IsNotExist(err) {
			return platform.BuildpackLayerMetadata{LayerMetadata: platform.LayerMetadata{SHA: ""}, LayerMetadataFile: layerMetadataFile}, nil
		}
		return platform.BuildpackLayerMetadata{}, err
	}
	return platform.BuildpackLayerMetadata{LayerMetadata: platform.LayerMetadata{SHA: string(sha)}, LayerMetadataFile: layerMetadataFile}, nil
}

func (bp *bpLayer) remove() error {
	if err := os.RemoveAll(bp.path); err != nil && !os.IsNotExist(err) {
		return err
	}
	if err := os.Remove(bp.path + ".sha"); err != nil && !os.IsNotExist(err) {
		return err
	}
	if err := os.Remove(bp.path + ".toml"); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func (bp *bpLayer) writeMetadata(metadata layertypes.LayerMetadataFile) error {
	path := filepath.Join(bp.path + ".toml")
	if err := os.MkdirAll(filepath.Dir(path), 0777); err != nil {
		return err
	}
	return buildpack.EncodeLayerMetadataFile(metadata, path, bp.api)
}

func (bp *bpLayer) hasLocalContents() bool {
	_, err := ioutil.ReadDir(bp.path)

	return !os.IsNotExist(err)
}

func (bp *bpLayer) writeSha(sha string) error {
	if err := ioutil.WriteFile(bp.path+".sha", []byte(sha), 0666); err != nil {
		return err
	} // #nosec G306
	return nil
}

func (bp *bpLayer) name() string {
	return filepath.Base(bp.path)
}

type layer struct {
	path       string
	identifier string
}

func (l *layer) Identifier() string {
	return l.identifier
}

func (l *layer) Path() string {
	return l.path
}

type layerDir interface {
	Identifier() string
	Path() string
}
