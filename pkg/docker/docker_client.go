package docker

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/docker/cli/cli/config"
	ctxdocker "github.com/docker/cli/cli/context/docker"
	"github.com/docker/cli/cli/context/store"
	"github.com/docker/go-connections/tlsconfig"
	"github.com/google/go-containerregistry/pkg/v1/daemon"
	"github.com/moby/moby/client"
	"golang.org/x/crypto/ssh"

	fnssh "knative.dev/func/pkg/ssh"
)

// DockerClient is a subset of client.APIClient with only the methods
// needed by this project and its third-party integrations (pack, s2i).
type DockerClient interface {
	daemon.Client // Ping, ImageSave, ImageLoad, ImageTag, ImageInspect, ImageHistory

	ContainerAttach(ctx context.Context, container string, options client.ContainerAttachOptions) (client.ContainerAttachResult, error)
	ContainerCommit(ctx context.Context, container string, options client.ContainerCommitOptions) (client.ContainerCommitResult, error)
	ContainerCreate(ctx context.Context, options client.ContainerCreateOptions) (client.ContainerCreateResult, error)
	ContainerInspect(ctx context.Context, containerID string, options client.ContainerInspectOptions) (client.ContainerInspectResult, error)
	ContainerKill(ctx context.Context, containerID string, options client.ContainerKillOptions) (client.ContainerKillResult, error)
	ContainerRemove(ctx context.Context, container string, options client.ContainerRemoveOptions) (client.ContainerRemoveResult, error)
	ContainerStart(ctx context.Context, container string, options client.ContainerStartOptions) (client.ContainerStartResult, error)
	ContainerStop(ctx context.Context, containerID string, options client.ContainerStopOptions) (client.ContainerStopResult, error)
	ContainerWait(ctx context.Context, containerID string, options client.ContainerWaitOptions) client.ContainerWaitResult
	CopyFromContainer(ctx context.Context, containerID string, options client.CopyFromContainerOptions) (client.CopyFromContainerResult, error)
	CopyToContainer(ctx context.Context, container string, options client.CopyToContainerOptions) (client.CopyToContainerResult, error)

	ImageBuild(ctx context.Context, buildContext io.Reader, options client.ImageBuildOptions) (client.ImageBuildResult, error)
	ImagePull(ctx context.Context, ref string, options client.ImagePullOptions) (client.ImagePullResponse, error)
	ImagePush(ctx context.Context, ref string, options client.ImagePushOptions) (client.ImagePushResponse, error)
	ImageRemove(ctx context.Context, image string, options client.ImageRemoveOptions) (client.ImageRemoveResult, error)

	Info(ctx context.Context, options client.InfoOptions) (client.SystemInfoResult, error)
	NetworkCreate(ctx context.Context, name string, options client.NetworkCreateOptions) (client.NetworkCreateResult, error)
	NetworkRemove(ctx context.Context, networkID string, options client.NetworkRemoveOptions) (client.NetworkRemoveResult, error)
	ServerVersion(ctx context.Context, options client.ServerVersionOptions) (client.ServerVersionResult, error)
	VolumeList(ctx context.Context, options client.VolumeListOptions) (client.VolumeListResult, error)
	VolumeRemove(ctx context.Context, volumeID string, options client.VolumeRemoveOptions) (client.VolumeRemoveResult, error)

	Close() error
}

var ErrNoDocker = errors.New("docker/podman API not available")

