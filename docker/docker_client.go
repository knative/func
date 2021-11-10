package docker

import (
	"context"
	"errors"
	"fmt"
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

func NewDockerClient(defaultDockerHost string) (dockerClient client.CommonAPIClient, dockerHost string, err error) {
	var _url *url.URL

	dockerHost = os.Getenv("DOCKER_HOST")

	if dockerHost == "" && runtime.GOOS == "linux" && podmanPresent() {
		_url, err = url.Parse(defaultDockerHost)
		if err != nil {
			return
		}
		_, err = os.Stat(_url.Path)
		switch {
		case err != nil && !os.IsNotExist(err):
			return
		case os.IsNotExist(err):
			dockerClient, dockerHost, err = newDockerClientWithPodmanService()
			return
		}
	}

	_url, err = url.Parse(dockerHost)
	isSSH := err == nil && _url.Scheme == "ssh"

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
	dialContext, dockerHost, err := ssh.NewDialContext(_url, credentialsConfig)
	if err != nil {
		return
	}

	httpClient := &http.Client{
		// No tls
		// No proxy
		Transport: &http.Transport{
			DialContext: dialContext,
		},
	}

	dockerClient, err = client.NewClientWithOpts(
		client.WithAPIVersionNegotiation(),
		client.WithHTTPClient(httpClient),
		client.WithHost("http://placeholder/"))

	return dockerClient, dockerHost, err
}

func podmanPresent() bool {
	_, err := exec.LookPath("podman")
	return err == nil
}

// creates a docker client that has its own podman service associated with it
// the service is shutdown when Close() is called on the client
func newDockerClientWithPodmanService() (dockerClient client.CommonAPIClient, dockerHost string, err error) {
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
	dockerClient = withPodman{
		pimpl:      dockerClient,
		stopPodman: stopPodmanService,
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

type withPodman struct {
	stopPodman func()
	pimpl      client.CommonAPIClient
}

// Close function need to stop associated podman service
func (w withPodman) Close() error {
	defer w.stopPodman()
	return w.pimpl.Close()
}
