package docker

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
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
			// Default socket doesn't exist, try Docker context
			if contextHost := GetDockerContextHostFunc(); contextHost != "" {
				// Verify the context socket actually exists
				contextURL, parseErr := url.Parse(contextHost)
				if parseErr == nil && (contextURL.Scheme == "unix" || contextURL.Scheme == "") {
					socketPath := contextURL.Path
					if contextURL.Scheme == "" {
						socketPath = contextHost
					}
					if _, statErr := os.Stat(socketPath); statErr == nil {
						dockerHost = contextHost
					}
				}
			}

			// If context didn't work, try Podman
			if dockerHost == "" && podmanPresent() {
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
				opts = append(opts, client.WithHTTPClient(httpClient))
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

// getDockerContextHost tries to get the Docker host from the current Docker context.
// This is useful for Docker Desktop which uses context-specific sockets.
// Returns empty string if unable to determine the context host.
func getDockerContextHost() string {
	// Check if docker CLI is available
	dockerPath, err := exec.LookPath("docker")
	if err != nil {
		return ""
	}

	// Run 'docker context inspect' to get current context details
	cmd := exec.Command(dockerPath, "context", "inspect")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return ""
	}

	// Parse the JSON output
	var contexts []struct {
		Name      string
		Endpoints struct {
			Docker struct {
				Host string `json:"Host"`
			} `json:"docker"`
		} `json:"Endpoints"`
	}

	if err := json.Unmarshal(out, &contexts); err != nil {
		return ""
	}

	// Return the host from the first (current) context
	if len(contexts) > 0 && contexts[0].Endpoints.Docker.Host != "" {
		return contexts[0].Endpoints.Docker.Host
	}

	return ""
}

// GetDockerContextHostFunc is a variable to allow mocking in tests
var GetDockerContextHostFunc = getDockerContextHost

type clientWithAdditionalCleanup struct {
	client.APIClient
	cleanUp func()
}

// Close function need to stop associated podman service
func (w clientWithAdditionalCleanup) Close() error {
	defer w.cleanUp()
	return w.APIClient.Close()
}
