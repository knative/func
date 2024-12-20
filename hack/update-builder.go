package main

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"syscall"

	"golang.org/x/oauth2"
	"golang.org/x/term"

	"github.com/buildpacks/pack/builder"
	"github.com/buildpacks/pack/buildpackage"
	pack "github.com/buildpacks/pack/pkg/client"
	"github.com/buildpacks/pack/pkg/dist"
	bpimage "github.com/buildpacks/pack/pkg/image"
	"github.com/containerd/errdefs"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/registry"
	docker "github.com/docker/docker/client"
	"github.com/docker/docker/pkg/jsonmessage"
	"github.com/google/go-containerregistry/pkg/authn"
	ghAuth "github.com/google/go-containerregistry/pkg/authn/github"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/partial"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/types"
	"github.com/google/go-github/v49/github"
	"github.com/paketo-buildpacks/libpak/carton"
	"github.com/pelletier/go-toml"
)

func main() {
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
		os.Exit(1)
	}
}

func buildBuilderImage(ctx context.Context, variant, arch string) (string, error) {
	buildDir, err := os.MkdirTemp("", "")
	if err != nil {
		return "", fmt.Errorf("cannot create temporary build directory: %w", err)
	}
	defer func(path string) {
		_ = os.RemoveAll(path)
	}(buildDir)

	ghClient := newGHClient(ctx)
	listOpts := &github.ListOptions{Page: 0, PerPage: 1}
	releases, ghResp, err := ghClient.Repositories.ListReleases(ctx, "paketo-buildpacks", "builder-jammy-"+variant, listOpts)
	if err != nil {
		return "", fmt.Errorf("cannot get upstream builder release: %w", err)
	}
	defer func(Body io.ReadCloser) {
		_ = Body.Close()
	}(ghResp.Body)

	if len(releases) <= 0 {
		return "", fmt.Errorf("cannot get latest release")
	}

	release := releases[0]

	if release.Name == nil {
		return "", fmt.Errorf("the name of the release is not defined")
	}
	if release.TarballURL == nil {
		return "", fmt.Errorf("the tarball url of the release is not defined")
	}
	newBuilderImage := "ghcr.io/knative/builder-jammy-" + variant
	newBuilderImageTagged := newBuilderImage + ":" + *release.Name + "-" + arch

	ref, err := name.ParseReference(newBuilderImageTagged)
	if err != nil {
		return "", fmt.Errorf("cannot parse reference to builder target: %w", err)
	}
	desc, err := remote.Head(ref, remote.WithAuthFromKeychain(DefaultKeychain), remote.WithContext(ctx))
	if err == nil {
		fmt.Fprintln(os.Stderr, "The image has been already built.")
		return newBuilderImage + "@" + desc.Digest.String(), nil
	}

	builderTomlPath := filepath.Join(buildDir, "builder.toml")
	err = downloadBuilderToml(ctx, *release.TarballURL, builderTomlPath)
	if err != nil {
		return "", fmt.Errorf("cannot download builder toml: %w", err)
	}

	builderConfig, _, err := builder.ReadConfig(builderTomlPath)
	if err != nil {
		return "", fmt.Errorf("cannot parse builder.toml: %w", err)
	}

	err = updateJavaBuildpacks(ctx, &builderConfig, arch)
	if err != nil {
		return "", fmt.Errorf("cannot patch java buildpacks: %w", err)
	}
	addGoAndRustBuildpacks(&builderConfig)

	var dockerClient docker.CommonAPIClient
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
		RelativeBaseDir: buildDir,
		Targets: []dist.Target{
			{
				OS:   "linux",
				Arch: arch,
			},
		},
		BuilderName: newBuilderImageTagged,
		Config:      builderConfig,
		Publish:     false,
		PullPolicy:  bpimage.PullAlways,
		Labels: map[string]string{
			"org.opencontainers.image.description": "Paketo Jammy builder enriched with Rust and Func-Go buildpacks.",
			"org.opencontainers.image.source":      "https://github.com/knative/func",
			"org.opencontainers.image.vendor":      "https://github.com/knative/func",
			"org.opencontainers.image.url":         "https://github.com/knative/func/pkgs/container/builder-jammy-" + variant,
			"org.opencontainers.image.version":     *release.Name,
		},
	}

	err = packClient.CreateBuilder(ctx, createBuilderOpts)
	if err != nil {
		return "", fmt.Errorf("canont create builder: %w", err)
	}

	pushImage := func(img string) (string, error) {
		regAuth, err := dockerDaemonAuthStr(img)
		if err != nil {
			return "", fmt.Errorf("cannot get credentials: %w", err)
		}
		imagePushOptions := image.PushOptions{
			All:          false,
			RegistryAuth: regAuth,
		}

		rc, err := dockerClient.ImagePush(ctx, img, imagePushOptions)
		if err != nil {
			return "", fmt.Errorf("cannot initialize image push: %w", err)
		}
		defer func(rc io.ReadCloser) {
			_ = rc.Close()
		}(rc)

		pr, pw := io.Pipe()
		digestCh := make(chan string, 1)
		go func() {
			var (
				jm  jsonmessage.JSONMessage
				dec = json.NewDecoder(pr)
				err error
			)
			for {
				err = dec.Decode(&jm)
				if err != nil {
					if errors.Is(err, io.EOF) {
						break
					}
					panic(err)
				}
				if jm.Error != nil {
					continue
				}

				re := regexp.MustCompile(`\sdigest: (?P<hash>sha256:[a-zA-Z0-9]+)\s`)
				matches := re.FindStringSubmatch(jm.Status)
				if len(matches) == 2 {
					digestCh <- matches[1]
				}
			}
		}()
		r := io.TeeReader(rc, pw)

		fd := os.Stdout.Fd()
		isTerminal := term.IsTerminal(int(os.Stdout.Fd()))
		err = jsonmessage.DisplayJSONMessagesStream(r, os.Stderr, fd, isTerminal, nil)
		_ = pw.Close()
		if err != nil {
			return "", err
		}

		return <-digestCh, nil
	}

	var d string
	d, err = pushImage(newBuilderImageTagged)
	if err != nil {
		return "", fmt.Errorf("cannot push the image: %w", err)
	}

	return newBuilderImage + "@" + d, nil
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

	remoteOpts := []remote.Option{
		remote.WithAuthFromKeychain(DefaultKeychain),
		remote.WithContext(ctx),
	}

	idx := mutate.IndexMediaType(empty.Index, types.DockerManifestList)
	for _, arch := range []string{"arm64", "amd64"} {
		if arch == "arm64" && variant != "tiny" {
			_, _ = fmt.Fprintf(os.Stderr, "skipping arm64 build for variant: %q\n", variant)
			continue
		}

		var imgName string

		imgName, err = buildBuilderImage(ctx, variant, arch)
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

	idxRef, err := name.ParseReference("ghcr.io/knative/builder-jammy-" + variant + ":" + *release.Name)
	if err != nil {
		return fmt.Errorf("cannot parse image index ref: %w", err)
	}

	err = remote.WriteIndex(idxRef, idx, remoteOpts...)
	if err != nil {
		return fmt.Errorf("cannot write image index: %w", err)
	}

	idxRef, err = name.ParseReference("ghcr.io/knative/builder-jammy-" + variant + ":latest")
	if err != nil {
		return fmt.Errorf("cannot parse image index ref: %w", err)
	}

	err = remote.WriteIndex(idxRef, idx, remoteOpts...)
	if err != nil {
		return fmt.Errorf("cannot write image index: %w", err)
	}

	return nil
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

	fmt.Println("src tar url:", *release.TarballURL)

	imageNameTagged := bp.image + ":" + version
	srcDir, err := os.MkdirTemp("", "src-*")
	if err != nil {
		return fmt.Errorf("cannot create temp dir: %w", err)
	}

	fmt.Println("imageNameTagged:", imageNameTagged)
	fmt.Println("srcDir:", srcDir)

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
		return fmt.Errorf("cannot apped to package.toml: %w", err)
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
		Publish:         false,
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

type auth struct {
	uname, pwd string
}

func (a auth) Authorization() (*authn.AuthConfig, error) {
	return &authn.AuthConfig{
		Username: a.uname,
		Password: a.pwd,
	}, nil
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

// Adds custom Rust and Go-Function buildpacks to the builder.
func addGoAndRustBuildpacks(config *builder.Config) {
	config.Description += "\nAddendum: this is modified builder that also contains Rust and Func-Go buildpacks."
	additionalBuildpacks := []builder.ModuleConfig{
		{
			ModuleInfo: dist.ModuleInfo{
				ID:      "paketo-community/rust",
				Version: "0.47.0",
			},
			ImageOrURI: dist.ImageOrURI{
				BuildpackURI: dist.BuildpackURI{URI: "docker://docker.io/paketocommunity/rust:0.47.0"},
			},
		},
		{
			ModuleInfo: dist.ModuleInfo{
				ID:      "dev.knative-extensions.go",
				Version: "0.0.6",
			},
			ImageOrURI: dist.ImageOrURI{
				BuildpackURI: dist.BuildpackURI{URI: "ghcr.io/boson-project/go-function-buildpack:0.0.6"},
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
		{
			Group: []dist.ModuleRef{
				{
					ModuleInfo: dist.ModuleInfo{
						ID: "paketo-buildpacks/go-dist",
					},
				},
				{
					ModuleInfo: dist.ModuleInfo{
						ID: "dev.knative-extensions.go",
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
				if strings.HasPrefix(builderConfig.Buildpacks[i].URI, "docker://gcr.io/paketo-buildpacks/"+bp+":") {
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
			URI: "docker://gcr.io/paketo-buildpacks/quarkus:" + latestQuarkusVersion,
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
	if resp.StatusCode != 200 {
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
			return fmt.Errorf("symlinks are not supported yet")
		case tar.TypeDir:
			err = os.MkdirAll(dest, 0755)
			if err != nil {
				return fmt.Errorf("cannmot create a directory: %w", err)
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

func dockerDaemonAuthStr(img string) (string, error) {
	ref, err := name.ParseReference(img)
	if err != nil {
		return "", err
	}

	a, err := DefaultKeychain.Resolve(ref.Context())
	if err != nil {
		return "", err
	}

	ac, err := a.Authorization()
	if err != nil {
		return "", err
	}

	authConfig := registry.AuthConfig{
		Username: ac.Username,
		Password: ac.Password,
	}

	bs, err := json.Marshal(&authConfig)
	if err != nil {
		return "", err
	}

	return base64.StdEncoding.EncodeToString(bs), nil
}

// Hack implementation of docker client returns NotFound for images ghcr.io/knative/buildpacks/*
// For some reason moby/docker erroneously returns 500 HTTP code for these missing images.
// Interestingly podman correctly returns 404 for same request.
type hackDockerClient struct {
	docker.CommonAPIClient
}

func (c hackDockerClient) ImagePull(ctx context.Context, ref string, options image.PullOptions) (io.ReadCloser, error) {
	if strings.HasPrefix(ref, "ghcr.io/knative/buildpacks/") {
		return nil, fmt.Errorf("this image is supposed to exist only in daemon: %w", errdefs.ErrNotFound)
	}
	return c.CommonAPIClient.ImagePull(ctx, ref, options)
}
