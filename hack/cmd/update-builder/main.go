package main

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"slices"
	"strings"
	"syscall"

	"golang.org/x/oauth2"

	"github.com/buildpacks/pack/builder"
	"github.com/buildpacks/pack/buildpackage"
	pack "github.com/buildpacks/pack/pkg/client"
	"github.com/buildpacks/pack/pkg/dist"
	bpimage "github.com/buildpacks/pack/pkg/image"
	"github.com/containerd/errdefs"
	"github.com/docker/docker/api/types/image"
	docker "github.com/docker/docker/client"
	"github.com/google/go-containerregistry/pkg/authn"
	ghAuth "github.com/google/go-containerregistry/pkg/authn/github"
	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/partial"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/remote/transport"
	"github.com/google/go-containerregistry/pkg/v1/types"
	"github.com/google/go-github/v68/github"
	"github.com/paketo-buildpacks/libpak/carton"
	"github.com/pelletier/go-toml"
)

func main() {
	// Set up context for possible signal inputs to not disrupt cleanup process.
	// This is not gonna do much for workflows since they finish and shutdown
	// but in case of local testing - don't leave left over resources on disk/RAM.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigs
		cancel()
		<-sigs
		os.Exit(130)
	}()

	var hadError bool
	for _, variant := range []string{"tiny", "base", "full"} {
		fmt.Println("::group::" + variant)
		err := buildBuilderImageMultiArch(ctx, variant)
		if err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
			hadError = true
		}
		fmt.Println("::endgroup::")
	}
	if hadError {
		fmt.Fprintln(os.Stderr, "failed to update builder")
		os.Exit(1)
	}
}

func buildBuilderImage(ctx context.Context, variant, version, arch, builderTomlPath string) (string, error) {

	newBuilderImage := "ghcr.io/knative/builder-jammy-" + variant
	newBuilderImageTagged := newBuilderImage + ":" + version + "-" + arch

	ref, err := name.ParseReference(newBuilderImageTagged)
	if err != nil {
		return "", fmt.Errorf("cannot parse reference to builder target: %w", err)
	}
	desc, err := remote.Head(ref, remote.WithAuthFromKeychain(DefaultKeychain), remote.WithContext(ctx))
	if err == nil {
		fmt.Fprintln(os.Stderr, "The image has been already built.")
		return newBuilderImage + "@" + desc.Digest.String(), nil
	}

	builderConfig, _, err := builder.ReadConfig(builderTomlPath)
	if err != nil {
		return "", fmt.Errorf("cannot parse builder.toml: %w", err)
	}

	fixupStacks(&builderConfig)
	err = updateJavaBuildpacks(ctx, &builderConfig, arch)
	if err != nil {
		return "", fmt.Errorf("cannot patch java buildpacks: %w", err)
	}
	if arch == "arm64" && variant == "base" {
		// dotnet,ruby&web-servers are not multiarch & we don't need them
		err = fixupRemoveUnusedBuildpacks(&builderConfig)
		if err != nil {
			return "", fmt.Errorf("failed to remove unused buildpacks: %w", err)
		}
	}

	addRustBuildpack(&builderConfig)

	var dockerClient docker.APIClient
	dockerClient, err = docker.NewClientWithOpts(docker.FromEnv, docker.WithAPIVersionNegotiation())
	if err != nil {
		return "", fmt.Errorf("cannot create docker client")
	}
	dockerClient = &hackDockerClient{dockerClient}

	packClient, err := pack.NewClient(pack.WithKeychain(DefaultKeychain), pack.WithDockerClient(dockerClient))
	if err != nil {
		return "", fmt.Errorf("cannot create pack client: %w", err)
	}

	createBuilderOpts := pack.CreateBuilderOptions{
		RelativeBaseDir: filepath.Dir(builderTomlPath),
		Targets: []dist.Target{
			{
				OS:   "linux",
				Arch: arch,
			},
		},
		BuilderName: newBuilderImageTagged,
		Config:      builderConfig,
		Publish:     true,
		PullPolicy:  bpimage.PullAlways,
		Labels: map[string]string{
			"org.opencontainers.image.description": "Paketo Jammy builder enriched with Rust and Quarkus buildpacks.",
			"org.opencontainers.image.source":      "https://github.com/knative/func",
			"org.opencontainers.image.vendor":      "https://github.com/knative/func",
			"org.opencontainers.image.url":         "https://github.com/knative/func/pkgs/container/builder-jammy-" + variant,
			"org.opencontainers.image.version":     version,
		},
	}

	err = packClient.CreateBuilder(ctx, createBuilderOpts)
	if err != nil {
		return "", fmt.Errorf("cannot create builder: %w", err)
	}

	// Get digest from registry after Pack published it
	newRef, err := name.ParseReference(newBuilderImageTagged)
	if err != nil {
		return "", fmt.Errorf("cannot parse reference: %w", err)
	}
	newDesc, err := remote.Head(newRef, remote.WithAuthFromKeychain(DefaultKeychain), remote.WithContext(ctx))
	if err != nil {
		return "", fmt.Errorf("cannot get image descriptor from registry: %w", err)
	}

	return newBuilderImage + "@" + newDesc.Digest.String(), nil
}

