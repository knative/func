package docker

import (
	"bytes"
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

// NewClient creates a new docker client.
// reads the DOCKER_HOST envvar but it may or may not return it as dockerHost.
//  - For local connection (unix socket and windows named pipe) it returns the
//    DOCKER_HOST directly.
//  - For ssh connections it reads the DOCKER_HOST from the ssh remote.
//  - For TCP connections it returns "" so it defaults in the remote (note that
//    one should not be use client.DefaultDockerHost in this situation). This is
//    needed beaus of TCP+tls connections.
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
			dockerClient = &closeGuardingClient{pimpl: dockerClient}
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
		dockerClient = &closeGuardingClient{pimpl: dockerClient}
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
			CommonAPIClient: dockerClient,
			cleanUp: func() {
				closer.Close()
			},
		}
	}

	dockerClient = &closeGuardingClient{pimpl: dockerClient}
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

	outBuff := bytes.Buffer{}
	cmd.Stdout = &outBuff
	cmd.Stderr = &outBuff

	err = cmd.Start()
	if err != nil {
		return
	}

	waitErrCh := make(chan error)
	go func() { waitErrCh <- cmd.Wait() }()

	dockerClient, err = client.NewClientWithOpts(client.FromEnv, client.WithHost(dockerHost), client.WithAPIVersionNegotiation())
	stopPodmanService := func() {
		_ = cmd.Process.Signal(syscall.SIGTERM)
		_ = os.RemoveAll(tmpDir)

		select {
		case <-waitErrCh:
			// the podman service has been shutdown, we don't care about error
			return
		case <-time.After(time.Second * 1):
			// failed to gracefully shutdown the podman service, sending SIGKILL
			_ = cmd.Process.Signal(syscall.SIGKILL)
		}
	}
	dockerClient = clientWithAdditionalCleanup{
		CommonAPIClient: dockerClient,
		cleanUp:         stopPodmanService,
	}

	svcUpCh := make(chan struct{})
	go func() {
		// give a time to podman to start
		for i := 0; i < 40; i++ {
			if _, e := dockerClient.Ping(context.Background()); e == nil {
				svcUpCh <- struct{}{}
			}
			time.Sleep(time.Millisecond * 250)
		}
	}()

	select {
	case <-svcUpCh:
		return
	case <-time.After(time.Second * 10):
		stopPodmanService()
		err = errors.New("the podman service has not come up in time")
	case err = <-waitErrCh:
		// If this `case` is not selected then the waitErrCh is eventually read by calling stopPodmanService
		if err != nil {
			err = fmt.Errorf("failed to start the podman service (cmd out: %q): %w", outBuff.String(), err)
		} else {
			err = fmt.Errorf("the podman process exited before the service come up (cmd out: %q)", outBuff.String())
		}
	}

	return
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
