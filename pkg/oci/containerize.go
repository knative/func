package oci

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	slashpath "path"
	"path/filepath"
	"strings"
	"time"

	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/tarball"
	"github.com/google/go-containerregistry/pkg/v1/types"
	"github.com/pkg/errors"
)

// languageLayerBuilder builds the layer for the given language whuch may
// be different from one platform to another.  For example, this is the
// layer in the image which contains the Go cross-compiled binary.
type languageLayerBuilder func(*buildConfig, v1.Platform) (v1.Descriptor, v1.Layer, error)

var languageLayerBuilders = map[string]languageLayerBuilder{
	"go":     buildGoLayer,
	"python": layerBuilderNotImplemented,
	"node":   layerBuilderNotImplemented,
	"rust":   layerBuilderNotImplemented,
}

func layerBuilderNotImplemented(cfg *buildConfig, _ v1.Platform) (d v1.Descriptor, l v1.Layer, err error) {
	err = fmt.Errorf("%v functions are not yet supported by the host builder", cfg.f.Runtime)
	return
}

func getLanguageLayerBuilder(cfg *buildConfig) (l languageLayerBuilder, err error) {
	// use the custom implementation, if provided
	if cfg.buildFn != nil {
		return cfg.buildFn, nil
	}
	// otherwise lookup the build function
	l, ok := languageLayerBuilders[cfg.f.Runtime]
	if !ok {
		err = fmt.Errorf("the language runtime '%v' is not a recognized language by the host builder", cfg.f.Runtime)
		return
	}
	return
}

// containerize the scaffolded project by creating and writing an OCI
// conformant directory structure into the functions .func/builds directory.
// The source code to be containerized is indicated by cfg.dir
func containerize(cfg *buildConfig) (err error) {
	// Create the required directories: oci/blobs/sha256
	if err = os.MkdirAll(cfg.blobsDir(), os.ModePerm); err != nil {
		return
	}

	// Create the static, required oci-layout metadata file
	if err = os.WriteFile(path(cfg.ociDir(), "oci-layout"),
		[]byte(`{ "imageLayoutVersion": "1.0.0" }`), os.ModePerm); err != nil {
		return
	}

	// Create the data layer and its descriptor
	dataDesc, dataLayer, err := newDataLayer(cfg) // shared
	if err != nil {
		return
	}

	// Create the root certificates layer and its decriptor
	certsDesc, certsLayer, err := newCertsLayer(cfg) // shared
	if err != nil {
		return
	}

	// Create an image for each platform consisting of the shared data layer,
	// the shared root certs layer, and an os/platform specific layer.
	imageDescs := []v1.Descriptor{}
	for _, p := range cfg.platforms {
		imageDesc, err := newImage(cfg, dataDesc, dataLayer, certsDesc, certsLayer, p, cfg.verbose)
		if err != nil {
			return err
		}
		imageDescs = append(imageDescs, imageDesc)
	}

	// Create the Image Index which enumerates all images contained within
	// the container.
	_, err = newImageIndex(cfg, imageDescs)
	return
}

// newDataLayer creates the shared data layer in the container file hierarchy and
// returns both its descriptor and layer metadata.
func newDataLayer(cfg *buildConfig) (desc v1.Descriptor, layer v1.Layer, err error) {

	// Create the data tarball
	// TODO: try WithCompressedCaching?
	source := cfg.f.Root // The source is the function's entire filesystem
	target := path(cfg.buildDir(), "datalayer.tar.gz")

	if err = newDataTarball(source, target, defaultIgnored, cfg.verbose); err != nil {
		return
	}

	// Layer
	if layer, err = tarball.LayerFromFile(target); err != nil {
		return
	}

	// Descriptor
	if desc, err = newDescriptor(layer); err != nil {
		return
	}

	// Blob
	blob := path(cfg.blobsDir(), desc.Digest.Hex)
	if cfg.verbose {
		fmt.Printf("mv %v %v\n", rel(cfg.buildDir(), target), rel(cfg.buildDir(), blob))
	}
	err = os.Rename(target, blob)
	return
}