// NewClient creates a new docker client.
// reads the DOCKER_HOST envvar but it may or may not return it as dockerHost.
//   - For local connection (unix socket and windows named pipe) it returns the
//     DOCKER_HOST directly.
//   - For ssh connections it reads the DOCKER_HOST from the ssh remote.
//   - For TCP connections it returns "" so it defaults in the remote (note that
//     one should not be use client.DefaultDockerHost in this situation). This is
//     needed beaus of TCP+tls connections.
//
// If DOCKER_HOST is not set, the host (and TLS material) of the current
// non-default Docker context is used, falling back to defaultHost.
func NewClient(defaultHost string) (dc DockerClient, dockerHostInRemote string, err error) {
	var rawClient client.APIClient
	defer func() {
		if rawClient != nil && err == nil {
			dc = &closeGuardingClient{pimpl: rawClient}
		}
	}()

	var _url *url.URL

	dockerHost := os.Getenv("DOCKER_HOST")
	dockerHostSSHIdentity := os.Getenv("DOCKER_HOST_SSH_IDENTITY")
	hostKeyCallback := fnssh.NewHostKeyCbk()

	// A selected (non-default) Docker context takes precedence over the
	// default host, as it does for the docker CLI.
	var dockerCtx *dockerContext
	if dockerHost == "" {
		if c := currentDockerContext(); c != nil && isUsableContextHost(c.endpoint.Host) {
			dockerHost = c.endpoint.Host
			dockerCtx = c
		}
	}

	if dockerHost == "" {
		_url, err = url.Parse(defaultHost)
		if err != nil {
			return
		}
		_, err = os.Stat(_url.Path)
		switch {
		case err == nil:
			dockerHost = defaultHost
		case err != nil && !os.IsNotExist(err):
			return
		case os.IsNotExist(err):
			// Default socket doesn't exist, try Podman
			if podmanPresent() {
				if runtime.GOOS == "linux" {
					// on Linux: spawn temporary podman service
					rawClient, dockerHostInRemote, err = newClientWithPodmanService()
					return
				} else {
					// on non-Linux: try to use connection to podman machine
					dh, dhid := tryGetPodmanRemoteConn()
					if dh != "" {
						dockerHost, dockerHostSSHIdentity = dh, dhid
						hostKeyCallback = func(hostPort string, pubKey ssh.PublicKey) error {
							return nil
						}
					}
				}
			}
		}
	}

	if dockerHost == "" {
		return nil, "", ErrNoDocker
	}

	dockerHostInRemote = dockerHost

	_url, err = url.Parse(dockerHost)
	isSSH := err == nil && _url.Scheme == "ssh"
	isTCP := err == nil && _url.Scheme == "tcp"
	isNPipe := err == nil && _url.Scheme == "npipe"
	isUnix := err == nil && _url.Scheme == "unix"

	if isTCP || isNPipe {
		// With TCP or npipe, it's difficult to determine how to expose the daemon socket to lifecycle containers,
		// so we are defaulting to standard docker location by returning empty string.
		// This should work well most of the time.
		dockerHostInRemote = ""
	}

	if isUnix && (runtime.GOOS == "darwin" || strings.HasSuffix(dockerHost, ".docker/desktop/docker.sock")) {
		// The unix socket is most likely tunneled from VM,
		// so it cannot be mounted under that path.
		dockerHostInRemote = ""
	}

	if !isSSH {
		opts := []client.Opt{client.FromEnv, client.WithHost(dockerHost)}
		if isTCP {
			if httpClient := newHttpClient(); httpClient != nil {
				// DOCKER_TLS_VERIFY/DOCKER_CERT_PATH override the context TLS.
				opts = append(opts, client.WithHTTPClient(httpClient))
			} else if dockerCtx != nil {
				ctxOpts, ctxErr := dockerCtx.endpoint.ClientOpts()
				if ctxErr != nil {
					return nil, "", fmt.Errorf("cannot load TLS configuration of docker context %q: %w", dockerCtx.name, ctxErr)
				}
				opts = append(opts, ctxOpts...)
			}
		}
		rawClient, err = client.New(opts...)
		return
	}

	credentialsConfig := fnssh.Config{
		Identity:           dockerHostSSHIdentity,
		PassPhrase:         os.Getenv("DOCKER_HOST_SSH_IDENTITY_PASSPHRASE"),
		PasswordCallback:   fnssh.NewPasswordCbk(),
		PassPhraseCallback: fnssh.NewPassPhraseCbk(),
		HostKeyCallback:    hostKeyCallback,
	}
	contextDialer, dockerHostInRemote, err := fnssh.NewDialContext(_url, credentialsConfig)
	if err != nil {
		return
	}

	httpClient := &http.Client{
		// No tls
		// No proxy
		Transport: &http.Transport{
			DialContext: contextDialer.DialContext,
		},
	}

	rawClient, err = client.New(
		client.WithHTTPClient(httpClient))

	if closer, ok := contextDialer.(io.Closer); ok {
		rawClient = clientWithAdditionalCleanup{
			APIClient: rawClient,
			cleanUp: func() {
				closer.Close()
			},
		}
	}

	return dc, dockerHostInRemote, err
}

