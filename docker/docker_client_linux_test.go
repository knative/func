package docker_test

import (
	"context"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"knative.dev/kn-plugin-func/docker"
)

// Test that we are starting podman service on behalf of user
// if docker daemon is not present.
func TestNewDockerClientWithAutomaticPodman(t *testing.T) {

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute*1)
	defer cancel()

	defer withMockedPodmanBinary(t)()
	defer withEnvVarHost(t, "DOCKER_HOST", "")()

	dockerClient, _, err := docker.NewClient("unix:///var/run/nonexistent.sock")
	if err != nil {
		t.Error(err)
	}
	defer dockerClient.Close()

	_, err = dockerClient.Ping(ctx)
	if err != nil {
		t.Error(err)
	}

}

const helperGoScriptContent = `package main

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

func withMockedPodmanBinary(t *testing.T) func() {
	var err error
	binDir := t.TempDir()

	newPath := binDir + string(os.PathListSeparator) + os.Getenv("PATH")
	cleanUpPath := withEnvVarHost(t, "PATH", newPath)

	helperGoScriptPath := filepath.Join(binDir, "main.go")

	err = ioutil.WriteFile(helperGoScriptPath,
		[]byte(helperGoScriptContent),
		0400)
	if err != nil {
		t.Fatal(err)
	}

	runnerScriptName := "podman"
	if runtime.GOOS == "windows" {
		runnerScriptName = runnerScriptName + ".bat"
	}

	runnerScriptContent := `#!/bin/sh
exec go run GO_SCRIPT_PATH $@;
`
	if runtime.GOOS == "windows" {
		runnerScriptContent = `@echo off
go.exe run GO_SCRIPT_PATH %*
`
	}

	runnerScriptPath := filepath.Join(binDir, runnerScriptName)
	runnerScriptContent = strings.ReplaceAll(runnerScriptContent, "GO_SCRIPT_PATH", helperGoScriptPath)
	err = ioutil.WriteFile(runnerScriptPath, []byte(runnerScriptContent), 0700)
	if err != nil {
		t.Fatal(err)
	}

	return func() {
		cleanUpPath()
	}
}
