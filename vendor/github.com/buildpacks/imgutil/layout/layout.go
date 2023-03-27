package layout

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/go-containerregistry/pkg/v1/tarball"

	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/types"
	"github.com/pkg/errors"

	"github.com/buildpacks/imgutil"
)

var _ imgutil.Image = (*Image)(nil)

type Image struct {
	v1.Image
	path       string
	prevLayers []v1.Layer
	createdAt  time.Time
	refName    string // holds org.opencontainers.image.ref.name value
}

type imageOptions struct {
	platform      imgutil.Platform
	baseImage     v1.Image
	baseImagePath string
	prevImagePath string
	createdAt     time.Time
}

type ImageOption func(*imageOptions) error

// WithPreviousImage loads an existing image as a source for reusable layers.
// Use with ReuseLayer().
// Ignored if underlyingImage is not found.
func WithPreviousImage(path string) ImageOption {
	return func(i *imageOptions) error {
		i.prevImagePath = path
		return nil
	}
}

// FromBaseImage loads the given image as the config and layers for the new image.
// Ignored if image is not found.
func FromBaseImage(base v1.Image) ImageOption {
	return func(i *imageOptions) error {
		i.baseImage = base
		return nil
	}
}

// WithDefaultPlatform provides Architecture/OS/OSVersion defaults for the new image.
// Defaults for a new image are ignored when FromBaseImage returns an image.
// FromBaseImage and WithPreviousImage will use the platform to choose an image from a manifest list.
func WithDefaultPlatform(platform imgutil.Platform) ImageOption {
	return func(i *imageOptions) error {
		i.platform = platform
		return nil
	}
}

// WithCreatedAt lets a caller set the created at timestamp for the image.
// Defaults for a new image is imgutil.NormalizedDateTime
func WithCreatedAt(createdAt time.Time) ImageOption {
	return func(i *imageOptions) error {
		i.createdAt = createdAt
		return nil
	}
}

// FromBaseImagePath loads an existing image as the config and layers for the new underlyingImage.
// Ignored if underlyingImage is not found.
func FromBaseImagePath(path string) ImageOption {
	return func(i *imageOptions) error {
		i.baseImagePath = path
		return nil
	}
}

func NewImage(path string, ops ...ImageOption) (*Image, error) {
	imageOpts := &imageOptions{}
	for _, op := range ops {
		if err := op(imageOpts); err != nil {
			return nil, err
		}
	}

	platform := defaultPlatform()
	if (imageOpts.platform != imgutil.Platform{}) {
		platform = imageOpts.platform
	}

	image, err := emptyImage(platform)
	if err != nil {
		return nil, err
	}

	ri := &Image{
		Image: image,
		path:  path,
	}

	if imageOpts.prevImagePath != "" {
		if err := processPreviousImageOption(ri, imageOpts.prevImagePath, platform); err != nil {
			return nil, err
		}
	}

	if imageOpts.baseImagePath != "" {
		if err := processBaseImagePathOption(ri, imageOpts.baseImagePath, platform); err != nil {
			return nil, err
		}
	} else if imageOpts.baseImage != nil {
		if err := ri.mutateImage(imageOpts.baseImage); err != nil {
			return nil, err
		}
	}

	if imageOpts.createdAt.IsZero() {
		ri.createdAt = imgutil.NormalizedDateTime
	} else {
		ri.createdAt = imageOpts.createdAt
	}

	return ri, nil
}

func processPreviousImageOption(ri *Image, prevImagePath string, platform imgutil.Platform) error {
	prevImage, err := newV1Image(prevImagePath, platform)
	if err != nil {
		return err
	}

	prevLayers, err := prevImage.Layers()
	if err != nil {
		return errors.Wrapf(err, "getting layers for previous image with path %q", prevImagePath)
	}

	ri.prevLayers = prevLayers

	return nil
}

func processBaseImagePathOption(ri *Image, baseImagePath string, platform imgutil.Platform) error {
	baseImage, err := newV1Image(baseImagePath, platform)
	if err != nil {
		return err
	}

	return ri.mutateImage(baseImage)
}

