package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"syscall"

	"knative.dev/func/cmd"
	"knative.dev/func/docker"
)

// Statically-populated build metadata set by `make build`.
var date, vers, hash string

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigs
		cancel()
		<-sigs // second sigint/sigterm is treated as sigkill
		os.Exit(137)
	}()

	cfg := cmd.RootCommandConfig{
		Name: "func",
		Version: cmd.Version{
			Date: date,
			Vers: vers,
			Hash: hash,
		}}

	if err := cmd.NewRootCmd(cfg).ExecuteContext(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		if ctx.Err() != nil {
			os.Exit(130)
		}

		if errors.Is(err, docker.ErrNoDocker) {
			if !dockerOrPodmanInstalled() {
				fmt.Fprintln(os.Stderr, `Docker/Podman not installed.
Please consider installing one of these:
  https://podman-desktop.io/
  https://www.docker.com/products/docker-desktop/`)
			} else {
				fmt.Fprintln(os.Stderr, `Possible causes:
  The docker/podman daemon is not running.
  The DOCKER_HOST environment variable is not set.`)
			}
		}

		os.Exit(1)
	}
}

func dockerOrPodmanInstalled() bool {
	_, err := exec.LookPath("podman")
	if err == nil {
		return true
	}
	_, err = exec.LookPath("docker")
	return err == nil
}
