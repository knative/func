package main

import (
	"archive/tar"
	"bytes"
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

	"golang.org/x/oauth2"
	"golang.org/x/term"

	"github.com/buildpacks/pack/builder"
	pack "github.com/buildpacks/pack/pkg/client"
	"github.com/buildpacks/pack/pkg/dist"
	"github.com/buildpacks/pack/pkg/image"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/registry"
	docker "github.com/docker/docker/client"
	"github.com/docker/docker/pkg/jsonmessage"
	"github.com/google/go-containerregistry/pkg/authn"
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

	var hadError bool
	for _, variant := range []string{"tiny", "base", "full"} {
		fmt.Println("::group::" + variant)
		err := buildBuilderImage(ctx, variant)
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

func buildBuilderImage(ctx context.Context, variant string) error {
	buildDir, err := os.MkdirTemp("", "")
	if err != nil {
		return fmt.Errorf("cannot create temporary build directory: %w", err)
	}
	defer func(path string) {
		_ = os.RemoveAll(path)
	}(buildDir)

	ghClient := github.NewClient(oauth2.NewClient(ctx, oauth2.StaticTokenSource(&oauth2.Token{
		AccessToken: os.Getenv("GITHUB_TOKEN"),
	})))
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

	newBuilderImage := "ghcr.io/knative/builder-jammy-" + variant
	newBuilderImageTagged := newBuilderImage + ":" + *release.Name
	newBuilderImageLatest := newBuilderImage + ":latest"
	dockerUser := "gh-action"
	dockerPassword := os.Getenv("GITHUB_TOKEN")

	ref, err := name.ParseReference(newBuilderImageTagged)
	if err != nil {
		return fmt.Errorf("cannot parse reference to builder target: %w", err)
	}
	_, err = remote.Head(ref, remote.WithAuth(auth{dockerUser, dockerPassword}))
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

	imgBldOpts := types.ImageBuildOptions{
		Tags: []string{newBuilderImageLatest, newBuilderImageTagged},
		Labels: map[string]string{
			"org.opencontainers.image.description": "Paketo Jammy builder enriched with Rust and Func-Go buildpacks.",
			"org.opencontainers.image.source":      "https://github.com/knative/func",
			"org.opencontainers.image.vendor":      "https://github.com/knative/func",
			"org.opencontainers.image.url":         "https://github.com/knative/func/pkgs/container/builder-jammy-" + variant,
			"org.opencontainers.image.version":     *release.Name,
		},
	}

	dockerFile := "FROM " + newBuilderImageTagged
	var buildCtxBuff bytes.Buffer
	tw := tar.NewWriter(&buildCtxBuff)
	hdr := tar.Header{Typeflag: tar.TypeReg, Name: "Dockerfile", Size: int64(len(dockerFile)), Mode: 0644}
	err = tw.WriteHeader(&hdr)
	if err != nil {
		return fmt.Errorf("cannot write tar header: %w", err)
	}
	_, err = tw.Write([]byte(dockerFile))
	if err != nil {
		return fmt.Errorf("cannot write docker file: %w", err)
	}
	_ = tw.Close()

	imgBldResp, err := dockerClient.ImageBuild(ctx, &buildCtxBuff, imgBldOpts)
	if err != nil {
		return fmt.Errorf("cannot initialize build of image with additional labels: %w", err)
	}
	defer func(Body io.ReadCloser) {
		_ = Body.Close()
	}(imgBldResp.Body)
	fd := os.Stdout.Fd()
	isTerminal := term.IsTerminal(int(os.Stdout.Fd()))
	err = jsonmessage.DisplayJSONMessagesStream(imgBldResp.Body, os.Stdout, fd, isTerminal, nil)
	if err != nil {
		return fmt.Errorf("cannot build image with additional labels: %w", err)
	}

	authConfig := registry.AuthConfig{
		Username: dockerUser,
		Password: dockerPassword,
	}
	bs, err := json.Marshal(&authConfig)
	if err != nil {
		return fmt.Errorf("cannot marshal credentials: %w", err)
	}
	imagePushOptions := types.ImagePushOptions{
		All:          false,
		RegistryAuth: base64.StdEncoding.EncodeToString(bs),
	}

	pushImage := func(image string) error {
		rc, err := dockerClient.ImagePush(ctx, image, imagePushOptions)
		if err != nil {
			return fmt.Errorf("cannot initialize image push: %w", err)
		}
		defer func(rc io.ReadCloser) {
			_ = rc.Close()
		}(rc)
		fd := os.Stdout.Fd()
		isTerminal := term.IsTerminal(int(os.Stdout.Fd()))
		err = jsonmessage.DisplayJSONMessagesStream(rc, os.Stderr, fd, isTerminal, nil)
		if err != nil {
			return err
		}
		return nil
	}

	err = pushImage(newBuilderImageTagged)
	if err != nil {
		return fmt.Errorf("cannot push the image: %w", err)
	}

	err = pushImage(newBuilderImageLatest)
	if err != nil {
		return fmt.Errorf("cannot push the image: %w", err)
	}

	return nil
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
func patchBuilder(config *builder.Config) {
	config.Description += "\nAddendum: this is modified builder that also contains Rust and Func-Go buildpacks."
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