func emptyImage(platform imgutil.Platform) (v1.Image, error) {
	cfg := &v1.ConfigFile{
		Architecture: platform.Architecture,
		OS:           platform.OS,
		OSVersion:    platform.OSVersion,
		RootFS: v1.RootFS{
			Type:    "layers",
			DiffIDs: []v1.Hash{},
		},
	}
	image := mutate.MediaType(empty.Image, types.OCIManifestSchema1)
	image = mutate.ConfigMediaType(image, types.OCIConfigJSON)
	return mutate.ConfigFile(image, cfg)
}

func defaultPlatform() imgutil.Platform {
	return imgutil.Platform{
		OS:           "linux",
		Architecture: "amd64",
	}
}

func (i *Image) Label(key string) (string, error) {
	cfg, err := i.Image.ConfigFile()
	if err != nil {
		return "", fmt.Errorf("getting config for image at path %q: %w", i.path, err)
	}
	if cfg == nil {
		return "", fmt.Errorf("missing config for image at path %q", i.path)
	}
	labels := cfg.Config.Labels
	return labels[key], nil
}

func (i *Image) Labels() (map[string]string, error) {
	cfg, err := i.Image.ConfigFile()
	if err != nil {
		return nil, errors.Wrapf(err, "getting config file for image at path %q", i.path)
	}
	if cfg == nil {
		return nil, fmt.Errorf("missing config for image at path %q", i.path)
	}
	return cfg.Config.Labels, nil
}

func (i *Image) Env(key string) (string, error) {
	cfg, err := i.Image.ConfigFile()
	if err != nil {
		return "", errors.Wrapf(err, "getting config file for image at path %q", i.path)
	}
	if cfg == nil {
		return "", fmt.Errorf("missing config for image at path %q", i.path)
	}
	for _, envVar := range cfg.Config.Env {
		parts := strings.Split(envVar, "=")
		if parts[0] == key {
			return parts[1], nil
		}
	}
	return "", nil
}

func (i *Image) WorkingDir() (string, error) {
	cfg, err := i.Image.ConfigFile()
	if err != nil {
		return "", errors.Wrapf(err, "getting config file for image at path %q", i.path)
	}
	if cfg == nil {
		return "", fmt.Errorf("missing config for image at path %q", i.path)
	}
	return cfg.Config.WorkingDir, nil
}

func (i *Image) Entrypoint() ([]string, error) {
	cfg, err := i.Image.ConfigFile()
	if err != nil {
		return nil, errors.Wrapf(err, "getting config file for image at path %q", i.path)
	}
	if cfg == nil {
		return nil, fmt.Errorf("missing config for image at path %q", i.path)
	}
	return cfg.Config.Entrypoint, nil
}

func (i *Image) OS() (string, error) {
	cfg, err := i.Image.ConfigFile()
	if err != nil {
		return "", errors.Wrapf(err, "getting config file for image at path %q", i.path)
	}
	if cfg == nil {
		return "", fmt.Errorf("missing config for image at path %q", i.path)
	}
	if cfg.OS == "" {
		return "", fmt.Errorf("missing OS for image at path %q", i.path)
	}
	return cfg.OS, nil
}

func (i *Image) OSVersion() (string, error) {
	cfg, err := i.Image.ConfigFile()
	if err != nil {
		return "", errors.Wrapf(err, "getting config file for image at path %q", i.path)
	}
	if cfg == nil {
		return "", fmt.Errorf("missing config for image at path %q", i.path)
	}
	return cfg.OSVersion, nil
}

func (i *Image) Architecture() (string, error) {
	cfg, err := i.Image.ConfigFile()
	if err != nil {
		return "", errors.Wrapf(err, "getting config file for image at path %q", i.path)
	}
	if cfg == nil {
		return "", fmt.Errorf("missing config for image at path %q", i.path)
	}
	if cfg.Architecture == "" {
		return "", fmt.Errorf("missing Architecture for image at path %q", i.path)
	}
	return cfg.Architecture, nil
}

