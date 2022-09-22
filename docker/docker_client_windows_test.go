package docker_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	winio "github.com/Microsoft/go-winio"
	"github.com/docker/docker/client"
	"knative.dev/kn-plugin-func/docker"
)

func TestNewClientWinPipe(t *testing.T) {

	const testNPipe = "test-npipe"

	defer startMockDaemonWinPipe(t, testNPipe)()
	t.Setenv("DOCKER_HOST", fmt.Sprintf("npipe:////./pipe/%s", testNPipe))

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute*1)
	defer cancel()

	dockerClient, dockerHostToMount, err := docker.NewClient(client.DefaultDockerHost)
	if err != nil {
		t.Error(err)
	}
	defer dockerClient.Close()

	if dockerHostToMount != "" {
		t.Error("dockerHostToMount should be empty for npipe")
	}

	_, err = dockerClient.Ping(ctx)
	if err != nil {
		t.Error(err)
	}
}

func startMockDaemonWinPipe(t *testing.T, pipeName string) func() {
	p, err := winio.ListenPipe(`\\.\pipe\`+pipeName, nil)
	if err != nil {
		t.Fatal(err)
	}
	return startMockDaemon(t, p)
}
