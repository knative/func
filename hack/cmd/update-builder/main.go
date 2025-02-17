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
	"os/exec"
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
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/partial"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/types"
	"github.com/google/go-github/v68/github"
	"github.com/paketo-buildpacks/libpak/carton"
	"github.com/pelletier/go-toml"
)

func main() {
	// Set up context for possible signal inputs to not disrupt cleanup process.
	// This is not gonna do much for workflows since they finish and shutdown
	// but in case of local testing - dont leave left over resources on disk/RAM.
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

	// temporary fix, for some reason paketo does not distribute several buildpacks for ARM64
	// we need ot fix that up
	if arch == "arm64" {
		err = fixupGoBuildpackARM64(ctx, &builderConfig)
		if err != nil {
			return "", fmt.Errorf("cannot fix Go buildpack: %w", err)
		}
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
		return "", fmt.Errorf("cannont create builder: %w", err)
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
		r := io.TeeReader(rc, pw)

		go func() {
			fd := os.Stdout.Fd()
			isTerminal := term.IsTerminal(int(os.Stdout.Fd()))
			e := jsonmessage.DisplayJSONMessagesStream(pr, os.Stderr, fd, isTerminal, nil)
			_ = pr.CloseWithError(e)
		}()

		var (
			digest string
			jm     jsonmessage.JSONMessage
			dec    = json.NewDecoder(r)
			re     = regexp.MustCompile(`\sdigest: (?P<hash>sha256:[a-zA-Z0-9]+)\s`)
		)
		for {
			err = dec.Decode(&jm)
			if err != nil {
				if errors.Is(err, io.EOF) {
					break
				}
				return "", err
			}
			if jm.Error != nil {
				continue
			}

			matches := re.FindStringSubmatch(jm.Status)
			if len(matches) == 2 {
				digest = matches[1]
				_, _ = io.Copy(io.Discard, r)
				break
			}
		}

		if digest == "" {
			return "", fmt.Errorf("digest not found")
		}
		return digest, nil
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
	idx = mutate.Annotations(idx, map[string]string{
		"org.opencontainers.image.description": "Paketo Jammy builder enriched with Rust and Func-Go buildpacks.",
		"org.opencontainers.image.source":      "https://github.com/knative/func",
		"org.opencontainers.image.vendor":      "https://github.com/knative/func",
		"org.opencontainers.image.url":         "https://github.com/knative/func/pkgs/container/builder-jammy-" + variant,
		"org.opencontainers.image.version":     *release.Name,
	}).(v1.ImageIndex)
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

func getReleaseByVersion(ctx context.Context, repo, vers string) (*github.RepositoryRelease, error) {
	ghClient := newGHClient(ctx)

	listOpts := &github.ListOptions{Page: 0, PerPage: 10}
	releases, resp, err := ghClient.Repositories.ListReleases(ctx, "paketo-buildpacks", repo, listOpts)
	if err != nil {
		return nil, fmt.Errorf("cannot get releases: %w", err)
	}
	defer func(Body io.ReadCloser) {
		_ = Body.Close()
	}(resp.Body)

	for _, r := range releases {
		if strings.TrimPrefix(*r.TagName, "v") == vers {
			return r, nil
		}
	}
	return nil, errors.New("release not found")
}

func fixupGoBuildpackARM64(ctx context.Context, config *builder.Config) error {
	var (
		goBuildpackIndex   int
		goBuildpackVersion string
	)
	for i, moduleConfig := range config.Buildpacks {
		uri := moduleConfig.ImageOrURI.URI
		if strings.Contains(uri, "paketo-buildpacks/go:") {
			goBuildpackIndex = i
			goBuildpackVersion = uri[strings.LastIndex(uri, ":")+1:]
			break
		}
	}
	if goBuildpackVersion == "" {
		return fmt.Errorf("go buildpack not found in the config")
	}

	buildDir, err := os.MkdirTemp("", "build-dir-*")
	if err != nil {
		return fmt.Errorf("cannot create temp dir: %w", err)
	}
	// sic! do not defer remove

	goBuildpackSrcDir := filepath.Join(buildDir, "go")

	goBuildpackRelease, err := getReleaseByVersion(ctx, "go", goBuildpackVersion)
	if err != nil {
		return fmt.Errorf("cannot get Go release: %w", err)
	}

	err = downloadTarball(*goBuildpackRelease.TarballURL, goBuildpackSrcDir)
	if err != nil {
		return fmt.Errorf("cannot download Go buildpack source code: %w", err)
	}

	cfgReader := buildpackage.NewConfigReader()
	packageConfig, err := cfgReader.Read(filepath.Join(goBuildpackSrcDir, "package.toml"))
	if err != nil {
		return fmt.Errorf("cannot read Go buildpack config: %w", err)
	}

	buildBuildpack := func(name, version string) error {
		srcDir := filepath.Join(buildDir, name)
		cmd := exec.CommandContext(ctx, "./scripts/package.sh", "--version", version)
		cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
		cmd.Dir = srcDir
		cmd.Env = append(os.Environ(), "GOARCH=arm64")
		err = cmd.Run()
		if err != nil {
			return fmt.Errorf("build of buildpack %q failed: %w", name, err)
		}
		return nil
	}

	type patchSourceFn = func(srcDir string) error
	// these buildpacks need rebuild since they are only amd64 in paketo upstream
	needsRebuild := map[string]patchSourceFn{
		"git":           nil,
		"go-build":      nil,
		"go-mod-vendor": nil,
		"go-dist": func(srcDir string) error {
			return fixupGoDistPkgRefs(filepath.Join(srcDir, "buildpack.toml"), "arm64")
		},
	}

	re := regexp.MustCompile(`^urn:cnb:registry:paketo-buildpacks/([\w-]+)@([\d.]+)$`)
	for i, dep := range packageConfig.Dependencies {
		m := re.FindStringSubmatch(dep.BuildpackURI.URI)
		if len(m) != 3 {
			return fmt.Errorf("cannot match buildpack name")
		}
		buildpackName := m[1]
		buildpackVersion := m[2]

		patch, ok := needsRebuild[buildpackName]
		if !ok {
			// this dependency does not require rebuild for arm64
			continue
		}

		var rel *github.RepositoryRelease
		rel, err = getReleaseByVersion(ctx, buildpackName, buildpackVersion)
		if err != nil {
			return fmt.Errorf("cannot get release: %w", err)
		}

		srcDir := filepath.Join(buildDir, buildpackName)

		err = downloadTarball(*rel.TarballURL, srcDir)
		if err != nil {
			return fmt.Errorf("cannot get tarball: %w", err)
		}
		if patch != nil {
			err = patch(srcDir)
			if err != nil {
				return fmt.Errorf("cannot patch source code: %w", err)
			}
		}

		err = buildBuildpack(buildpackName, buildpackVersion)
		if err != nil {
			return err
		}

		packageConfig.Dependencies[i].URI = "file://" + filepath.Join(srcDir, "build", "buildpackage.cnb")

	}

	bs, err := toml.Marshal(&packageConfig)
	err = os.WriteFile(filepath.Join(goBuildpackSrcDir, "package.toml"), bs, 0644)
	if err != nil {
		return fmt.Errorf("cannot update package.toml: %w", err)
	}

	err = buildBuildpack("go", goBuildpackVersion)
	if err != nil {
		return err
	}

	config.Buildpacks[goBuildpackIndex].BuildpackURI.URI = "file://" + filepath.Join(goBuildpackSrcDir, "build", "buildpackage.cnb")
	fmt.Println(goBuildpackSrcDir)
	return nil
}

// The paketo go-dist buildpack refer to the amd64 version of Go.
// This function replaces these references with references to the arm64 version.
func fixupGoDistPkgRefs(buildpackToml, arch string) error {
	tomlBytes, err := os.ReadFile(buildpackToml)
	if err != nil {
		return err
	}

	var config any
	err = toml.Unmarshal(tomlBytes, &config)
	if err != nil {
		return err
	}
	deps := config.(map[string]any)["metadata"].(map[string]any)["dependencies"].([]map[string]any)

	versions := make(map[string]struct{}, len(deps))
	for _, dep := range deps {
		versions[dep["version"].(string)] = struct{}{}
	}

	resp, err := http.Get("https://go.dev/dl/?mode=json&include=all")
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	var releases []struct {
		Version string
		Stable  bool
		Files   []struct {
			Sha256   string
			Filename string
			Arch     string
			OS       string
		}
	}
	err = json.NewDecoder(resp.Body).Decode(&releases)
	if err != nil {
		return err
	}

	var replacements = make([]struct {
		Old string
		New string
	}, 0, len(releases))

	for _, r := range releases {
		if _, ok := versions[strings.TrimPrefix(r.Version, "go")]; !ok {
			continue
		}
		var newSha256, newFilename, oldSha256, oldFilename string
		for _, f := range r.Files {
			if f.OS != "linux" {
				continue
			}
			switch f.Arch {
			case "amd64":
				oldSha256, oldFilename = f.Sha256, f.Filename
			case arch:
				newSha256, newFilename = f.Sha256, f.Filename
			default:
				continue
			}
		}
		replacements = append(replacements,
			struct {
				Old string
				New string
			}{Old: oldSha256, New: newSha256},
			struct {
				Old string
				New string
			}{Old: "/" + oldFilename, New: "/" + newFilename})

	}

	tomlStr := string(tomlBytes)
	for _, r := range replacements {
		tomlStr = strings.ReplaceAll(tomlStr, r.Old, r.New)
	}

	err = os.WriteFile(buildpackToml, []byte(tomlStr), 0644)
	if err != nil {
		return err
	}

	return nil
}
