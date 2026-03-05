package docker

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"

	mobyClient "github.com/moby/moby/client"
)

// newClientWithPodmanService creates a moby client that has its own podman
// service associated with it. The service is shutdown when Close() is called
// on the client.
func newClientWithPodmanService() (dockerClient *mobyClient.Client, dockerHost string, err error) {
	tmpDir, err := os.MkdirTemp("", "func-podman-")
	if err != nil {
		return
	}

	podmanSocket := filepath.Join(tmpDir, "podman.sock")
	dockerHost = fmt.Sprintf("unix://%s", podmanSocket)

	cmd := exec.Command("podman", "system", "service", dockerHost, "--time=0")

	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true, Pgid: 0}
	outBuff := bytes.Buffer{}
	cmd.Stdout = &outBuff
	cmd.Stderr = &outBuff

	err = cmd.Start()
	if err != nil {
		return
	}

	waitErrCh := make(chan error)
	go func() { waitErrCh <- cmd.Wait() }()

	dockerClient, err = mobyClient.New(mobyClient.FromEnv, mobyClient.WithHost(dockerHost))
	if err != nil {
		return
	}
	stopPodmanService := func() {
		_ = cmd.Process.Signal(syscall.SIGTERM)
		_ = os.RemoveAll(tmpDir)

		select {
		case <-waitErrCh:
			return
		case <-time.After(time.Second * 1):
			_ = cmd.Process.Signal(syscall.SIGKILL)
		}
	}

	svcUpCh := make(chan struct{})
	go func() {
		for i := 0; i < 40; i++ {
			if _, e := dockerClient.Ping(context.Background(), mobyClient.PingOptions{}); e == nil {
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
		if err != nil {
			err = fmt.Errorf("failed to start the podman service (cmd out: %q): %w", outBuff.String(), err)
		} else {
			err = fmt.Errorf("the podman process exited before the service come up (cmd out: %q)", outBuff.String())
		}
	}

	return
}
