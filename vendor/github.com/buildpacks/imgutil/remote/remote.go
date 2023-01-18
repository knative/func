package remote

import (
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/buildpacks/imgutil/layer"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/remote/transport"
	"github.com/google/go-containerregistry/pkg/v1/static"
	"github.com/google/go-containerregistry/pkg/v1/tarball"
	"github.com/google/go-containerregistry/pkg/v1/types"
	"github.com/pkg/errors"

	"github.com/buildpacks/imgutil"
)

const maxRetries = 2

type Image struct {
	keychain            authn.Keychain
	repoName            string
	image               v1.Image
	prevLayers          []v1.Layer
	createdAt           time.Time
	addEmptyLayerOnSave bool
	registrySettings    []registrySetting
}

type options struct {
	platform            imgutil.Platform
	baseImageRepoName   string
	prevImageRepoName   string
	createdAt           time.Time
	addEmptyLayerOnSave bool
	registrySettings    []registrySetting
}

type registrySetting struct {
	prefix             string
	insecure           bool
	insecureSkipVerify bool
}

type ImageOption func(*options) error

// WithPreviousImage loads an existing image as a source for reusable layers.
// Use with ReuseLayer().
// Ignored if image is not found.
func WithPreviousImage(imageName string) ImageOption {
	return func(opts *options) error {
		opts.prevImageRepoName = imageName
		return nil
	}
}

// FromBaseImage loads an existing image as the config and layers for the new image.
// Ignored if image is not found.
func FromBaseImage(imageName string) ImageOption {
	return func(opts *options) error {
		opts.baseImageRepoName = imageName
		return nil
	}
}

// WithDefaultPlatform provides Architecture/OS/OSVersion defaults for the new image.
// Defaults for a new image are ignored when FromBaseImage returns an image.
// FromBaseImage and WithPreviousImage will use the platform to choose an image from a manifest list.
func WithDefaultPlatform(platform imgutil.Platform) ImageOption {
	return func(opts *options) error {
		opts.platform = platform
		return nil
	}
}

// WithCreatedAt lets a caller set the created at timestamp for the image.
// Defaults for a new image is imgutil.NormalizedDateTime
func WithCreatedAt(createdAt time.Time) ImageOption {
	return func(opts *options) error {
		opts.createdAt = createdAt
		return nil
	}
}

// AddEmptyLayerOnSave adds an empty layer before saving if the image has no layer at all.
// This option is useful when exporting to registries that do not allow saving an image without layers,
// for example: gcr.io
func AddEmptyLayerOnSave() ImageOption {
	return func(opts *options) error {
		opts.addEmptyLayerOnSave = true
		return nil
	}
}

// WithRegistrySetting registers options to use when accessing images in a registry in order to construct
// the image. The referenced images could include the base image, a previous image, or the image itself.
func WithRegistrySetting(repository string, insecure, insecureSkipVerify bool) ImageOption {
	return func(opts *options) error {
		opts.registrySettings = append(opts.registrySettings, registrySetting{
			prefix:             repository,
			insecure:           insecure,
			insecureSkipVerify: insecureSkipVerify,
		})
		return nil
	}
}