// Builds builder for each arch and creates manifest list
func buildBuilderImageMultiArch(ctx context.Context, variant string) error {
	ghClient := newGHClient(ctx)
	listOpts := &github.ListOptions{Page: 0, PerPage: 1}
	releases, ghResp, err := ghClient.Repositories.ListReleases(ctx, "paketo-buildpacks", "builder-jammy-"+variant, listOpts)
	if err != nil {
		return fmt.Errorf("cannot get upstream builder release: %w", err)
	}
	defer func(Body io.ReadCloser) {
		_ = Body.Close()
	}(ghResp.Body)

	if len(releases) <= 0 {
		return fmt.Errorf("cannot get latest release")
	}

	release := releases[0]

	if release.Name == nil {
		return fmt.Errorf("the name of the release is not defined")
	}
	if release.TarballURL == nil {
		return fmt.Errorf("the tarball url of the release is not defined")
	}

	buildDir, err := os.MkdirTemp("", "")
	if err != nil {
		return fmt.Errorf("cannot create temporary build directory: %w", err)
	}
	defer func(path string) {
		_ = os.RemoveAll(path)
	}(buildDir)

	builderTomlPath := filepath.Join(buildDir, "builder.toml")
	err = downloadBuilderToml(ctx, *release.TarballURL, builderTomlPath)
	if err != nil {
		return fmt.Errorf("cannot download builder toml: %w", err)
	}

	remoteOpts := []remote.Option{
		remote.WithAuthFromKeychain(DefaultKeychain),
		remote.WithContext(ctx),
	}

	idxRef, err := name.ParseReference("ghcr.io/knative/builder-jammy-" + variant + ":" + release.GetName())
	if err != nil {
		return fmt.Errorf("cannot parse image index ref: %w", err)
	}

	_, err = remote.Index(idxRef, remoteOpts...)
	if err != nil {
		if !isNotFound(err) {
			return fmt.Errorf("cannot get image index: %w", err)
		}
	} else {
		_, _ = fmt.Printf("index already present for tag: %s\n", release.GetName())
		return nil
	}

	err = copyStackImages(ctx, builderTomlPath)
	if err != nil {
		return fmt.Errorf("failed to copy over stack images for variant %v: %w", variant, err)
	}
	idx := mutate.IndexMediaType(empty.Index, types.DockerManifestList)
	idx = mutate.Annotations(idx, map[string]string{
		"org.opencontainers.image.description": "Paketo Jammy builder enriched with Rust and Quarkus buildpacks.",
		"org.opencontainers.image.source":      "https://github.com/knative/func",
		"org.opencontainers.image.vendor":      "https://github.com/knative/func",
		"org.opencontainers.image.url":         "https://github.com/knative/func/pkgs/container/builder-jammy-" + variant,
		"org.opencontainers.image.version":     *release.Name,
	}).(v1.ImageIndex)
	for _, arch := range []string{"arm64", "amd64"} {
		if arch == "arm64" && variant == "full" {
			_, _ = fmt.Fprintf(os.Stderr, "skipping arm64 build for variant: %q\n", variant)
			continue
		}

		var imgName string

		imgName, err = buildBuilderImage(ctx, variant, release.GetName(), arch, builderTomlPath)
		if err != nil {
			return err
		}

		imgRef, err := name.ParseReference(imgName)
		if err != nil {
			return fmt.Errorf("cannot parse image ref: %w", err)
		}
		img, err := remote.Image(imgRef, remoteOpts...)
		if err != nil {
			return fmt.Errorf("cannot get the image: %w", err)
		}
		cf, err := img.ConfigFile()
		if err != nil {
			return fmt.Errorf("cannot get config file for the image: %w", err)
		}

		newDesc, err := partial.Descriptor(img)
		if err != nil {
			return fmt.Errorf("cannot get partial descriptor for the image: %w", err)
		}
		newDesc.Platform = cf.Platform()

		idx = mutate.AppendManifests(idx, mutate.IndexAddendum{
			Add:        img,
			Descriptor: *newDesc,
		})
	}

	// Write index of variant with version
	err = remote.WriteIndex(idxRef, idx, remoteOpts...)
	if err != nil {
		return fmt.Errorf("cannot write image index: %w", err)
	}

	idxRef, err = name.ParseReference("ghcr.io/knative/builder-jammy-" + variant + ":v2")
	if err != nil {
		return fmt.Errorf("cannot parse image index ref: %w", err)
	}

	// Write preffered image to be pulled (v2 as successor to 'latest'
	// when incompatible changes were made). This is unfortunate but keeping
	// latest would break building any go function with pack builder with func
	// binary which does not contain the right scaffolding change.
	//
	// The incompatible change is removing the custom go buildpack for the upstream one
	err = remote.WriteIndex(idxRef, idx, remoteOpts...)
	if err != nil {
		return fmt.Errorf("cannot write image index: %w", err)
	}

	return nil
}

