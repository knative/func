package docker_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"knative.dev/kn-plugin-func/docker"
	. "knative.dev/kn-plugin-func/testing"
)

// Test that we are starting podman service on behalf of user
// if docker daemon is not present.
func TestNewDockerClientWithAutomaticPodmanSuccess(t *testing.T) {

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute*1)
	defer cancel()

	WithExecutable(t, "podman", mockPodmanSrc)
	t.Setenv("DOCKER_HOST", "")

	dockerClient, dockerHostToMount, err := docker.NewClient("unix:///var/run/nonexistent.sock")
	if err != nil {
		t.Fatal(err)
	}
	defer dockerClient.Close()
	if !strings.Contains(dockerHostToMount, "func-podman") {
		t.Error("got bad socket to mount")
	}

	_, err = dockerClient.Ping(ctx)
	if err != nil {
		t.Error(err)
	}

}

func TestNewDockerClientWithAutomaticPodmanFail(t *testing.T) {
	src := `package main;import ("os";"fmt");func main(){fmt.Println("something went wrong");os.Exit(1);}`

	WithExecutable(t, "podman", src)
	t.Setenv("DOCKER_HOST", "")

	_, _, err := docker.NewClient("unix:///var/run/nonexistent.sock")
	if err == nil {
		t.Error("expected error but got nil")
		return
	}
	if !strings.Contains(err.Error(), "something went wrong") {
		t.Error("error doesn't contain stdout of the podman command")
	}
}

// Go source code of mock podman implementation.
// It just emulates docker /_ping endpoint for all URIs.
const mockPodmanSrc = `package main

import (
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
)

func main() {
	dockerHost := os.Args[3]
	u, err := url.Parse(dockerHost)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	sock := u.Path

	server := http.Server{}
	listener, err := net.Listen("unix", sock)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	// mimics /_ping endpoint
	server.Handler = http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Add("Content-Type", "text/plain")
		writer.WriteHeader(200)
		writer.Write([]byte("OK"))
	})
	server.Serve(listener)
}
`