// NewImage returns a new Image that can be modified and saved to a Docker daemon.
func NewImage(repoName string, keychain authn.Keychain, ops ...ImageOption) (*Image, error) {
	imageOpts := &options{}
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
		keychain:            keychain,
		repoName:            repoName,
		image:               image,
		addEmptyLayerOnSave: imageOpts.addEmptyLayerOnSave,
		registrySettings:    imageOpts.registrySettings,
	}

	if imageOpts.prevImageRepoName != "" {
		if err := processPreviousImageOption(ri, imageOpts.prevImageRepoName, platform); err != nil {
			return nil, err
		}
	}

	if imageOpts.baseImageRepoName != "" {
		if err := processBaseImageOption(ri, imageOpts.baseImageRepoName, platform); err != nil {
			return nil, err
		}
	}

	imgOS, err := ri.OS()
	if err != nil {
		return nil, err
	}
	if imgOS == "windows" {
		if err := prepareNewWindowsImage(ri); err != nil {
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

func getRegistry(repoName string, registrySettings []registrySetting) registrySetting {
	for _, r := range registrySettings {
		if strings.HasPrefix(repoName, r.prefix) {
			return r
		}
	}

	return registrySetting{}
}

func processPreviousImageOption(ri *Image, prevImageRepoName string, platform imgutil.Platform) error {
	reg := getRegistry(prevImageRepoName, ri.registrySettings)

	prevImage, err := newV1Image(ri.keychain, prevImageRepoName, platform, reg)
	if err != nil {
		return err
	}

	prevLayers, err := prevImage.Layers()
	if err != nil {
		return errors.Wrapf(err, "getting layers for previous image with repo name %q", prevImageRepoName)
	}

	ri.prevLayers = prevLayers

	return nil
}

func processBaseImageOption(ri *Image, baseImageRepoName string, platform imgutil.Platform) error {
	reg := getRegistry(baseImageRepoName, ri.registrySettings)

	baseImage, err := newV1Image(ri.keychain, baseImageRepoName, platform, reg)
	if err != nil {
		return err
	}

	ri.image = baseImage

	return nil
}

func prepareNewWindowsImage(ri *Image) error {
	// only append base layer to empty image
	cfgFile, err := ri.image.ConfigFile()
	if err != nil {
		return err
	}
	if len(cfgFile.RootFS.DiffIDs) > 0 {
		return nil
	}

	layerBytes, err := layer.WindowsBaseLayer()
	if err != nil {
		return err
	}

	windowsBaseLayer, err := tarball.LayerFromReader(layerBytes) // TODO: LayerFromReader is deprecated; LayerFromOpener or stream.NewLayer are suggested alternatives however the tests do not pass when they are used
	if err != nil {
		return err
	}

	image, err := mutate.AppendLayers(ri.image, windowsBaseLayer)
	if err != nil {
		return err
	}

	ri.image = image

	return nil
}

func newV1Image(keychain authn.Keychain, repoName string, platform imgutil.Platform, reg registrySetting) (v1.Image, error) {
	ref, auth, err := referenceForRepoName(keychain, repoName, reg.insecure)
	if err != nil {
		return nil, err
	}

	v1Platform := v1.Platform{
		Architecture: platform.Architecture,
		OS:           platform.OS,
		OSVersion:    platform.OSVersion,
	}

	opts := []remote.Option{remote.WithAuth(auth), remote.WithPlatform(v1Platform)}
	// #nosec G402
	if reg.insecureSkipVerify {
		opts = append(opts, remote.WithTransport(&http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
			},
		}))
	} else {
		opts = append(opts, remote.WithTransport(http.DefaultTransport))
	}

	var image v1.Image
	for i := 0; i <= maxRetries; i++ {
		time.Sleep(100 * time.Duration(i) * time.Millisecond) // wait if retrying
		image, err = remote.Image(ref, opts...)
		if err != nil {
			if err == io.EOF && i != maxRetries {
				continue // retry if EOF
			}
			if transportErr, ok := err.(*transport.Error); ok && len(transportErr.Errors) > 0 {
				switch transportErr.StatusCode {
				case http.StatusNotFound, http.StatusUnauthorized:
					return emptyImage(platform)
				}
			}
			if strings.Contains(err.Error(), "no child with platform") {
				return emptyImage(platform)
			}
			return nil, errors.Wrapf(err, "connect to repo store %q", repoName)
		}
		break
	}

	return image, nil
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

	return mutate.ConfigFile(empty.Image, cfg)
}

func defaultPlatform() imgutil.Platform {
	return imgutil.Platform{
		OS:           "linux",
		Architecture: "amd64",
	}
}

func referenceForRepoName(keychain authn.Keychain, ref string, insecure bool) (name.Reference, authn.Authenticator, error) {
	var auth authn.Authenticator
	opts := []name.Option{name.WeakValidation}
	if insecure {
		opts = append(opts, name.Insecure)
	}
	r, err := name.ParseReference(ref, opts...)
	if err != nil {
		return nil, nil, err
	}

	auth, err = keychain.Resolve(r.Context().Registry)
	if err != nil {
		return nil, nil, err
	}
	return r, auth, nil
}

func (i *Image) Label(key string) (string, error) {
	cfg, err := i.image.ConfigFile()
	if err != nil {
		return "", errors.Wrapf(err, "getting config file for image %q", i.repoName)
	}
	if cfg == nil {
		return "", fmt.Errorf("missing config for image %q", i.repoName)
	}
	labels := cfg.Config.Labels
	return labels[key], nil
}

func (i *Image) Labels() (map[string]string, error) {
	cfg, err := i.image.ConfigFile()
	if err != nil {
		return nil, errors.Wrapf(err, "getting config file for image %q", i.repoName)
	}
	if cfg == nil {
		return nil, fmt.Errorf("missing config for image %q", i.repoName)
	}
	return cfg.Config.Labels, nil
}