// If the DOCKER_TLS_VERIFY environment variable is set
// this function returns HTTP client with appropriately configured TLS config.
// Otherwise, nil is returned.
func newHttpClient() *http.Client {
	tlsVerifyStr, tlsVerifyChanged := os.LookupEnv("DOCKER_TLS_VERIFY")

	if !tlsVerifyChanged {
		return nil
	}

	var tlsOpts []func(*tls.Config)

	tlsVerify := true
	if b, err := strconv.ParseBool(tlsVerifyStr); err == nil {
		tlsVerify = b
	}

	if !tlsVerify {
		tlsOpts = append(tlsOpts, func(t *tls.Config) {
			t.InsecureSkipVerify = true
		})
	}

	dockerCertPath := os.Getenv("DOCKER_CERT_PATH")
	if dockerCertPath == "" {
		dockerCertPath = config.Dir()
	}

	// Set root CA.
	caData, err := os.ReadFile(filepath.Join(dockerCertPath, "ca.pem"))
	if err == nil {
		certPool := x509.NewCertPool()
		if certPool.AppendCertsFromPEM(caData) {
			tlsOpts = append(tlsOpts, func(t *tls.Config) {
				t.RootCAs = certPool
			})
		}
	}

	// Set client certificate.
	certData, certErr := os.ReadFile(filepath.Join(dockerCertPath, "cert.pem"))
	keyData, keyErr := os.ReadFile(filepath.Join(dockerCertPath, "key.pem"))
	if certErr == nil && keyErr == nil {
		cliCert, err := tls.X509KeyPair(certData, keyData)
		if err == nil {
			tlsOpts = append(tlsOpts, func(cfg *tls.Config) {
				cfg.Certificates = []tls.Certificate{cliCert}
			})
		}
	}

	dialer := &net.Dialer{
		KeepAlive: 30 * time.Second,
		Timeout:   30 * time.Second,
	}

	tlsConfig := tlsconfig.ClientDefault(tlsOpts...)

	return &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: tlsConfig,
			DialContext:     dialer.DialContext,
		},
		CheckRedirect: client.CheckRedirect,
	}
}

// tries to get connection to default podman machine
func tryGetPodmanRemoteConn() (uri string, identity string) {
	cmd := exec.Command("podman", "system", "connection", "list", "--format=json")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", ""
	}
	var connections []struct {
		Name     string
		URI      string
		Identity string
		Default  bool
	}
	err = json.Unmarshal(out, &connections)
	if err != nil {
		return "", ""
	}

	for _, c := range connections {
		if c.Default {
			uri = c.URI
			identity = c.Identity
			break
		}
	}

	return uri, identity
}

func podmanPresent() bool {
	_, err := exec.LookPath("podman")
	return err == nil
}

// dockerContext is the docker endpoint of the currently selected Docker context,
// including its TLS material.
type dockerContext struct {
	name     string
	endpoint ctxdocker.Endpoint
}

// currentDockerContext resolves the current Docker context the same way the
// docker CLI does: DOCKER_CONTEXT, or currentContext from config.json in the
// Docker config directory (DOCKER_CONFIG or ~/.docker).
// It returns nil for the default context, for a context without a docker
// endpoint host, and when the context cannot be loaded.
func currentDockerContext() *dockerContext {
	configDir := os.Getenv("DOCKER_CONFIG")
	if configDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil
		}
		configDir = filepath.Join(home, ".docker")
	}

	name := os.Getenv("DOCKER_CONTEXT")
	if name == "" {
		cfg, err := config.Load(configDir)
		if err != nil {
			return nil
		}
		name = cfg.CurrentContext
	}
	if name == "" || name == "default" {
		return nil
	}

	s := store.New(filepath.Join(configDir, "contexts"), store.NewConfig(nil,
		store.EndpointTypeGetter(ctxdocker.DockerEndpoint, func() any { return &ctxdocker.EndpointMeta{} })))
	meta, err := s.GetMetadata(name)
	if err != nil {
		return nil
	}
	epMeta, err := ctxdocker.EndpointFromContext(meta)
	if err != nil || epMeta.Host == "" {
		return nil
	}
	ep, err := ctxdocker.WithTLSData(s, name, epMeta)
	if err != nil {
		return nil
	}
	return &dockerContext{name: name, endpoint: ep}
}

// isUsableContextHost reports whether the host of a Docker context can be used.
// Unix sockets must exist; ssh, tcp and npipe hosts are accepted as is.
func isUsableContextHost(host string) bool {
	u, err := url.Parse(host)
	if err != nil {
		return false
	}
	switch u.Scheme {
	case "unix", "":
		socketPath := u.Path
		if u.Scheme == "" {
			socketPath = host
		}
		_, err = os.Stat(socketPath)
		return err == nil
	case "ssh", "tcp", "npipe":
		return true
	}
	return false
}

type clientWithAdditionalCleanup struct {
	client.APIClient
	cleanUp func()
}

// Close function need to stop associated podman service
func (w clientWithAdditionalCleanup) Close() error {
	defer w.cleanUp()
	return w.APIClient.Close()
}