func newDataTarball(root, target string, ignored []string, verbose bool) error {
	targetFile, err := os.Create(target)
	if err != nil {
		return err
	}
	defer targetFile.Close()

	gw := gzip.NewWriter(targetFile)
	defer gw.Close()

	tw := tar.NewWriter(gw)
	defer tw.Close()

	return filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip files explicitly ignored
		for _, v := range ignored {
			if info.Name() == v {
				if info.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
		}

		lnk := "" // if link, this will be used as the target
		if info.Mode()&fs.ModeSymlink != 0 {
			if lnk, err = validatedLinkTarget(root, path); err != nil {
				return err
			}
		}

		header, err := tar.FileInfoHeader(info, lnk)
		if err != nil {
			return err
		}

		relPath, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		header.Name = slashpath.Join("/func", filepath.ToSlash(relPath))
		if err := tw.WriteHeader(header); err != nil {
			return err
		}
		if verbose {
			fmt.Printf("→ %v \n", header.Name)
		}
		if !info.Mode().IsRegular() { //nothing more to do for non-regular
			return nil
		}

		file, err := os.Open(path)
		if err != nil {
			return err
		}
		defer file.Close()

		_, err = io.Copy(tw, file)
		return err
	})
}

// validatedLinkTarget returns the target of a given link or an error if
// that target is either absolute or outside the given project root.
func validatedLinkTarget(root, path string) (tgt string, err error) {
	// tgt is the raw target of the link.
	// This path is either absolute or relative to the link's location.
	tgt, err = os.Readlink(path)
	if err != nil {
		return tgt, fmt.Errorf("cannot read link: %w", err)
	}

	// Absolute links will not be correct when copied into the runtime
	// container, because they are placed into path into '/func',
	if filepath.IsAbs(tgt) {
		return tgt, errors.New("project may not contain absolute links")
	}

	// Calculate the actual target of the link
	// (relative to the parent of the symlink)
	lnkTgt := filepath.Join(filepath.Dir(path), tgt)

	// Calculate the relative path from the function's root to
	// this actual target location
	relLnkTgt, err := filepath.Rel(root, lnkTgt)
	if err != nil {
		return
	}

	// Fail if this path is outside the function's root.
	if strings.HasPrefix(relLnkTgt, ".."+string(filepath.Separator)) || relLnkTgt == ".." {
		return tgt, errors.New("links must stay within project root")
	}
	return
}

// newCertLayer creates the shared data layer in the container file hierarchy and
// returns both its descriptor and layer metadata.
func newCertsLayer(cfg *buildConfig) (desc v1.Descriptor, layer v1.Layer, err error) {

	// Create the data tarball
	// TODO: try WithCompressedCaching?
	source := filepath.Join(cfg.buildDir(), "ca-certificates.crt")
	target := path(cfg.buildDir(), "certslayer.tar.gz")

	if err = newCertsTarball(source, target, defaultIgnored, cfg.verbose); err != nil {
		return
	}

	// Layer
	if layer, err = tarball.LayerFromFile(target); err != nil {
		return
	}

	// Descriptor
	if desc, err = newDescriptor(layer); err != nil {
		return
	}

	// Blob
	blob := path(cfg.blobsDir(), desc.Digest.Hex)
	if cfg.verbose {
		fmt.Printf("mv %v %v\n", rel(cfg.buildDir(), target), rel(cfg.buildDir(), blob))
	}
	err = os.Rename(target, blob)
	return
}

func newCertsTarball(source, target string, ignored []string, verbose bool) error {
	targetFile, err := os.Create(target)
	if err != nil {
		return err
	}
	defer targetFile.Close()

	gw := gzip.NewWriter(targetFile)
	defer gw.Close()

	tw := tar.NewWriter(gw)
	defer tw.Close()

	paths := []string{
		"/etc/ssl/certs/ca-certificates.crt",
		"/etc/pki/tls/certs/ca-certificates.crt",
	}

	fi, err := os.Stat(source)
	if err != nil {
		return err
	}

	// For each ssl certs path we want to create
	for _, path := range paths {
		// Create a header for it
		header, err := tar.FileInfoHeader(fi, "")
		if err != nil {
			return err
		}
		header.Name = path

		if err := tw.WriteHeader(header); err != nil {
			return err
		}
		if verbose {
			fmt.Printf("→ %v \n", header.Name)
		}
		file, err := os.Open(source)
		if err != nil {
			return err
		}
		defer file.Close()
		_, err = io.Copy(tw, file)
		if err != nil {
			return err
		}
	}

	return nil
}