func (i *Image) Env(key string) (string, error) {
	cfg, err := i.image.ConfigFile()
	if err != nil {
		return "", errors.Wrapf(err, "getting config file for image %q", i.repoName)
	}
	if cfg == nil {
		return "", fmt.Errorf("missing config for image %q", i.repoName)
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
	cfg, err := i.image.ConfigFile()
	if err != nil {
		return "", errors.Wrapf(err, "getting config file for image %q", i.repoName)
	}
	if cfg == nil {
		return "", fmt.Errorf("missing config for image %q", i.repoName)
	}
	return cfg.Config.WorkingDir, nil
}

func (i *Image) Entrypoint() ([]string, error) {
	cfg, err := i.image.ConfigFile()
	if err != nil {
		return nil, errors.Wrapf(err, "getting config file for image %q", i.repoName)
	}
	if cfg == nil {
		return nil, fmt.Errorf("missing config for image %q", i.repoName)
	}
	return cfg.Config.Entrypoint, nil
}

func (i *Image) OS() (string, error) {
	cfg, err := i.image.ConfigFile()
	if err != nil {
		return "", errors.Wrapf(err, "getting config file for image %q", i.repoName)
	}
	if cfg == nil {
		return "", fmt.Errorf("missing config for image %q", i.repoName)
	}
	if cfg.OS == "" {
		return "", fmt.Errorf("missing OS for image %q", i.repoName)
	}
	return cfg.OS, nil
}

func (i *Image) OSVersion() (string, error) {
	cfg, err := i.image.ConfigFile()
	if err != nil {
		return "", errors.Wrapf(err, "getting config file for image %q", i.repoName)
	}
	if cfg == nil {
		return "", fmt.Errorf("missing config for image %q", i.repoName)
	}
	return cfg.OSVersion, nil
}

func (i *Image) Architecture() (string, error) {
	cfg, err := i.image.ConfigFile()
	if err != nil {
		return "", errors.Wrapf(err, "getting config file for image %q", i.repoName)
	}
	if cfg == nil {
		return "", fmt.Errorf("missing config for image %q", i.repoName)
	}
	if cfg.Architecture == "" {
		return "", fmt.Errorf("missing Architecture for image %q", i.repoName)
	}
	return cfg.Architecture, nil
}

func (i *Image) Rename(name string) {
	i.repoName = name
}

func (i *Image) Name() string {
	return i.repoName
}

func (i *Image) Found() bool {
	_, err := i.found()
	return err == nil
}

func (i *Image) CheckReadWriteAccess() bool {
	reg := getRegistry(i.repoName, i.registrySettings)
	ref, _, err := referenceForRepoName(i.keychain, i.repoName, reg.insecure)
	if err != nil {
		return false
	}
	return i.CheckReadAccess() && remote.CheckPushPermission(ref, i.keychain, http.DefaultTransport) == nil
}

func (i *Image) CheckReadAccess() bool {
	_, err := i.found()
	if err != nil {
		if transportErr, ok := err.(*transport.Error); ok {
			return transportErr.StatusCode != http.StatusUnauthorized &&
				transportErr.StatusCode != http.StatusForbidden
		}
		return false
	}
	return true
}

func (i *Image) found() (*v1.Descriptor, error) {
	reg := getRegistry(i.repoName, i.registrySettings)
	ref, auth, err := referenceForRepoName(i.keychain, i.repoName, reg.insecure)
	if err != nil {
		return nil, err
	}
	return remote.Head(ref, remote.WithAuth(auth), remote.WithTransport(http.DefaultTransport))
}

func (i *Image) Identifier() (imgutil.Identifier, error) {
	ref, err := name.ParseReference(i.repoName, name.WeakValidation)
	if err != nil {
		return nil, errors.Wrapf(err, "parsing reference for image %q", i.repoName)
	}

	hash, err := i.image.Digest()
	if err != nil {
		return nil, errors.Wrapf(err, "getting digest for image %q", i.repoName)
	}

	digestRef, err := name.NewDigest(fmt.Sprintf("%s@%s", ref.Context().Name(), hash.String()), name.WeakValidation)
	if err != nil {
		return nil, errors.Wrap(err, "creating digest reference")
	}

	return DigestIdentifier{
		Digest: digestRef,
	}, nil
}

func (i *Image) CreatedAt() (time.Time, error) {
	configFile, err := i.image.ConfigFile()
	if err != nil {
		return time.Time{}, errors.Wrapf(err, "getting createdAt time for image %q", i.repoName)
	}
	return configFile.Created.UTC(), nil
}

func (i *Image) Rebase(baseTopLayer string, newBase imgutil.Image) error {
	newBaseRemote, ok := newBase.(*Image)
	if !ok {
		return errors.New("expected new base to be a remote image")
	}

	newImage, err := mutate.Rebase(i.image, &subImage{img: i.image, topDiffID: baseTopLayer}, newBaseRemote.image)
	if err != nil {
		return errors.Wrap(err, "rebase")
	}

	newImageConfig, err := newImage.ConfigFile()
	if err != nil {
		return err
	}

	newBaseRemoteConfig, err := newBaseRemote.image.ConfigFile()
	if err != nil {
		return err
	}

	newImageConfig.Architecture = newBaseRemoteConfig.Architecture
	newImageConfig.OS = newBaseRemoteConfig.OS
	newImageConfig.OSVersion = newBaseRemoteConfig.OSVersion

	newImage, err = mutate.ConfigFile(newImage, newImageConfig)
	if err != nil {
		return err
	}

	i.image = newImage
	return nil
}

func (i *Image) SetLabel(key, val string) error {
	configFile, err := i.image.ConfigFile()
	if err != nil {
		return err
	}
	config := *configFile.Config.DeepCopy()
	if config.Labels == nil {
		config.Labels = map[string]string{}
	}
	config.Labels[key] = val
	i.image, err = mutate.Config(i.image, config)
	return err
}

func (i *Image) RemoveLabel(key string) error {
	cfg, err := i.image.ConfigFile()
	if err != nil {
		return errors.Wrapf(err, "getting config file for image %q", i.repoName)
	}
	if cfg == nil {
		return fmt.Errorf("missing config for image %q", i.repoName)
	}
	config := *cfg.Config.DeepCopy()
	delete(config.Labels, key)
	i.image, err = mutate.Config(i.image, config)
	return err
}

func (i *Image) SetEnv(key, val string) error {
	configFile, err := i.image.ConfigFile()
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
			i.image, err = mutate.Config(i.image, config)
			return err
		}
	}
	config.Env = append(config.Env, fmt.Sprintf("%s=%s", key, val))
	i.image, err = mutate.Config(i.image, config)
	return err
}

