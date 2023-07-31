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
	"strings"
	"syscall"

	"github.com/buildpacks/pack/builder"
	pack "github.com/buildpacks/pack/pkg/client"
	"github.com/buildpacks/pack/pkg/dist"
	"github.com/buildpacks/pack/pkg/image"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/registry"
	docker "github.com/docker/docker/client"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-github/v49/github"
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

	err := buildBuilderImage(ctx)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "ERROR: %v", err)
		os.Exit(1)
	}
}

func buildBuilderImage(ctx context.Context) error {
	buildDir, err := os.MkdirTemp("", "")
	if err != nil {
		return fmt.Errorf("cannot create temporary build directory: %w", err)
	}
	defer func(path string) {
		_ = os.RemoveAll(path)
	}(buildDir)

	ghClient := github.NewClient(http.DefaultClient)
	listOpts := &github.ListOptions{Page: 0, PerPage: 1}
	releases, ghResp, err := ghClient.Repositories.ListReleases(ctx, "paketo-buildpacks", "builder-jammy-full", listOpts)
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

	newBuilderImage := "ghcr.io/matejvasek/builder-jammy-full"
	newBuilderImageTagged := newBuilderImage + ":" + *release.Name

	ref, err := name.ParseReference(newBuilderImageTagged)
	if err != nil {
		return fmt.Errorf("cannot parse reference to builder target: %w", err)
	}
	_, err = remote.Head(ref)
	if err == nil {
		fmt.Fprintln(os.Stderr, "The image has been already built.")
		return nil
	}

	builderTomlPath := filepath.Join(buildDir, "builder.toml")
	err = downloadBuilderToml(ctx, *release.TarballURL, builderTomlPath)
	if err != nil {
		return fmt.Errorf("cannot download builder toml: %w", err)
	}

	builderConfig, _, err := builder.ReadConfig(builderTomlPath)
	if err != nil {
		return fmt.Errorf("cannot parse builder.toml: %w", err)
	}

	patchBuilder(&builderConfig)

	packClient, err := pack.NewClient()
	if err != nil {
		return fmt.Errorf("cannot create pack client: %w", err)
	}
	createBuilderOpts := pack.CreateBuilderOptions{
		RelativeBaseDir: buildDir,
		BuilderName:     newBuilderImageTagged,
		Config:          builderConfig,
		Publish:         false,
		PullPolicy:      image.PullIfNotPresent,
	}
	err = packClient.CreateBuilder(ctx, createBuilderOpts)
	if err != nil {
		return fmt.Errorf("canont create builder: %w", err)
	}

	dockerClient, err := docker.NewClientWithOpts(docker.FromEnv, docker.WithAPIVersionNegotiation())
	if err != nil {
		return fmt.Errorf("cannot create docker client")
	}

	err = dockerClient.ImageTag(ctx, newBuilderImageTagged, newBuilderImage+":tip")
	if err != nil {
		return fmt.Errorf("cannot tag latest image: %w", err)
	}

	authConfig := registry.AuthConfig{
		Username: "gh-action-bot",
		Password: os.Getenv("GITHUB_TOKEN"),
	}
	bs, err := json.Marshal(&authConfig)
	if err != nil {
		return fmt.Errorf("cannot marshal credentials: %w", err)
	}
	imagePushOptions := types.ImagePushOptions{
		All:          false,
		RegistryAuth: base64.StdEncoding.EncodeToString(bs),
	}

	rc, err := dockerClient.ImagePush(ctx, newBuilderImage+":tip", imagePushOptions)
	if err != nil {
		return fmt.Errorf("cannot push the image: %w", err)
	}
	defer func(rc io.ReadCloser) {
		_ = rc.Close()
	}(rc)
	_, _ = io.Copy(os.Stderr, rc)

	return nil
}

func downloadBuilderToml(ctx context.Context, tarballUrl, builderTomlPath string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, tarballUrl, nil)
	if err != nil {
		return fmt.Errorf("cannot create request for release tarball: %w", err)
	}
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
func patchBuilder(config *builder.Config) {
	additionalBuildpacks := []builder.ModuleConfig{
		{
			ModuleInfo: dist.ModuleInfo{
				ID:      "paketo-community/rust",
				Version: "0.35.0",
			},
			ImageOrURI: dist.ImageOrURI{
				BuildpackURI: dist.BuildpackURI{URI: "docker://docker.io/paketocommunity/rust:0.35.0"},
			},
		},
		{
			ModuleInfo: dist.ModuleInfo{
				ID:      "dev.knative-sandbox.go",
				Version: "0.0.4",
			},
			ImageOrURI: dist.ImageOrURI{
				BuildpackURI: dist.BuildpackURI{URI: "ghcr.io/boson-project/go-function-buildpack:0.0.4"},
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
						ID: "dev.knative-sandbox.go",
					},
				},
			},
		},
	}

	config.Buildpacks = append(additionalBuildpacks, config.Buildpacks...)
	config.Order = append(additionalGroups, config.Order...)
}
