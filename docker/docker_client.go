package docker

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"runtime"

	"github.com/docker/docker/client"
	"golang.org/x/crypto/ssh"

	fnssh "knative.dev/kn-plugin-func/ssh"
)

var ErrNoDocker = errors.New("docker/podman API not available")

// NewClient creates a new docker client.
// reads the DOCKER_HOST envvar but it may or may not return it as dockerHost.
//  - For local connection (unix socket and windows named pipe) it returns the
//    DOCKER_HOST directly.
//  - For ssh connections it reads the DOCKER_HOST from the ssh remote.
//  - For TCP connections it returns "" so it defaults in the remote (note that
//    one should not be use client.DefaultDockerHost in this situation). This is
//    needed beaus of TCP+tls connections.
func NewClient(defaultHost string) (dockerClient client.CommonAPIClient, dockerHostInRemote string, err error) {
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
		case os.IsNotExist(err) && podmanPresent():
			if runtime.GOOS == "linux" {
				// on Linux: spawn temporary podman service
				dockerClient, dockerHostInRemote, err = newClientWithPodmanService()
				dockerClient = &closeGuardingClient{pimpl: dockerClient}
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

	if isUnix && runtime.GOOS == "darwin" {
		// A unix socket on macOS is most likely tunneled from VM,
		// so it cannot be mounted under that path.
		dockerHostInRemote = ""
	}

	if !isSSH {
		dockerClient, err = client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation(), client.WithHost(dockerHost))
		dockerClient = &closeGuardingClient{pimpl: dockerClient}
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

	dockerClient, err = client.NewClientWithOpts(
		client.WithAPIVersionNegotiation(),
		client.WithHTTPClient(httpClient),
		client.WithHost("http://placeholder/"))

	if closer, ok := contextDialer.(io.Closer); ok {
		dockerClient = clientWithAdditionalCleanup{
			CommonAPIClient: dockerClient,
			cleanUp: func() {
				closer.Close()
			},
		}
	}

	dockerClient = &closeGuardingClient{pimpl: dockerClient}
	return dockerClient, dockerHostInRemote, err
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

type clientWithAdditionalCleanup struct {
	client.CommonAPIClient
	cleanUp func()
}

// Close function need to stop associated podman service
func (w clientWithAdditionalCleanup) Close() error {
	defer w.cleanUp()
	return w.CommonAPIClient.Close()
}