func (i *Image) SetWorkingDir(dir string) error {
	configFile, err := i.image.ConfigFile()
	if err != nil {
		return err
	}
	config := *configFile.Config.DeepCopy()
	config.WorkingDir = dir
	i.image, err = mutate.Config(i.image, config)
	return err
}

func (i *Image) SetEntrypoint(ep ...string) error {
	configFile, err := i.image.ConfigFile()
	if err != nil {
		return err
	}
	config := *configFile.Config.DeepCopy()
	config.Entrypoint = ep
	i.image, err = mutate.Config(i.image, config)
	return err
}

func (i *Image) SetCmd(cmd ...string) error {
	configFile, err := i.image.ConfigFile()
	if err != nil {
		return err
	}
	config := *configFile.Config.DeepCopy()
	config.Cmd = cmd
	i.image, err = mutate.Config(i.image, config)
	return err
}

func (i *Image) SetOS(osVal string) error {
	configFile, err := i.image.ConfigFile()
	if err != nil {
		return err
	}
	configFile.OS = osVal
	i.image, err = mutate.ConfigFile(i.image, configFile)
	return err
}

func (i *Image) SetOSVersion(osVersion string) error {
	configFile, err := i.image.ConfigFile()
	if err != nil {
		return err
	}
	configFile.OSVersion = osVersion
	i.image, err = mutate.ConfigFile(i.image, configFile)
	return err
}

func (i *Image) SetArchitecture(architecture string) error {
	configFile, err := i.image.ConfigFile()
	if err != nil {
		return err
	}
	configFile.Architecture = architecture
	i.image, err = mutate.ConfigFile(i.image, configFile)
	return err
}

func (i *Image) TopLayer() (string, error) {
	all, err := i.image.Layers()
	if err != nil {
		return "", err
	}
	if len(all) == 0 {
		return "", fmt.Errorf("image %q has no layers", i.Name())
	}
	topLayer := all[len(all)-1]
	hex, err := topLayer.DiffID()
	if err != nil {
		return "", err
	}
	return hex.String(), nil
}

func (i *Image) GetLayer(sha string) (io.ReadCloser, error) {
	layers, err := i.image.Layers()
	if err != nil {
		return nil, err
	}

	layer, err := findLayerWithSha(layers, sha)
	if err != nil {
		return nil, err
	}

	return layer.Uncompressed()
}