func isNotFound(err error) bool {
	var te *transport.Error
	if errors.As(err, &te) {
		return te.StatusCode == http.StatusNotFound
	}
	return false
}

type buildpack struct {
	repo      string
	version   string
	image     string
	patchFunc func(packageDesc *buildpackage.Config, bpDesc *dist.BuildpackDescriptor)
}

func buildBuildpackImage(ctx context.Context, bp buildpack, arch string) error {
	ghClient := newGHClient(ctx)

	var (
		release *github.RepositoryRelease
		ghResp  *github.Response
		err     error
	)

	if bp.version == "" {
		release, ghResp, err = ghClient.Repositories.GetLatestRelease(ctx, "paketo-buildpacks", bp.repo)
	} else {
		release, ghResp, err = ghClient.Repositories.GetReleaseByTag(ctx, "paketo-buildpacks", bp.repo, "v"+bp.version)
	}
	if err != nil {
		return fmt.Errorf("cannot get upstream builder release: %w", err)
	}
	defer func(Body io.ReadCloser) {
		_ = Body.Close()
	}(ghResp.Body)

	if release.TarballURL == nil {
		return fmt.Errorf("tarball url is nil")
	}
	if release.TagName == nil {
		return fmt.Errorf("tag name is nil")
	}

	version := strings.TrimPrefix(*release.TagName, "v")

	imageNameTagged := bp.image + ":" + version
	srcDir, err := os.MkdirTemp("", "src-*")
	if err != nil {
		return fmt.Errorf("cannot create temp dir: %w", err)
	}

	err = downloadTarball(*release.TarballURL, srcDir)
	if err != nil {
		return fmt.Errorf("cannot download source code: %w", err)
	}

	packageDir := filepath.Join(srcDir, "out")
	p := carton.Package{
		CacheLocation:           "",
		DependencyFilters:       nil,
		StrictDependencyFilters: false,
		IncludeDependencies:     false,
		Destination:             packageDir,
		Source:                  srcDir,
		Version:                 version,
	}
	eh := exitHandler{}
	p.Create(carton.WithExitHandler(&eh))
	if eh.err != nil {
		return fmt.Errorf("cannot create package: %w", eh.err)
	}
	if eh.fail {
		return fmt.Errorf("cannot create package")
	}

	// set URI and OS in package.toml
	f, err := os.OpenFile(filepath.Join(srcDir, "package.toml"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return fmt.Errorf("cannot open package.toml: %w", err)
	}
	defer func(f *os.File) {
		_ = f.Close()
	}(f)
	_, err = fmt.Fprintf(f, "[buildpack]\nuri = \"%s\"\n\n[platform]\nos = \"%s\"\n", packageDir, "linux")
	_ = f.Close()
	if err != nil {
		return fmt.Errorf("cannot append to package.toml: %w", err)
	}

	cfgReader := buildpackage.NewConfigReader()
	cfg, err := cfgReader.Read(filepath.Join(srcDir, "package.toml"))
	if err != nil {
		return fmt.Errorf("cannot read buildpack config: %w", err)
	}

	if bp.patchFunc != nil {
		var bpDesc dist.BuildpackDescriptor
		var bs []byte
		bpDescPath := filepath.Join(packageDir, "buildpack.toml")
		bs, err = os.ReadFile(bpDescPath)
		if err != nil {
			return fmt.Errorf("cannot read buildpack.toml: %w", err)
		}
		err = toml.Unmarshal(bs, &bpDesc)
		if err != nil {
			return fmt.Errorf("cannot unmarshall buildpack descriptor: %w", err)
		}
		bp.patchFunc(&cfg, &bpDesc)
		bs, err = toml.Marshal(&bpDesc)
		if err != nil {
			return fmt.Errorf("cannot marshal buildpack descriptor: %w", err)
		}
		err = os.WriteFile(bpDescPath, bs, 0644)
		if err != nil {
			return fmt.Errorf("cannot write buildpack.toml: %w", err)
		}
	}

	pbo := pack.PackageBuildpackOptions{
		RelativeBaseDir: packageDir,
		Name:            imageNameTagged,
		Format:          pack.FormatImage,
		Config:          cfg,
		Publish:         true,
		PullPolicy:      bpimage.PullAlways,
		Registry:        "",
		Flatten:         false,
		FlattenExclude:  nil,
		Targets: []dist.Target{
			{
				OS:   "linux",
				Arch: arch,
			},
		},
	}
	packClient, err := pack.NewClient(pack.WithKeychain(DefaultKeychain))
	if err != nil {
		return fmt.Errorf("cannot create pack client: %w", err)
	}
	err = packClient.PackageBuildpack(ctx, pbo)
	if err != nil {
		return fmt.Errorf("cannot package buildpack: %w", err)
	}

	return nil
}

type exitHandler struct {
	err  error
	fail bool
}

func (e *exitHandler) Error(err error) {
	e.err = err
}

func (e *exitHandler) Fail() {
	e.fail = true
}

func (e *exitHandler) Pass() {
}

func downloadBuilderToml(ctx context.Context, tarballUrl, builderTomlPath string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, tarballUrl, nil)
	if err != nil {
		return fmt.Errorf("cannot create request for release tarball: %w", err)
	}
	//nolint:bodyclose
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("cannot get release tarball: %w", err)
	}
	defer func(Body io.ReadCloser) {
		_ = Body.Close()
	}(resp.Body)

	gr, err := gzip.NewReader(resp.Body)
	if err != nil {
		return fmt.Errorf("cannot create gzip stream from release tarball: %w", err)
	}
	defer func(gr *gzip.Reader) {
		_ = gr.Close()
	}(gr)
	tr := tar.NewReader(gr)
	for {
		hdr, err := tr.Next()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return fmt.Errorf("error while processing release tarball: %w", err)
		}

		if hdr.FileInfo().Mode().Type() != 0 || !strings.HasSuffix(hdr.Name, "/builder.toml") {
			continue
		}
		builderToml, err := os.OpenFile(builderTomlPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0644)
		if err != nil {
			return fmt.Errorf("cannot create builder.toml file: %w", err)
		}
		_, err = io.CopyN(builderToml, tr, hdr.Size)
		if err != nil {
			return fmt.Errorf("cannot copy data to builder.toml file: %w", err)
		}
		break
	}

	return nil
}