func newDescriptor(layer v1.Layer) (desc v1.Descriptor, err error) {
	size, err := layer.Size()
	if err != nil {
		return
	}
	digest, err := layer.Digest()
	if err != nil {
		return
	}
	return v1.Descriptor{
		MediaType: types.OCILayer,
		Size:      size,
		Digest:    digest,
	}, nil
}

// newImage creates an image for the given platform.
// The image consists of the shared data layer which is provided
func newImage(cfg *buildConfig, dataDesc v1.Descriptor, dataLayer v1.Layer, certsDesc v1.Descriptor, certsLayer v1.Layer, p v1.Platform, verbose bool) (imageDesc v1.Descriptor, err error) {
	buildFn, err := getLanguageLayerBuilder(cfg)
	if err != nil {
		return
	}

	// Write Exec Layer as Blob -> Layer
	execDesc, execLayer, err := buildFn(cfg, p)
	if err != nil {
		return
	}

	// Write Config Layer as Blob -> Layer
	configDesc, _, err := newConfig(cfg, p, dataLayer, certsLayer, execLayer)
	if err != nil {
		return
	}

	// Image Manifest
	image := v1.Manifest{
		SchemaVersion: 2,
		MediaType:     types.OCIManifestSchema1,
		Config:        configDesc,
		Layers:        []v1.Descriptor{dataDesc, certsDesc, execDesc},
	}

	// Write image manifest out as json to a tempfile
	filePath := fmt.Sprintf("image.%v.%v.json", p.OS, p.Architecture)
	file, err := os.Create(filePath)
	if err != nil {
		return
	}
	enc := json.NewEncoder(file)
	enc.SetIndent("", "  ")
	if err = enc.Encode(image); err != nil {
		return
	}
	if err = file.Close(); err != nil {
		return
	}

	// Create a descriptor from hash and size
	file, err = os.Open(filePath)
	if err != nil {
		return
	}
	hash, size, err := v1.SHA256(file)
	if err != nil {
		return
	}
	imageDesc = v1.Descriptor{
		MediaType: types.OCIManifestSchema1,
		Digest:    hash,
		Size:      size,
		Platform:  &p,
	}
	if err = file.Close(); err != nil {
		return
	}

	// move image into blobs
	blob := path(cfg.blobsDir(), hash.Hex)
	if cfg.verbose {
		fmt.Printf("mv %v %v\n", rel(cfg.buildDir(), filePath), rel(cfg.buildDir(), blob))
	}
	err = os.Rename(filePath, blob)
	return
}