func (i *Image) AddLayer(path string) error {
	layer, err := tarball.LayerFromFile(path)
	if err != nil {
		return err
	}
	i.image, err = mutate.AppendLayers(i.image, layer)
	if err != nil {
		return errors.Wrap(err, "add layer")
	}
	return nil
}

func (i *Image) AddLayerWithDiffID(path, diffID string) error {
	// this is equivalent to AddLayer in the remote case
	// it exists to provide optimize performance for local images
	return i.AddLayer(path)
}

func (i *Image) ReuseLayer(sha string) error {
	layer, err := findLayerWithSha(i.prevLayers, sha)
	if err != nil {
		return err
	}
	i.image, err = mutate.AppendLayers(i.image, layer)
	return err
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
	var err error

	allNames := append([]string{i.repoName}, additionalNames...)

	i.image, err = mutate.CreatedAt(i.image, v1.Time{Time: i.createdAt})
	if err != nil {
		return errors.Wrap(err, "set creation time")
	}

	cfg, err := i.image.ConfigFile()
	if err != nil {
		return errors.Wrap(err, "get image config")
	}
	cfg = cfg.DeepCopy()

	layers, err := i.image.Layers()
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
	i.image, err = mutate.ConfigFile(i.image, cfg)
	if err != nil {
		return errors.Wrap(err, "zeroing history")
	}

	if len(layers) == 0 && i.addEmptyLayerOnSave {
		empty := static.NewLayer([]byte{}, types.OCILayer)
		i.image, err = mutate.AppendLayers(i.image, empty)
		if err != nil {
			return errors.Wrap(err, "empty layer could not be added")
		}
	}

	var diagnostics []imgutil.SaveDiagnostic
	for _, n := range allNames {
		if err := i.doSave(n); err != nil {
			diagnostics = append(diagnostics, imgutil.SaveDiagnostic{ImageName: n, Cause: err})
		}
	}
	if len(diagnostics) > 0 {
		return imgutil.SaveError{Errors: diagnostics}
	}

	return nil
}

func (i *Image) SaveFile() (string, error) {
	return "", errors.New("not yet implemented")
}

func (i *Image) doSave(imageName string) error {
	reg := getRegistry(i.repoName, i.registrySettings)
	ref, auth, err := referenceForRepoName(i.keychain, imageName, reg.insecure)
	if err != nil {
		return err
	}
	return remote.Write(ref, i.image, remote.WithAuth(auth))
}

func (i *Image) Delete() error {
	id, err := i.Identifier()
	if err != nil {
		return err
	}
	reg := getRegistry(i.repoName, i.registrySettings)
	ref, auth, err := referenceForRepoName(i.keychain, id.String(), reg.insecure)
	if err != nil {
		return err
	}
	return remote.Delete(ref, remote.WithAuth(auth))
}

func (i *Image) ManifestSize() (int64, error) {
	return i.image.Size()
}

type subImage struct {
	img       v1.Image
	topDiffID string
}

func (si *subImage) Layers() ([]v1.Layer, error) {
	all, err := si.img.Layers()
	if err != nil {
		return nil, err
	}
	for i, l := range all {
		d, err := l.DiffID()
		if err != nil {
			return nil, err
		}
		if d.String() == si.topDiffID {
			return all[0 : i+1], nil
		}
	}
	return nil, errors.New("could not find base layer in image")
}
func (si *subImage) ConfigFile() (*v1.ConfigFile, error)     { return si.img.ConfigFile() }
func (si *subImage) BlobSet() (map[v1.Hash]struct{}, error)  { panic("Not Implemented") }
func (si *subImage) MediaType() (types.MediaType, error)     { panic("Not Implemented") }
func (si *subImage) ConfigName() (v1.Hash, error)            { panic("Not Implemented") }
func (si *subImage) RawConfigFile() ([]byte, error)          { panic("Not Implemented") }
func (si *subImage) Digest() (v1.Hash, error)                { panic("Not Implemented") }
func (si *subImage) Manifest() (*v1.Manifest, error)         { panic("Not Implemented") }
func (si *subImage) RawManifest() ([]byte, error)            { panic("Not Implemented") }
func (si *subImage) LayerByDigest(v1.Hash) (v1.Layer, error) { panic("Not Implemented") }
func (si *subImage) LayerByDiffID(v1.Hash) (v1.Layer, error) { panic("Not Implemented") }
func (si *subImage) Size() (int64, error)                    { panic("Not Implemented") }