func (i *Image) Name() string {
	return i.path
}

func (i *Image) Rename(name string) {
	i.path = name
}

// Found tells whether the image exists in the repository by `Name()`.
func (i *Image) Found() bool {
	return ImageExists(i.path)
}

// Identifier
// Each image's ID is given by the SHA256 hash of its configuration JSON. It is represented as a hexadecimal encoding of 256 bits,
// e.g., sha256:a9561eb1b190625c9adb5a9513e72c4dedafc1cb2d4c5236c9a6957ec7dfd5a9.
func (i *Image) Identifier() (imgutil.Identifier, error) {
	hash, err := i.Image.Digest()
	if err != nil {
		return nil, errors.Wrapf(err, "getting identifier for image at path %q", i.path)
	}
	return newLayoutIdentifier(i.path, hash)
}

func (i *Image) CreatedAt() (time.Time, error) {
	configFile, err := i.Image.ConfigFile()
	if err != nil {
		return time.Time{}, errors.Wrapf(err, "getting createdAt time for image at path %q", i.path)
	}
	return configFile.Created.UTC(), nil
}

func (i *Image) Rebase(s string, image imgutil.Image) error {
	return errors.New("not yet implemented")
}

func (i *Image) SetLabel(key string, val string) error {
	configFile, err := i.Image.ConfigFile()
	if err != nil {
		return err
	}
	config := *configFile.Config.DeepCopy()
	if config.Labels == nil {
		config.Labels = map[string]string{}
	}
	config.Labels[key] = val
	err = i.mutateConfig(i.Image, config)
	if err != nil {
		return errors.Wrapf(err, "set label key=%s value=%s", key, val)
	}
	return nil
}

func (i *Image) RemoveLabel(key string) error {
	cfg, err := i.Image.ConfigFile()
	if err != nil {
		return errors.Wrapf(err, "getting config file for image at path %q", i.path)
	}
	if cfg == nil {
		return fmt.Errorf("missing config for image at path %q", i.path)
	}
	config := *cfg.Config.DeepCopy()
	delete(config.Labels, key)
	err = i.mutateConfig(i.Image, config)
	return err
}

func (i *Image) SetEnv(key string, val string) error {
	configFile, err := i.Image.ConfigFile()
	if err != nil {
		return err
	}
	config := *configFile.Config.DeepCopy()
	ignoreCase := configFile.OS == "windows"
	for idx, e := range config.Env {
		parts := strings.Split(e, "=")
		foundKey := parts[0]
		searchKey := key
		if ignoreCase {
			foundKey = strings.ToUpper(foundKey)
			searchKey = strings.ToUpper(searchKey)
		}
		if foundKey == searchKey {
			config.Env[idx] = fmt.Sprintf("%s=%s", key, val)
			err = i.mutateConfig(i.Image, config)
			return err
		}
	}
	config.Env = append(config.Env, fmt.Sprintf("%s=%s", key, val))
	err = i.mutateConfig(i.Image, config)
	return err
}

func (i *Image) SetWorkingDir(dir string) error {
	configFile, err := i.Image.ConfigFile()
	if err != nil {
		return err
	}
	config := *configFile.Config.DeepCopy()
	config.WorkingDir = dir
	err = i.mutateConfig(i.Image, config)
	return err
}

func (i *Image) SetEntrypoint(ep ...string) error {
	configFile, err := i.Image.ConfigFile()
	if err != nil {
		return err
	}
	config := *configFile.Config.DeepCopy()
	config.Entrypoint = ep
	err = i.mutateConfig(i.Image, config)
	return err
}

func (i *Image) SetCmd(cmd ...string) error {
	configFile, err := i.Image.ConfigFile()
	if err != nil {
		return err
	}
	config := *configFile.Config.DeepCopy()
	config.Cmd = cmd
	err = i.mutateConfig(i.Image, config)
	return err
}