func newConfig(cfg *buildConfig, p v1.Platform, layers ...v1.Layer) (desc v1.Descriptor, config v1.ConfigFile, err error) {
	volumes := make(map[string]struct{}) // Volumes are odd, see spec.
	for _, v := range cfg.f.Run.Volumes {
		if v.Path == nil {
			continue // TODO: remove pointers from Volume and Env struct members
		}
		volumes[*v.Path] = struct{}{}
	}

	rootfs := v1.RootFS{
		Type: "layers",
	}
	var diff v1.Hash
	for _, v := range layers {
		if v == nil {
			continue
		}
		if diff, err = v.DiffID(); err != nil {
			return
		}
		rootfs.DiffIDs = append(rootfs.DiffIDs, diff)
	}

	config = v1.ConfigFile{
		Created:      v1.Time{Time: cfg.t},
		Architecture: p.Architecture,
		OS:           p.OS,
		OSVersion:    p.OSVersion,
		// OSFeatures:   p.OSFeatures, // TODO: need to update dep to get this
		Variant: p.Variant,
		Config: v1.Config{
			ExposedPorts: map[string]struct{}{"8080/tcp": {}},
			Env:          newConfigEnvs(cfg),
			Cmd:          []string{"/func/f"}, // NOTE: Using Cmd because Entrypoint can not be overridden
			WorkingDir:   "/func/",
			StopSignal:   "SIGKILL",
			User:         "1000",
			Volumes:      volumes,
			// Labels
			// History
		},
		RootFS: rootfs,
	}

	// Write the config out as json to a tempfile
	filePath := path(cfg.buildDir(), "config.json")
	file, err := os.Create(filePath)
	if err != nil {
		return
	}
	enc := json.NewEncoder(file)
	enc.SetIndent("", "  ")
	if err = enc.Encode(config); err != nil {
		return
	}
	if err = file.Close(); err != nil {
		return
	}

	// Create a descriptor using hash and size
	file, err = os.Open(filePath)
	if err != nil {
		return
	}
	hash, size, err := v1.SHA256(file)
	if err != nil {
		return
	}
	desc = v1.Descriptor{
		MediaType: types.OCIConfigJSON,
		Digest:    hash,
		Size:      size,
	}
	if err = file.Close(); err != nil {
		return
	}

	// move config into blobs
	blobPath := path(cfg.blobsDir(), hash.Hex)
	if cfg.verbose {
		fmt.Printf("mv %v %v\n", rel(cfg.buildDir(), filePath), rel(cfg.buildDir(), blobPath))
	}
	err = os.Rename(filePath, blobPath)
	return
}

// newConfigEnvs returns the final set of environment variables to build into
// the container.  This consists of func-provided build metadata envs as well
// as any environment variables provided on the function itself.
func newConfigEnvs(cfg *buildConfig) []string {
	envs := []string{}

	// FUNC_CREATED
	// Formats container timestamp as RFC3339; a stricter version of the ISO 8601
	// format used by the container image manifest's 'Created' attribute.
	envs = append(envs, "FUNC_CREATED="+cfg.t.Format(time.RFC3339))

	// FUNC_VERSION
	// If source controlled, and if being built from a system with git, the
	// environment FUNC_VERSION will be populated.  Otherwise it will exist
	// (to indicate this logic was executed) but have an empty value.
	if cfg.verbose {
		fmt.Printf("cd %v && export FUNC_VERSION=$(git describe --tags)\n", cfg.f.Root)
	}
	cmd := exec.CommandContext(cfg.ctx, "git", "describe", "--tags")
	cmd.Dir = cfg.f.Root
	output, err := cmd.Output()
	if err != nil {
		if cfg.verbose {
			fmt.Fprintf(os.Stderr, "unable to determine function version. %v", err)
		}
		envs = append(envs, "FUNC_VERSION=")
	} else {
		envs = append(envs, "FUNC_VERSION="+strings.TrimSpace(string(output)))
	}

	// TODO: OTHERS?
	// Other metadata that may be useful. Perhaps:
	//   - func client version (func cli) used when building this file?
	//   - user/environment which triggered this build?
	//   - A reflection of the function itself?  Image, registry, etc. etc?

	// ENVs defined on the Function
	return append(envs, cfg.f.Run.Envs.Slice()...)
}

func newImageIndex(cfg *buildConfig, imageDescs []v1.Descriptor) (index v1.IndexManifest, err error) {
	index = v1.IndexManifest{
		SchemaVersion: 2,
		MediaType:     types.OCIImageIndex,
		Manifests:     imageDescs,
	}

	filePath := path(cfg.ociDir(), "index.json")
	file, err := os.Create(filePath)
	if err != nil {
		return
	}
	defer file.Close()

	enc := json.NewEncoder(file)
	enc.SetIndent("", "  ")
	err = enc.Encode(index)
	return
}

// rel is a simple prefix trim used exclusively for verbose debugging
// statements to print paths as relative to the current build directory
// rather than absolute. Returns the path relative to the current working
// build directory.  If it is not a subpath, the full path is returned
// unchanged.
func rel(base, path string) string {
	if strings.HasPrefix(path, base) {
		return "." + strings.TrimPrefix(path, base)
	}
	return path
}