// Adds custom Rust buildpack to the builder.
func addRustBuildpack(config *builder.Config) {
	config.Description += "\nAddendum: this builder contains community multi-arch Rust buildpack."
	additionalBuildpacks := []builder.ModuleConfig{
		{
			ModuleInfo: dist.ModuleInfo{
				ID:      "paketo-community/rust",
				Version: "0.65.0",
			},
			ImageOrURI: dist.ImageOrURI{
				BuildpackURI: dist.BuildpackURI{URI: "docker://docker.io/paketocommunity/rust:0.65.0"},
			},
		},
	}

	additionalGroups := []dist.OrderEntry{
		{
			Group: []dist.ModuleRef{
				{
					ModuleInfo: dist.ModuleInfo{
						ID: "paketo-community/rust",
					},
				},
			},
		},
	}

	config.Buildpacks = append(additionalBuildpacks, config.Buildpacks...)
	config.Order = append(additionalGroups, config.Order...)
}

// updated java and java-native-image buildpack to include quarkus buildpack
func updateJavaBuildpacks(ctx context.Context, builderConfig *builder.Config, arch string) error {
	var err error

	for _, entry := range builderConfig.Order {
		bp := strings.TrimPrefix(entry.Group[0].ID, "paketo-buildpacks/")
		if bp == "java" || bp == "java-native-image" {
			img := "ghcr.io/knative/buildpacks/" + bp
			err = buildBuildpackImage(ctx, buildpack{
				repo:      bp,
				version:   entry.Group[0].Version,
				image:     img,
				patchFunc: addQuarkusBuildpack,
			}, arch)
			// TODO we might want to push these images to registry
			// but it's not absolutely necessary since they are included in builder
			if err != nil {
				return fmt.Errorf("cannot build %q buildpack: %w", bp, err)
			}
			for i := range builderConfig.Buildpacks {
				if strings.HasPrefix(builderConfig.Buildpacks[i].URI, "docker://docker.io/paketobuildpacks/"+bp+":") {
					builderConfig.Buildpacks[i].URI = "docker://ghcr.io/knative/buildpacks/" + bp + ":" + entry.Group[0].Version
				}
			}
		}
	}
	return nil
}