func (i *Image) SetOS(osVal string) error {
	configFile, err := i.Image.ConfigFile()
	if err != nil {
		return err
	}
	configFile.OS = osVal
	err = i.mutateConfigFile(i.Image, configFile)
	return err
}

func (i *Image) SetOSVersion(osVersion string) error {
	configFile, err := i.Image.ConfigFile()
	if err != nil {
		return err
	}
	configFile.OSVersion = osVersion
	err = i.mutateConfigFile(i.Image, configFile)
	return err
}

func (i *Image) SetArchitecture(architecture string) error {
	configFile, err := i.Image.ConfigFile()
	if err != nil {
		return err
	}
	configFile.Architecture = architecture
	err = i.mutateConfigFile(i.Image, configFile)
	return err
}

func (i *Image) TopLayer() (string, error) {
	all, err := i.Image.Layers()
	if err != nil {
		return "", err
	}
	if len(all) == 0 {
		return "", fmt.Errorf("image at path %q has no layers", i.Name())
	}
	topLayer := all[len(all)-1]
	hex, err := topLayer.DiffID()
	if err != nil {
		return "", err
	}
	return hex.String(), nil
}

// GetLayer retrieves layer by diff id. Returns a reader of the uncompressed contents of the layer.
// When the layers (notExistsLayer) came from a sparse image returns an empty reader
func (i *Image) GetLayer(sha string) (io.ReadCloser, error) {
	layers, err := i.Image.Layers()
	if err != nil {
		return nil, err
	}

	layer, err := findLayerWithSha(layers, sha)
	if err != nil {
		return nil, err
	}

	return layer.Uncompressed()
}

// AddLayer adds an uncompressed tarred layer to the image
func (i *Image) AddLayer(path string) error {
	layer, err := tarball.LayerFromFile(path)
	if err != nil {
		return err
	}
	return i.addOCILayer(layer)
}

func (i *Image) AddLayerWithDiffID(path, diffID string) error {
	// this is equivalent to AddLayer in the layout case
	// it exists to provide optimize performance for local images
	return i.AddLayer(path)
}

func (i *Image) ReuseLayer(sha string) error {
	layer, err := findLayerWithSha(i.prevLayers, sha)
	if err != nil {
		return err
	}
	return i.addOCILayer(layer)
}

func findLayerWithSha(layers []v1.Layer, diffID string) (v1.Layer, error) {
	for _, layer := range layers {
		dID, err := layer.DiffID()
		if err != nil {
			return nil, errors.Wrap(err, "get diff ID for previous image layer")
		}
		if diffID == dID.String() {
			return layer, nil
		}
	}
	return nil, fmt.Errorf("previous image did not have layer with diff id %q", diffID)
}

func (i *Image) Save(additionalNames ...string) error {
	return i.SaveAs(i.Name(), additionalNames...)
}

// SaveAs ignores the image `Name()` method and saves the image according to name & additional names provided to this method
func (i *Image) SaveAs(name string, additionalNames ...string) error {
	err := i.mutateCreatedAt(i.Image, v1.Time{Time: i.createdAt})
	if err != nil {
		return errors.Wrap(err, "set creation time")
	}

	cfg, err := i.Image.ConfigFile()
	if err != nil {
		return errors.Wrap(err, "get image config")
	}
	cfg = cfg.DeepCopy()

	layers, err := i.Image.Layers()
	if err != nil {
		return errors.Wrap(err, "get image layers")
	}
	cfg.History = make([]v1.History, len(layers))
	for j := range cfg.History {
		cfg.History[j] = v1.History{
			Created: v1.Time{Time: i.createdAt},
		}
	}

	cfg.DockerVersion = ""
	cfg.Container = ""
	err = i.mutateConfigFile(i.Image, cfg)
	if err != nil {
		return errors.Wrap(err, "zeroing history")
	}

	var diagnostics []imgutil.SaveDiagnostic
	annotations := ImageRefAnnotation(i.refName)
	pathsToSave := append([]string{name}, additionalNames...)
	for _, path := range pathsToSave {
		// initialize image path
		path, err := Write(path, empty.Index)
		if err != nil {
			return err
		}

		err = path.AppendImage(i.Image, WithAnnotations(annotations))
		if err != nil {
			diagnostics = append(diagnostics, imgutil.SaveDiagnostic{ImageName: i.Name(), Cause: err})
		}
	}

	if len(diagnostics) > 0 {
		return imgutil.SaveError{Errors: diagnostics}
	}

	return nil
}

