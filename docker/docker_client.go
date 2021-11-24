package docker

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"syscall"
	"time"

	"knative.dev/kn-plugin-func/ssh"

	"github.com/docker/docker/client"
)

func NewClient(defaultHost string) (dockerClient client.CommonAPIClient, dockerHost string, err error) {
	var _url *url.URL

	dockerHost = os.Getenv("DOCKER_HOST")

	if dockerHost == "" && runtime.GOOS == "linux" && podmanPresent() {
		_url, err = url.Parse(defaultHost)
		if err != nil {
			return
		}
		_, err = os.Stat(_url.Path)
		switch {
		case err != nil && !os.IsNotExist(err):
			return
		case os.IsNotExist(err):
			dockerClient, dockerHost, err = newClientWithPodmanService()
			return
		}
	}

	_url, err = url.Parse(dockerHost)
	isSSH := err == nil && _url.Scheme == "ssh"
	isTCP := err == nil && _url.Scheme == "tcp"

	if isTCP {
		// With TCP, it's difficult to determine how to expose the daemon socket to lifecycle containers,
		// so we are defaulting to standard docker location by returning empty string.
		// This should work well most of the time.
		dockerHost = ""
	}

	if !isSSH {
		dockerClient, err = client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
		return
	}

	credentialsConfig := ssh.Config{
		Identity:           os.Getenv("DOCKER_HOST_SSH_IDENTITY"),
		PassPhrase:         os.Getenv("DOCKER_HOST_SSH_IDENTITY_PASSPHRASE"),
		PasswordCallback:   ssh.NewPasswordCbk(),
		PassPhraseCallback: ssh.NewPassPhraseCbk(),
		HostKeyCallback:    ssh.NewHostKeyCbk(),
	}
	contextDialer, dockerHost, err := ssh.NewDialContext(_url, credentialsConfig)
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
			pimpl: dockerClient,
			cleanUp: func() {
				closer.Close()
			},
		}
	}

	return dockerClient, dockerHost, err
}

func podmanPresent() bool {
	_, err := exec.LookPath("podman")
	return err == nil
}

// creates a docker client that has its own podman service associated with it
// the service is shutdown when Close() is called on the client
func newClientWithPodmanService() (dockerClient client.CommonAPIClient, dockerHost string, err error) {
	tmpDir, err := os.MkdirTemp("", "func-podman-")
	if err != nil {
		return
	}

	podmanSocket := filepath.Join(tmpDir, "podman.sock")
	dockerHost = fmt.Sprintf("unix://%s", podmanSocket)

	cmd := exec.Command("podman", "system", "service", dockerHost, "--time=0")
	err = cmd.Start()
	if err != nil {
		return
	}

	dockerClient, err = client.NewClientWithOpts(client.FromEnv, client.WithHost(dockerHost), client.WithAPIVersionNegotiation())
	stopPodmanService := func() {
		_ = cmd.Process.Signal(syscall.SIGTERM)
		_ = os.RemoveAll(tmpDir)
	}
	dockerClient = clientWithAdditionalCleanup{
		pimpl:   dockerClient,
		cleanUp: stopPodmanService,
	}

	podmanServiceRunning := false
	// give a time to podman to start
	for i := 0; i < 40; i++ {
		if _, e := dockerClient.Ping(context.Background()); e == nil {
			podmanServiceRunning = true
			break
		}
		time.Sleep(time.Millisecond * 250)
	}

	if !podmanServiceRunning {
		stopPodmanService()
		err = errors.New("failed to start podman service")
	}

	return
}

type clientWithAdditionalCleanup struct {
	cleanUp func()
	pimpl   client.CommonAPIClient
}

// Close function need to stop associated podman service
func (w clientWithAdditionalCleanup) Close() error {
	defer w.cleanUp()
	return w.pimpl.Close()
}