// patches "Java" or "Java Native Image" buildpacks to include Quarkus BP just before Maven BP
func addQuarkusBuildpack(packageDesc *buildpackage.Config, bpDesc *dist.BuildpackDescriptor) {
	ghClient := newGHClient(context.Background())

	rr, resp, err := ghClient.Repositories.GetLatestRelease(context.TODO(), "paketo-buildpacks", "quarkus")
	if err != nil {
		panic(err)
	}
	defer func(Body io.ReadCloser) {
		_ = Body.Close()
	}(resp.Body)

	latestQuarkusVersion := strings.TrimPrefix(*rr.TagName, "v")

	packageDesc.Dependencies = append(packageDesc.Dependencies, dist.ImageOrURI{
		BuildpackURI: dist.BuildpackURI{
			URI: "docker://index.docker.io/paketobuildpacks/quarkus:" + latestQuarkusVersion,
		},
	})
	quarkusBP := dist.ModuleRef{
		ModuleInfo: dist.ModuleInfo{
			ID:      "paketo-buildpacks/quarkus",
			Version: latestQuarkusVersion,
		},
		Optional: true,
	}
	idx := slices.IndexFunc(bpDesc.WithOrder[0].Group, func(ref dist.ModuleRef) bool {
		return ref.ID == "paketo-buildpacks/maven"
	})
	bpDesc.WithOrder[0].Group = slices.Insert(bpDesc.WithOrder[0].Group, idx, quarkusBP)
}

func downloadTarball(tarballUrl, destDir string) error {
	//nolint:bodyclose
	resp, err := http.Get(tarballUrl)
	if err != nil {
		return fmt.Errorf("cannot get tarball: %w", err)
	}
	defer func(Body io.ReadCloser) {
		_ = Body.Close()
	}(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("cannot get tarball: %s", resp.Status)
	}
	defer func(Body io.ReadCloser) {
		_ = Body.Close()
	}(resp.Body)

	gzipReader, err := gzip.NewReader(resp.Body)
	if err != nil {
		return fmt.Errorf("cannot create gzip reader: %w", err)
	}
	defer func(gzipReader *gzip.Reader) {
		_ = gzipReader.Close()
	}(gzipReader)

	tarReader := tar.NewReader(gzipReader)
	var hdr *tar.Header
	for {
		hdr, err = tarReader.Next()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return fmt.Errorf("cannot read tar header: %w", err)
		}
		if strings.Contains(hdr.Name, "..") {
			return fmt.Errorf("file name in tar header contains '..'")
		}

		n := filepath.Clean(filepath.Join(strings.Split(hdr.Name, "/")[1:]...))
		if strings.HasPrefix(n, "..") {
			return fmt.Errorf("path in tar header escapes")
		}
		dest := filepath.Join(destDir, n)

		switch hdr.Typeflag {
		case tar.TypeReg:
			var f *os.File
			f, err = os.OpenFile(dest, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, os.FileMode(hdr.Mode&0777))
			if err != nil {
				return fmt.Errorf("cannot create a file: %w", err)
			}
			_, err = io.Copy(f, tarReader)
			_ = f.Close()
			if err != nil {
				return fmt.Errorf("cannot read from tar reader: %w", err)
			}
		case tar.TypeSymlink:
			err = os.Symlink(hdr.Linkname, dest)
			if err != nil {
				return fmt.Errorf("cannot create a symlink: %w", err)
			}
		case tar.TypeDir:
			err = os.MkdirAll(dest, 0755)
			if err != nil {
				return fmt.Errorf("cannot create a directory: %w", err)
			}
		case tar.TypeXGlobalHeader:
			// ignore this type
		default:
			return fmt.Errorf("unknown type: %x", hdr.Typeflag)
		}
	}
	return nil
}