func (i *Image) SaveFile() (string, error) {
	// TODO issue https://github.com/buildpacks/imgutil/issues/170
	return "", errors.New("not yet implemented")
}

func (i *Image) Delete() error {
	return os.RemoveAll(i.path)
}

func (i *Image) ManifestSize() (int64, error) {
	return i.Image.Size()
}

// Layers overrides v1.Image Layers(), because we allow sparse image in OCI layout, sometimes some blobs
// are missing. This method checks:
// If there is data, return the layer
// If there is no data, return a notExistsLayer
func (i *Image) Layers() ([]v1.Layer, error) {
	layers, err := i.Image.Layers()
	if err != nil {
		return nil, err
	}

	var retLayers []v1.Layer
	for pos, layer := range layers {
		if hasData(layer) {
			retLayers = append(retLayers, layer)
		} else {
			cfg, err := i.Image.ConfigFile()
			if err != nil {
				return nil, err
			}
			diffID := cfg.RootFS.DiffIDs[pos]
			retLayers = append(retLayers, &notExistsLayer{Layer: layer, diffID: diffID})
		}
	}
	return retLayers, nil
}

func (i *Image) AnnotateRefName(refName string) error {
	i.refName = refName
	return nil
}

func (i *Image) GetAnnotateRefName() (string, error) {
	return i.refName, nil
}

func ImageExists(path string) bool {
	if !pathExists(path) {
		return false
	}
	index := filepath.Join(path, "index.json")
	if _, err := os.Stat(index); os.IsNotExist(err) {
		return false
	}
	return true
}

func hasData(layer v1.Layer) bool {
	_, err := layer.Compressed()
	return err == nil
}

func pathExists(path string) bool {
	if path != "" {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			return true
		}
	}
	return false
}

// newV1Image creates a layout image from the given path.
//   - If a ImageIndex for multiples platforms exists, then it will try to select the image
//     according to the platform provided
//   - If the image does not exist, then an empty image is returned
func newV1Image(path string, platform imgutil.Platform) (v1.Image, error) {
	var (
		image  v1.Image
		layout Path
		err    error
	)

	if ImageExists(path) {
		layout, err = FromPath(path)
		if err != nil {
			return nil, errors.Wrap(err, "loading layout from path new")
		}

		index, err := layout.ImageIndex()
		if err != nil {
			return nil, errors.Wrap(err, "reading index")
		}

		image, err = imageFromIndex(index, platform)
		if err != nil {
			return nil, errors.Wrap(err, "getting image from index")
		}
	} else {
		image, err = emptyImage(platform)
		if err != nil {
			return nil, errors.Wrap(err, "initializing empty image")
		}
	}
	return &Image{
		Image: image,
		path:  path,
	}, nil
}

// imageFromIndex creates a v1.Image from the given Image Index, selecting the image manifest
// that matches the given OS and architecture.
func imageFromIndex(index v1.ImageIndex, platform imgutil.Platform) (v1.Image, error) {
	indexManifest, err := index.IndexManifest()
	if err != nil {
		return nil, err
	}

	if len(indexManifest.Manifests) == 0 {
		return nil, errors.New("no underlyingImage indexManifest found")
	}

	manifest := indexManifest.Manifests[0]
	if len(indexManifest.Manifests) > 1 {
		// Find based on platform (os/arch)
		for _, m := range indexManifest.Manifests {
			if m.Platform.OS == platform.OS && m.Platform.Architecture == platform.OS {
				manifest = m
				break
			}
		}
		return nil, fmt.Errorf("manifest matching platform %v not found", platform)
	}

	image, err := index.Image(manifest.Digest)
	if err != nil {
		return nil, err
	}

	return image, nil
}