func newGHClient(ctx context.Context) *github.Client {
	return github.NewClient(oauth2.NewClient(ctx, oauth2.StaticTokenSource(&oauth2.Token{
		AccessToken: os.Getenv("GITHUB_TOKEN"),
	})))
}

var DefaultKeychain = authn.NewMultiKeychain(ghAuth.Keychain, authn.DefaultKeychain)

// Hack implementation of docker client returns NotFound for images ghcr.io/knative/buildpacks/*
// For some reason moby/docker erroneously returns 500 HTTP code for these missing images.
// Interestingly podman correctly returns 404 for same request.
type hackDockerClient struct {
	docker.APIClient
}

func (c hackDockerClient) ImagePull(ctx context.Context, ref string, options image.PullOptions) (io.ReadCloser, error) {
	if strings.HasPrefix(ref, "ghcr.io/knative/buildpacks/") {
		return nil, fmt.Errorf("this image is supposed to exist only in daemon: %w", errdefs.ErrNotFound)
	}
	return c.APIClient.ImagePull(ctx, ref, options)
}

// update metadata in builders so that we use knative (ghcr) based images
// instead of docker registry
func fixupStacks(builderConfig *builder.Config) {
	newBuilder := stackImageToMirror(builderConfig.Stack.BuildImage)
	builderConfig.Stack.BuildImage = newBuilder
	builderConfig.Build.Image = newBuilder

	newRun := stackImageToMirror(builderConfig.Stack.RunImage)
	builderConfig.Stack.RunImage = newRun
	builderConfig.Run.Images = []builder.RunImageConfig{{
		Image: newRun,
	}}
}

func copyImage(ctx context.Context, srcRef, destRef string) error {
	_, _ = fmt.Fprintf(os.Stderr, "## copying: %s => %s\n", srcRef, destRef)
	cmd := exec.CommandContext(ctx, "skopeo", "copy",
		"--multi-arch=all",
		"docker://"+srcRef,
		"docker://"+destRef,
	)
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stdout
	err := cmd.Run()
	if err != nil {
		return fmt.Errorf("error while running skopeo: %w", err)
	}
	return nil
}

func stackImageToMirror(ref string) string {
	parts := strings.Split(ref, "/")
	lastPart := parts[len(parts)-1]
	switch {
	case strings.HasPrefix(lastPart, "build-"):
		return "localhost:5000/" + lastPart
	case strings.HasPrefix(lastPart, "run-"):
		return "ghcr.io/knative/" + lastPart
	default:
		panic("non reachable")
	}
}

func fixupRemoveUnusedBuildpacks(c *builder.Config) error {
	// these contain single-arch buildpacks so this also removes multi-arch issues
	unusedBuildpacks := []string{"dotnet", "ruby", "web-servers"}

	var buildpacksToDel []int
	for i, config := range c.Buildpacks {
		for _, unused := range unusedBuildpacks {
			if strings.Contains(config.URI, unused) {
				buildpacksToDel = append(buildpacksToDel, i)
				break
			}
		}
	}
	for i := len(buildpacksToDel) - 1; i >= 0; i-- {
		idx := buildpacksToDel[i]
		c.Buildpacks = slices.Delete(c.Buildpacks, idx, idx+1)
	}

	// Remove all orders containing unused buildpacks
	var ordersToDel []int
	for i, entry := range c.Order {
		found := false
		for _, g := range entry.Group {
			for _, unused := range unusedBuildpacks {
				if strings.Contains(g.ID, unused) {
					found = true
					break
				}
			}
			if found {
				break
			}
		}
		if found {
			ordersToDel = append(ordersToDel, i)
		}
	}
	for i := len(ordersToDel) - 1; i >= 0; i-- {
		idx := ordersToDel[i]
		c.Order = slices.Delete(c.Order, idx, idx+1)
	}

	return nil
}

// Copy build & run images from stack so that we don't use docker registry based images
// Run images are simply copied over to ghcr
func copyStackImages(ctx context.Context, configPath string) error {
	var err error

	builderConfig, _, err := builder.ReadConfig(configPath)
	if err != nil {
		return fmt.Errorf("cannot parse builder.toml: %w", err)
	}

	runImage := builderConfig.Stack.RunImage
	buildImage := builderConfig.Stack.BuildImage

	err = copyImage(ctx, runImage, stackImageToMirror(runImage))
	if err != nil {
		return fmt.Errorf("cannot mirror run image: %w", err)
	}
	err = copyImage(ctx, buildImage, stackImageToMirror(buildImage))
	if err != nil {
		return fmt.Errorf("cannot mirror build image: %w", err)
	}
	return nil
}