// mutateConfig mutates the provided v1.Image to have the provided v1.Config and wraps the result
// into a layout.Image (requires for override methods like Layers()
func (i *Image) mutateConfig(base v1.Image, config v1.Config) error {
	image, err := mutate.Config(base, config)
	if err != nil {
		return err
	}
	return i.mutateImage(image)
}

// mutateConfigFile mutates the provided v1.Image to have the provided v1.ConfigFile and wraps the result
// into a layout.Image (requires for override methods like Layers()
func (i *Image) mutateConfigFile(base v1.Image, configFile *v1.ConfigFile) error {
	image, err := mutate.ConfigFile(base, configFile)
	if err != nil {
		return err
	}
	return i.mutateImage(image)
}

// mutateCreatedAt mutates the provided v1.Image to have the provided v1.Time and wraps the result
// into a layout.Image (requires for override methods like Layers()
func (i *Image) mutateCreatedAt(base v1.Image, created v1.Time) error {
	image, err := mutate.CreatedAt(i.Image, v1.Time{Time: i.createdAt})
	if err != nil {
		return err
	}
	return i.mutateImage(image)
}

// mutateImage wraps the provided v1.Image into a layout.Image
func (i *Image) mutateImage(base v1.Image) error {
	manifest, err := base.Manifest()
	if err != nil {
		return err
	}
	if validMediaTypes(manifest) {
		i.Image = &Image{
			Image: base,
		}
	} else {
		// images has docker media types, we need to override them
		newBaseImage, err := overrideMediaTypes(base)
		if err != nil {
			return err
		}
		i.Image = &Image{
			Image: newBaseImage,
		}
	}
	return nil
}

// addOCILayer appends the provided layer with media type application/vnd.oci.image.layer.v1.tar+gzip
func (i *Image) addOCILayer(layer v1.Layer) error {
	additions := layersAddendum([]v1.Layer{layer})
	image, err := mutate.Append(i.Image, additions...)
	if err != nil {
		return errors.Wrap(err, "add layer")
	}
	return i.mutateImage(image)
}

// validMediaTypes returns true if media types present in the manifest are the ones defined by the OCI spec
// Docker Media Types will return false.
func validMediaTypes(manifest *v1.Manifest) bool {
	return manifest.MediaType == types.OCIManifestSchema1 &&
		manifest.Config.MediaType == types.OCIConfigJSON
}

// overrideMediaTypes will create a new v1.Image from the provided base image, but replacing
// manifest media type, config media type and layers media type by the ones defined by the OCI spec
func overrideMediaTypes(base v1.Image) (v1.Image, error) {
	config, err := base.ConfigFile()
	if err != nil {
		return nil, err
	}
	config.RootFS.DiffIDs = make([]v1.Hash, 0)

	image := mutate.MediaType(empty.Image, types.OCIManifestSchema1)
	image, err = mutate.ConfigFile(image, config)
	if err != nil {
		return nil, err
	}
	image = mutate.ConfigMediaType(image, types.OCIConfigJSON)

	layers, err := base.Layers()
	if err != nil {
		return nil, err
	}

	additions := layersAddendum(layers)
	image, err = mutate.Append(image, additions...)
	if err != nil {
		return nil, err
	}

	return image, nil
}

// layersAddendum creates an Addendum array with the given layers
// and 'application/vnd.oci.image.layer.v1.tar+gzip' media type
func layersAddendum(layers []v1.Layer) []mutate.Addendum {
	additions := make([]mutate.Addendum, 0)
	for _, layer := range layers {
		additions = append(additions, mutate.Addendum{
			MediaType: types.OCILayer,
			Layer:     layer,
		})
	}
	return additions
}
